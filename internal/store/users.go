package store

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
	"github.com/dtmesa/gravappbe/internal/model"
)

// Username uniqueness was a Postgres @unique constraint. DynamoDB has none, so
// it is enforced by a conditional write on the Usernames lookup table inside
// the same transaction that creates or renames the user.

var errUsernameTaken = httpx.New(http.StatusConflict, "USERNAME_TAKEN", "Username already taken")

// NormalizeEmail is trim + lowercase, matching EmailRepository.Normalize
// (ToLowerInvariant, not full Unicode case folding).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// User returns the user, or nil when absent.
func (s *Store) User(ctx context.Context, id int, consistent bool) (*model.User, error) {
	var u model.User

	found, err := s.get(ctx, s.db.T.Users, dynamo.Item{"id": dynamo.N(id)}, consistent, &u)
	if !found || err != nil {
		return nil, err
	}

	return &u, nil
}

// RequireUser returns the user or 404 USER_NOT_FOUND.
func (s *Store) RequireUser(ctx context.Context, id int) (model.User, error) {
	u, err := s.User(ctx, id, true)
	if err != nil {
		return model.User{}, err
	}

	if u == nil {
		return model.User{}, httpx.NotFound("User", "USER_NOT_FOUND")
	}

	return *u, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*model.User, error) {
	var claim struct {
		UserID int `dynamodbav:"userId"`
	}

	found, err := s.get(ctx, s.db.T.Usernames, dynamo.Item{"username": dynamo.S(username)}, true, &claim)
	if !found || err != nil {
		return nil, err
	}

	return s.User(ctx, claim.UserID, true)
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (model.User, error) {
	id, err := s.nextID(ctx, entityUser)
	if err != nil {
		return model.User{}, err
	}

	u := model.User{ID: id, Username: username, Password: passwordHash, CreatedAt: s.stamp()}

	item, err := attributevalue.MarshalMap(u)
	if err != nil {
		return model.User{}, err
	}

	_, err = s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimUsername(username, id),
			{Put: &types.Put{TableName: aws.String(s.db.T.Users), Item: item}},
		},
	})
	if dynamo.AnyConditionFailed(err) {
		return model.User{}, errUsernameTaken
	}

	return u, err
}

func (s *Store) RenameUser(ctx context.Context, id int, current, next string) error {
	if current == next {
		return nil
	}

	_, err := s.db.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			s.claimUsername(next, id),
			{Delete: &types.Delete{
				TableName: aws.String(s.db.T.Usernames),
				Key:       dynamo.Item{"username": dynamo.S(current)},
			}},
			{Update: &types.Update{
				TableName:                 aws.String(s.db.T.Users),
				Key:                       dynamo.Item{"id": dynamo.N(id)},
				UpdateExpression:          aws.String("SET username = :u"),
				ExpressionAttributeValues: dynamo.Item{":u": dynamo.S(next)},
			}},
		},
	})
	if dynamo.AnyConditionFailed(err) {
		return errUsernameTaken
	}

	return err
}

// UpdatePassword stores a new hash and bumps tokenVersion, which invalidates
// every token issued before it -- a password change is as security-sensitive
// as a reset, so both use this. It returns the new version so the caller can
// issue a fresh token for its own session.
//
// The attribute_exists condition is new: an unconditional UpdateItem on a
// deleted account (a reset code outliving its user) would have recreated a
// partial user row. ok is false when the user no longer exists.
func (s *Store) UpdatePassword(ctx context.Context, id int, passwordHash string) (version int, ok bool, err error) {
	out, err := s.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.db.T.Users),
		Key:                 dynamo.Item{"id": dynamo.N(id)},
		UpdateExpression:    aws.String("SET password = :p ADD tokenVersion :one"),
		ConditionExpression: aws.String("attribute_exists(id)"),
		ExpressionAttributeValues: dynamo.Item{
			":p":   dynamo.S(passwordHash),
			":one": dynamo.N(1),
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if dynamo.IsConditionFailed(err) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, err
	}

	return dynamo.Int(out.Attributes, "tokenVersion"), true, nil
}

// DeleteUser removes the user row and releases its username and confirmed
// email claims. The account's data is removed separately by DeleteUserData.
func (s *Store) DeleteUser(ctx context.Context, u model.User) error {
	if err := s.deleteKey(ctx, s.db.T.Users, dynamo.Item{"id": dynamo.N(u.ID)}); err != nil {
		return err
	}

	if err := s.deleteKey(ctx, s.db.T.Usernames, dynamo.Item{"username": dynamo.S(u.Username)}); err != nil {
		return err
	}

	if u.Email != nil {
		return s.deleteKey(ctx, s.db.T.Emails, dynamo.Item{"email": dynamo.S(NormalizeEmail(*u.Email))})
	}

	return nil
}

func (s *Store) claimUsername(username string, userID int) types.TransactWriteItem {
	return types.TransactWriteItem{Put: &types.Put{
		TableName:           aws.String(s.db.T.Usernames),
		Item:                dynamo.Item{"username": dynamo.S(username), "userId": dynamo.N(userID)},
		ConditionExpression: aws.String("attribute_not_exists(username)"),
	}}
}
