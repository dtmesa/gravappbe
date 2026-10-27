// Package jsonx holds the wire-format primitives shared by every model: the
// timestamp type the API reads and writes in both JSON and DynamoDB.
package jsonx

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Layout is the single timestamp format this API emits, in JSON and in
// DynamoDB alike.
//
// The three fractional digits are load-bearing. Go's own time.Time marshals as
// RFC 3339 with trailing zeros trimmed, so 250ms would come out as ".25Z" where
// every stored value and every previously issued response used ".250Z". Beyond
// consistency, dates are stored as DynamoDB strings and compared
// lexicographically by the date-keyed indexes, so a variable-width fraction
// breaks range queries -- the history month window's exclusive upper bound
// works by subtracting one millisecond and relying on fixed width.
//
// ".000" zero-pads to exactly three digits; ".999" would trim, which is the trap.
const Layout = "2006-01-02T15:04:05.000Z"

// Timestamp is a UTC instant at millisecond precision. time.Time is embedded so
// the usual methods (Before, Add, Sub, IsZero) stay available.
type Timestamp struct {
	time.Time
}

// New normalizes any time into the only representation the API uses: UTC,
// truncated (not rounded) to milliseconds.
//
// time.Now() carries nanoseconds, so without truncation the value returned by a
// create would carry more precision than the stored string can hold, and a
// later read of the same row would disagree with the create response.
func New(t time.Time) Timestamp {
	return Timestamp{t.UTC().Truncate(time.Millisecond)}
}

// Parse reads any timestamp the API accepts on input.
//
// A string without an offset is read as UTC rather than host-local time. The
// C# implementation was inconsistent here (its DynamoDB reader assumed UTC, its
// JSON reader assumed local); UTC is the defensible reading, and the client
// only ever sends toISOString() output, which always ends in Z.
func Parse(s string) (Timestamp, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return New(t), nil
		}
	}

	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return New(t), nil
		}
	}

	return Timestamp{}, fmt.Errorf("jsonx: %q is not a recognized timestamp", s)
}

func (t Timestamp) String() string {
	return t.UTC().Format(Layout)
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	raw := string(data)

	if raw == "null" {
		*t = Timestamp{}
		return nil
	}

	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return fmt.Errorf("jsonx: timestamp must be a string, got %s", raw)
	}

	parsed, err := Parse(strings.Trim(raw, `"`))
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// MarshalDynamoDBAttributeValue stores the timestamp as a string in the same
// layout as JSON, which is what keeps the date-keyed indexes sortable.
func (t Timestamp) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: t.String()}, nil
}

func (t *Timestamp) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	switch v := av.(type) {
	case *types.AttributeValueMemberNULL:
		*t = Timestamp{}
		return nil
	case *types.AttributeValueMemberS:
		parsed, err := Parse(v.Value)
		if err != nil {
			return err
		}

		*t = parsed

		return nil
	default:
		return fmt.Errorf("jsonx: cannot read a timestamp from %T", av)
	}
}

// Compile-time proof the SDK routes through the methods above instead of
// reflecting over the embedded time.Time, which would store a map.
var (
	_ attributevalue.Marshaler   = Timestamp{}
	_ attributevalue.Unmarshaler = (*Timestamp)(nil)
)
