package store

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
)

// Email uniqueness mirrors username uniqueness: a conditional put on the Emails
// lookup table inside a transaction. Only confirmed emails are ever written
// there -- a pending target lives solely on EmailConfirmationCodes until
// confirmed, so a typo'd or contested pending email can never block anyone
// else from claiming it.

// UserByEmail resolves a confirmed email to its user.
func (s *Store) UserByEmail(ctx context.Context, email string) (*model.User, error) {
	var claim struct {
		UserID int `dynamodbav:"userId"`
	}

	found, err := s.get(ctx, s.db.T.Emails, dynamo.Item{"email": dynamo.S(NormalizeEmail(email))}, true, &claim)
	if !found || err != nil {
		return nil, err
	}

	return s.User(ctx, claim.UserID, true)
}

// ConfirmEmail claims newEmail, releases currentEmail, stamps the confirmed
// email onto the user and clears the pending row, atomically. Reaching
// EMAIL_TAKEN requires having already proven mailbox ownership via the code,
// so it is not an enumeration leak.
//
// Callers must not pass newEmail equal to currentEmail: the transaction would
// put and delete the same key, which DynamoDB rejects outright.
func (s *Store) ConfirmEmail(ctx context.Context, userID int, currentEmail *string, newEmail string) error {
	items := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.db.T.Emails),
			Item:                dynamo.Item{"email": dynamo.S(NormalizeEmail(newEmail)), "userId": dynamo.N(userID)},
			ConditionExpression: aws.String("attribute_not_exists(email)"),
		}},
		{Update: &types.Update{
			TableName:                 aws.String(s.db.T.Users),
			Key:                       dynamo.Item{"id": dynamo.N(userID)},
			UpdateExpression:          aws.String("SET email = :e, emailConfirmed = :t"),
			ExpressionAttributeValues: dynamo.Item{":e": dynamo.S(newEmail), ":t": dynamo.Bool(true)},
		}},
		{Delete: &types.Delete{
			TableName: aws.String(s.db.T.EmailConfirmationCodes),
			Key:       dynamo.Item{"userId": dynamo.N(userID)},
		}},
	}

	if currentEmail != nil {
		items = append(items, types.TransactWriteItem{Delete: &types.Delete{
			TableName: aws.String(s.db.T.Emails),
			Key:       dynamo.Item{"email": dynamo.S(NormalizeEmail(*currentEmail))},
		}})
	}

	_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if dynamo.AnyConditionFailed(err) {
		return httpx.New(http.StatusConflict, "EMAIL_TAKEN", "Email already in use")
	}

	return err
}

// Code is an outstanding one-time code: a password reset (keyed by email) or
// a pending email confirmation (keyed by user).
type Code struct {
	UserID    int    `dynamodbav:"userId"`
	Email     string `dynamodbav:"email"`
	CodeHash  string `dynamodbav:"codeHash"`
	Attempts  int    `dynamodbav:"attempts"`
	ExpiresAt int64  `dynamodbav:"expiresAt"`
}

// CodeTable is one of the two code tables. Each holds at most one live code
// per key: a new code overwrites the row, which rotates out any earlier one.
type CodeTable struct {
	s     *Store
	table string
}

func (s *Store) ResetCodes() CodeTable        { return CodeTable{s, s.db.T.PasswordResetCodes} }
func (s *Store) ConfirmationCodes() CodeTable { return CodeTable{s, s.db.T.EmailConfirmationCodes} }

// ResetKey and ConfirmationKey build each table's key.
func ResetKey(email string) dynamo.Item      { return dynamo.Item{"email": dynamo.S(NormalizeEmail(email))} }
func ConfirmationKey(userID int) dynamo.Item { return dynamo.Item{"userId": dynamo.N(userID)} }

// Put stores a code. The email is stored normalized. expiresAt is Unix seconds
// as int64 -- the C# service cast it to int, which overflows in 2038.
func (c CodeTable) Put(ctx context.Context, userID int, email, codeHash string, ttl time.Duration) error {
	return c.s.put(ctx, c.table, Code{
		UserID:    userID,
		Email:     NormalizeEmail(email),
		CodeHash:  codeHash,
		ExpiresAt: c.s.now().Add(ttl).Unix(),
	})
}

// Get returns the live code, or nil. Expiry is checked here as well as by TTL,
// because DynamoDB's TTL deletion is a background sweep that can lag by up to
// 48 hours, so an expired row can still be read.
func (c CodeTable) Get(ctx context.Context, key dynamo.Item) (*Code, error) {
	var code Code

	found, err := c.s.get(ctx, c.table, key, true, &code)
	if !found || err != nil {
		return nil, err
	}

	if code.ExpiresAt <= c.s.now().Unix() {
		return nil, nil
	}

	return &code, nil
}

// IncrementAttempts counts a wrong guess. The condition stops ADD from
// recreating a stub row (key + attempts only) when the code was deleted in the
// meantime; that case is simply ignored.
func (c CodeTable) IncrementAttempts(ctx context.Context, key dynamo.Item) error {
	_, err := c.s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(c.table),
		Key:                       key,
		UpdateExpression:          aws.String("ADD attempts :one"),
		ConditionExpression:       aws.String("attribute_exists(codeHash)"),
		ExpressionAttributeValues: dynamo.Item{":one": dynamo.N(1)},
	})
	if dynamo.IsConditionFailed(err) {
		return nil
	}

	return err
}

func (c CodeTable) Delete(ctx context.Context, key dynamo.Item) error {
	return c.s.deleteKey(ctx, c.table, key)
}
