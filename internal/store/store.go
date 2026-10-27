// Package store is the data layer: one method set over DynamoDB for every
// entity. It returns *httpx.Error for the domain failures the client sees
// (not found, name taken), mirroring the repositories in the C# service.
package store

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/jsonx"
)

type Store struct {
	db  *dynamo.DB
	now func() time.Time
}

func New(db *dynamo.DB, now func() time.Time) *Store {
	return &Store{db: db, now: now}
}

func (s *Store) DB() *dynamo.DB { return s.db }

func (s *Store) stamp() jsonx.Timestamp { return jsonx.New(s.now()) }

// fanOut bounds the parallel reads and writes that replaced Task.WhenAll,
// which in the C# service was unbounded.
const fanOut = 16

// Counter names, matching IdGenerator.Entities so existing counters continue.
const (
	entityUser            = "User"
	entityWorkout         = "Workout"
	entityExercise        = "Exercise"
	entityWorkoutSession  = "WorkoutSession"
	entityExerciseSession = "ExerciseSession"
	entitySetSession      = "SetSession"
)

// nextIDs reserves n sequential ids and returns the first. Ids stay small
// integers because the client types every id as a number; they come from an
// atomic ADD on a counter item, so a block of n costs one round trip.
func (s *Store) nextIDs(ctx context.Context, entity string, n int) (int, error) {
	out, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.db.T.Counters),
		Key:                       dynamo.Item{"entity": dynamo.S(entity)},
		UpdateExpression:          aws.String("ADD #n :n"),
		ExpressionAttributeNames:  map[string]string{"#n": "value"},
		ExpressionAttributeValues: dynamo.Item{":n": dynamo.N(n)},
		ReturnValues:              types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}

	return dynamo.Int(out.Attributes, "value") - n + 1, nil
}

func (s *Store) nextID(ctx context.Context, entity string) (int, error) {
	return s.nextIDs(ctx, entity, 1)
}

// get reads one item into dst, reporting whether it existed.
func (s *Store) get(ctx context.Context, table string, key dynamo.Item, consistent bool, dst any) (bool, error) {
	out, err := s.db.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(table),
		Key:            key,
		ConsistentRead: aws.Bool(consistent),
	})
	if err != nil || out.Item == nil {
		return false, err
	}

	return true, attributevalue.UnmarshalMap(out.Item, dst)
}

func (s *Store) put(ctx context.Context, table string, v any) error {
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		return err
	}

	_, err = s.db.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(table), Item: item})

	return err
}

func (s *Store) deleteKey(ctx context.Context, table string, key dynamo.Item) error {
	_, err := s.db.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(table), Key: key})
	return err
}

func unmarshalAll[T any](items []dynamo.Item) ([]T, error) {
	out := make([]T, 0, len(items))

	if err := attributevalue.UnmarshalListOfMaps(items, &out); err != nil {
		return nil, err
	}

	return out, nil
}

// setExpr builds a "SET #a = :a, …" update expression from attribute values.
type setExpr struct {
	parts  []string
	names  map[string]string
	values dynamo.Item
}

func newSetExpr() *setExpr {
	return &setExpr{names: map[string]string{}, values: dynamo.Item{}}
}

func (e *setExpr) set(attr string, v types.AttributeValue) {
	i := strconv.Itoa(len(e.parts))
	e.names["#s"+i] = attr
	e.values[":s"+i] = v
	e.parts = append(e.parts, "#s"+i+" = :s"+i)
}

func (e *setExpr) empty() bool { return len(e.parts) == 0 }

func (e *setExpr) expression() *string { return aws.String("SET " + strings.Join(e.parts, ", ")) }

func unmarshalItem(item dynamo.Item, dst any) error {
	return attributevalue.UnmarshalMap(item, dst)
}
