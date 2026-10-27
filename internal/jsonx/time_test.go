package jsonx

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestMarshalAlwaysThreeFractionalDigits(t *testing.T) {
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 3, 4, 5, 6, 7, 250_000_000, time.UTC), `"2026-03-04T05:06:07.250Z"`},
		{time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), `"2026-03-04T05:06:07.000Z"`},
		{time.Date(2026, 3, 4, 5, 6, 7, 5_000_000, time.UTC), `"2026-03-04T05:06:07.005Z"`},
		// Truncated toward zero, never rounded up.
		{time.Date(2026, 3, 4, 5, 6, 7, 999_999_999, time.UTC), `"2026-03-04T05:06:07.999Z"`},
		// Converted to UTC.
		{time.Date(2026, 3, 4, 5, 6, 7, 120_000_000, time.FixedZone("x", -7*3600)), `"2026-03-04T12:06:07.120Z"`},
	}

	for _, c := range cases {
		got, err := json.Marshal(New(c.in))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != c.want {
			t.Errorf("Marshal(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseAcceptsClientAndLegacyForms(t *testing.T) {
	want := time.Date(2026, 1, 2, 3, 4, 5, 670_000_000, time.UTC)

	for _, in := range []string{
		"2026-01-02T03:04:05.670Z",
		"2026-01-02T03:04:05.67Z",
		"2026-01-02T03:04:05.6701234Z",
		"2026-01-01T19:04:05.670-08:00",
		"2026-01-02T03:04:05.670", // offset-less: read as UTC
	} {
		got, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}

		if !got.Equal(want) {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}

	if _, err := Parse("not a date"); err == nil {
		t.Error("Parse accepted garbage")
	}
}

func TestUnmarshalRejectsNonStrings(t *testing.T) {
	var ts Timestamp

	if err := json.Unmarshal([]byte(`12345`), &ts); err == nil {
		t.Error("accepted a number")
	}
}

func TestDynamoRoundTripUsesSameLayout(t *testing.T) {
	type row struct {
		At Timestamp `dynamodbav:"at"`
	}

	in := row{New(time.Date(2026, 5, 6, 7, 8, 9, 100_000_000, time.UTC))}

	item, err := attributevalue.MarshalMap(in)
	if err != nil {
		t.Fatal(err)
	}

	s, ok := item["at"].(*types.AttributeValueMemberS)
	if !ok || s.Value != "2026-05-06T07:08:09.100Z" {
		t.Fatalf("stored %#v, want S 2026-05-06T07:08:09.100Z", item["at"])
	}

	var out row
	if err := attributevalue.UnmarshalMap(item, &out); err != nil {
		t.Fatal(err)
	}

	if !out.At.Equal(in.At.Time) {
		t.Errorf("round trip = %v, want %v", out.At, in.At)
	}
}

func TestStoredStringsSortChronologically(t *testing.T) {
	a := New(time.Date(2026, 1, 1, 0, 0, 0, 90_000_000, time.UTC)).String()
	b := New(time.Date(2026, 1, 1, 0, 0, 0, 100_000_000, time.UTC)).String()

	if !(a < b) {
		t.Errorf("%s should sort before %s", a, b)
	}
}
