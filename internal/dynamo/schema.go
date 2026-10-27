package dynamo

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Tables holds every table name, resolved against the prefix. The prefix is
// supplied by SAM in deployed environments and defaults to "Gravity_" locally;
// tests use a unique prefix per run so they never touch each other's data.
type Tables struct {
	Users                  string
	Usernames              string
	Workouts               string
	WorkoutNames           string
	Exercises              string
	ExerciseNames          string
	WorkoutSessions        string
	ExerciseSessions       string
	SetSessions            string
	Counters               string
	RateLimits             string
	Emails                 string
	PasswordResetCodes     string
	EmailConfirmationCodes string
}

// Index names. The order-index LSIs still exist on the deployed tables (an LSI
// can only be removed by recreating the table) but nothing reads them any
// more: the lists they sorted are small enough to sort in memory.
const (
	IndexWorkoutsByOrder            = "order-index"
	IndexExercisesByOrder           = "order-index"
	IndexSessionsByDate             = "date-index"
	IndexExerciseSessionsByExercise = "exerciseId-sessionDate-index"
)

func NewTables(prefix string) Tables {
	return Tables{
		Users:                  prefix + "Users",
		Usernames:              prefix + "Usernames",
		Workouts:               prefix + "Workouts",
		WorkoutNames:           prefix + "WorkoutNames",
		Exercises:              prefix + "Exercises",
		ExerciseNames:          prefix + "ExerciseNames",
		WorkoutSessions:        prefix + "WorkoutSessions",
		ExerciseSessions:       prefix + "ExerciseSessions",
		SetSessions:            prefix + "SetSessions",
		Counters:               prefix + "Counters",
		RateLimits:             prefix + "RateLimits",
		Emails:                 prefix + "Emails",
		PasswordResetCodes:     prefix + "PasswordResetCodes",
		EmailConfirmationCodes: prefix + "EmailConfirmationCodes",
	}
}

func (t Tables) all() []string {
	return []string{
		t.Users, t.Usernames, t.Workouts, t.WorkoutNames, t.Exercises, t.ExerciseNames,
		t.WorkoutSessions, t.ExerciseSessions, t.SetSessions, t.Counters, t.RateLimits,
		t.Emails, t.PasswordResetCodes, t.EmailConfirmationCodes,
	}
}

type keyDef struct{ name, typ string }

type indexDef struct {
	name           string
	partition, srt keyDef
}

type tableDef struct {
	name           string
	partition, srt keyDef
	local, global  []indexDef
	ttl            bool
}

var (
	numKey = func(name string) keyDef { return keyDef{name, "N"} }
	strKey = func(name string) keyDef { return keyDef{name, "S"} }
)

// definitions mirrors template.yaml (and LocalTables.cs before it), so local
// development matches production. The workoutId-date-index GSI is gone: its
// only reader was the workout-delete cascade, which now queries the user's
// sessions directly.
func (t Tables) definitions() []tableDef {
	return []tableDef{
		{name: t.Users, partition: numKey("id")},
		{name: t.Usernames, partition: strKey("username")},
		{name: t.Counters, partition: strKey("entity")},
		{name: t.RateLimits, partition: strKey("key"), ttl: true},
		{name: t.Emails, partition: strKey("email")},
		{name: t.PasswordResetCodes, partition: strKey("email"), ttl: true},
		{name: t.EmailConfirmationCodes, partition: numKey("userId"), ttl: true},
		{name: t.WorkoutNames, partition: numKey("userId"), srt: strKey("name")},
		{name: t.ExerciseNames, partition: numKey("workoutId"), srt: strKey("name")},
		{
			name: t.Workouts, partition: numKey("userId"), srt: numKey("id"),
			local: []indexDef{{IndexWorkoutsByOrder, numKey("userId"), numKey("order")}},
		},
		{
			name: t.Exercises, partition: numKey("workoutId"), srt: numKey("id"),
			local: []indexDef{{IndexExercisesByOrder, numKey("workoutId"), numKey("order")}},
		},
		{
			name: t.WorkoutSessions, partition: numKey("userId"), srt: numKey("id"),
			local: []indexDef{{IndexSessionsByDate, numKey("userId"), strKey("date")}},
		},
		{
			name: t.ExerciseSessions, partition: numKey("workoutSessionId"), srt: numKey("id"),
			global: []indexDef{{IndexExerciseSessionsByExercise, numKey("exerciseId"), strKey("sessionDate")}},
		},
		{name: t.SetSessions, partition: numKey("exerciseSessionId"), srt: numKey("id")},
	}
}

