// Package ratelimit is a fixed-window counter backed by DynamoDB, so limits
// hold across concurrent Lambda instances rather than per process.
package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/dtmesa/gravappbe/internal/config"
	"github.com/dtmesa/gravappbe/internal/dynamo"
	"github.com/dtmesa/gravappbe/internal/httpx"
)

type Limiter struct {
	db      *dynamo.DB
	now     func() time.Time
	enabled bool
}

func New(db *dynamo.DB, now func() time.Time, enabled bool) *Limiter {
	return &Limiter{db: db, now: now, enabled: enabled}
}

// Check counts one request against policy for identifier. Each
// policy+identifier+window is its own item, incremented atomically with ADD,
// which expires through the table's TTL -- nothing ever cleans up.
func (l *Limiter) Check(ctx context.Context, policy config.Policy, identifier string) (allowed bool, retryAfter int, err error) {
	if !l.enabled {
		return true, 0, nil
	}

	window := int64(policy.WindowSeconds)
	now := l.now().Unix()
	bucket := now / window
	windowEnd := (bucket + 1) * window

	out, err := l.db.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(l.db.T.RateLimits),
		Key: dynamo.Item{
			"key": dynamo.S(policy.Name + "#" + identifier + "#" + strconv.FormatInt(bucket, 10)),
		},
		UpdateExpression:         aws.String("ADD #c :one SET #ttl = if_not_exists(#ttl, :ttl)"),
		ExpressionAttributeNames: map[string]string{"#c": "count", "#ttl": "expiresAt"},
		ExpressionAttributeValues: dynamo.Item{
			":one": dynamo.N(1),
			// Slack past the window end, so an item near the boundary never
			// vanishes mid-check.
			":ttl": dynamo.N64(windowEnd + window),
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return false, 0, err
	}

	return dynamo.Int(out.Attributes, "count") <= policy.MaxRequests, int(windowEnd - now), nil
}

// Ensure is Check that fails with 429 RATE_LIMITED.
func (l *Limiter) Ensure(ctx context.Context, policy config.Policy, identifier string) error {
	allowed, retryAfter, err := l.Check(ctx, policy, identifier)
	if err != nil {
		return err
	}

	if !allowed {
		return httpx.TooManyRequests(retryAfter)
	}

	return nil
}
