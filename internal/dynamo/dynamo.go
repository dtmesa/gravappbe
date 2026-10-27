// Package dynamo wraps the DynamoDB client with the table names and the
// handful of access helpers every repository needs.
package dynamo

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Item is a raw DynamoDB item.
type Item = map[string]types.AttributeValue

// DB is the client plus the resolved table names.
type DB struct {
	Client *dynamodb.Client
	T      Tables
}

// Connect builds the client once, at startup.
//
// The region is set explicitly and IMDS is disabled: Lambda supplies both the
// region and credentials as environment variables, and letting the SDK probe
// for them costs tens of milliseconds of cold start for nothing. A non-empty
// endpoint means DynamoDB Local, which accepts any static credentials -- note
// it keeps a separate database per (access key, region), so every tool that
// inspects local tables must use the same pair.
func Connect(ctx context.Context, region, endpoint, tablePrefix string) (*DB, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
		awsconfig.WithEC2IMDSClientEnableState(imds.ClientDisabled),
	}

	if endpoint != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &DB{Client: client, T: NewTables(tablePrefix)}, nil
}

// N, S and Bool build attribute values.
func N(n int) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.Itoa(n)}
}

func N64(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}

func F(f float64) types.AttributeValue {
	// Shortest round-trip form, equivalent to C#'s "R" format.
	return &types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'g', -1, 64)}
}

func S(s string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: s}
}

func Bool(b bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: b}
}

// Key builds a two-part numeric key.
func Key(partitionName string, partition int, sortName string, sort int) Item {
	return Item{partitionName: N(partition), sortName: N(sort)}
}

// Int reads a numeric attribute, returning 0 when absent -- the same default
// as DynamoExtensions.GetInt.
func Int(item Item, name string) int {
	if v, ok := item[name].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.Atoi(v.Value)
		return n
	}

	return 0
}

// QueryAll runs a query to completion, following pagination.
func (db *DB) QueryAll(ctx context.Context, in *dynamodb.QueryInput) ([]Item, error) {
	var items []Item

	p := dynamodb.NewQueryPaginator(db.Client, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, page.Items...)
	}

	return items, nil
}

// BatchGet fetches items by key in 100-key chunks, retrying whatever the
// service reports as unprocessed. Missing keys are simply absent from the
// result.
func (db *DB) BatchGet(ctx context.Context, table string, keys []Item) ([]Item, error) {
	var results []Item

	for start := 0; start < len(keys); start += 100 {
		pending := map[string]types.KeysAndAttributes{
			table: {Keys: keys[start:min(start+100, len(keys))]},
		}

		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == 5 {
				return nil, errors.New("dynamo: batch get left unprocessed keys after retries")
			}

			out, err := db.Client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: pending})
			if err != nil {
				return nil, err
			}

			results = append(results, out.Responses[table]...)
			pending = out.UnprocessedKeys

			if len(pending) > 0 {
				if err := backoff(ctx, attempt); err != nil {
					return nil, err
				}
			}
		}
	}

	return results, nil
}

// BatchDelete deletes items by key in 25-key chunks, retrying unprocessed
// writes. It is not atomic.
func (db *DB) BatchDelete(ctx context.Context, table string, keys []Item) error {
	for start := 0; start < len(keys); start += 25 {
		chunk := keys[start:min(start+25, len(keys))]

		requests := make([]types.WriteRequest, len(chunk))
		for i, key := range chunk {
			requests[i] = types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: key}}
		}

		pending := map[string][]types.WriteRequest{table: requests}

		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == 5 {
				return errors.New("dynamo: batch delete left unprocessed items after retries")
			}

			out, err := db.Client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{RequestItems: pending})
			if err != nil {
				return err
			}

			pending = out.UnprocessedItems

			if len(pending) > 0 {
				if err := backoff(ctx, attempt); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func backoff(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(50*(attempt+1)) * time.Millisecond):
		return nil
	}
}

// IsConditionFailed reports whether err is a failed condition on a
// single-item write.
func IsConditionFailed(err error) bool {
	var ccf *types.ConditionalCheckFailedException
	return errors.As(err, &ccf)
}

// CanceledReasons returns the per-item cancellation codes of a failed
// transaction, in the order the items were sent, or nil when err is not a
// cancelled transaction. Callers inspect specific positions to learn *which*
// condition failed, rather than matching on the error message.
func CanceledReasons(err error) []string {
	var tce *types.TransactionCanceledException
	if !errors.As(err, &tce) {
		return nil
	}

	codes := make([]string, len(tce.CancellationReasons))
	for i, r := range tce.CancellationReasons {
		codes[i] = aws.ToString(r.Code)
	}

	return codes
}

// ConditionFailedAt reports whether the transaction item at index i failed
// its condition.
func ConditionFailedAt(err error, i int) bool {
	reasons := CanceledReasons(err)
	return i < len(reasons) && reasons[i] == "ConditionalCheckFailed"
}

// AnyConditionFailed reports whether any item in a cancelled transaction
// failed its condition.
func AnyConditionFailed(err error) bool {
	for _, code := range CanceledReasons(err) {
		if code == "ConditionalCheckFailed" {
			return true
		}
	}

	return false
}