// EnsureTables creates any missing table. It runs only against DynamoDB Local;
// deployed environments get their tables from template.yaml.
func (db *DB) EnsureTables(ctx context.Context) error {
	existing, err := db.listTables(ctx)
	if err != nil {
		return err
	}

	for _, def := range db.T.definitions() {
		if slices.Contains(existing, def.name) {
			continue
		}

		if _, err := db.Client.CreateTable(ctx, def.createInput()); err != nil {
			return err
		}

		// Current DynamoDB Local versions do sweep expired items, against
		// wall-clock time. (Older ones ignored TTL; the C# service's comments
		// assumed that.) The code tables check expiry on read regardless.
		if def.ttl {
			_, err := db.Client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
				TableName: aws.String(def.name),
				TimeToLiveSpecification: &types.TimeToLiveSpecification{
					AttributeName: aws.String("expiresAt"),
					Enabled:       aws.Bool(true),
				},
			})
			if err != nil {
				return err
			}
		}

		slog.InfoContext(ctx, "created local table", "table", def.name)
	}

	return nil
}

// DropTables deletes every table under this prefix. Used by tests.
func (db *DB) DropTables(ctx context.Context) error {
	var errs []error

	for _, name := range db.T.all() {
		_, err := db.Client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(name)})

		var missing *types.ResourceNotFoundException
		if err != nil && !errors.As(err, &missing) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (db *DB) listTables(ctx context.Context) ([]string, error) {
	var names []string

	p := dynamodb.NewListTablesPaginator(db.Client, &dynamodb.ListTablesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		names = append(names, page.TableNames...)
	}

	return names, nil
}

func (def tableDef) createInput() *dynamodb.CreateTableInput {
	attrs := map[string]string{def.partition.name: def.partition.typ}
	if def.srt.name != "" {
		attrs[def.srt.name] = def.srt.typ
	}

	for _, ix := range slices.Concat(def.local, def.global) {
		attrs[ix.partition.name] = ix.partition.typ
		attrs[ix.srt.name] = ix.srt.typ
	}

	in := &dynamodb.CreateTableInput{
		TableName:   aws.String(def.name),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema:   schema(def.partition, def.srt),
	}

	for name, typ := range attrs {
		in.AttributeDefinitions = append(in.AttributeDefinitions, types.AttributeDefinition{
			AttributeName: aws.String(name),
			AttributeType: types.ScalarAttributeType(typ),
		})
	}

	all := &types.Projection{ProjectionType: types.ProjectionTypeAll}

	for _, ix := range def.local {
		in.LocalSecondaryIndexes = append(in.LocalSecondaryIndexes, types.LocalSecondaryIndex{
			IndexName: aws.String(ix.name), KeySchema: schema(ix.partition, ix.srt), Projection: all,
		})
	}

	for _, ix := range def.global {
		in.GlobalSecondaryIndexes = append(in.GlobalSecondaryIndexes, types.GlobalSecondaryIndex{
			IndexName: aws.String(ix.name), KeySchema: schema(ix.partition, ix.srt), Projection: all,
		})
	}

	return in
}

func schema(partition, srt keyDef) []types.KeySchemaElement {
	ks := []types.KeySchemaElement{{AttributeName: aws.String(partition.name), KeyType: types.KeyTypeHash}}
	if srt.name != "" {
		ks = append(ks, types.KeySchemaElement{AttributeName: aws.String(srt.name), KeyType: types.KeyTypeRange})
	}

	return ks
}
