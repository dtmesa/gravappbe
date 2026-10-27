package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/dtmesa/gravappbe/internal/jsonx"
)

func ptr[T any](v T) *T { return &v }

// A fresh set's tracked metrics are 0. They must be stored (not omitted as
// "empty") and serialized (not dropped), or the client hides the input.
func TestZeroMetricsSurviveStorageAndJSON(t *testing.T) {
	set := SetSession{ID: 1, Weight: ptr(0.0), Reps: ptr(0), ExerciseSessionID: 2,
		CreatedAt: jsonx.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))}

	item, err := attributevalue.MarshalMap(set)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := item["weight"].(*types.AttributeValueMemberN); !ok {
		t.Errorf("weight 0 was not stored: %#v", item["weight"])
	}

	if _, ok := item["reps"].(*types.AttributeValueMemberN); !ok {
		t.Errorf("reps 0 was not stored: %#v", item["reps"])
	}

	if _, present := item["duration"]; present {
		t.Errorf("nil duration was stored: %#v", item["duration"])
	}

	var back SetSession
	if err := attributevalue.UnmarshalMap(item, &back); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(back)

	const want = `{"id":1,"weight":0,"reps":0,"duration":null,"distance":null,"exerciseSessionId":2,"createdAt":"2026-01-01T00:00:00.000Z"}`
	if string(body) != want {
		t.Errorf("json = %s\nwant   %s", body, want)
	}
}

func TestNullDescriptionIsExplicit(t *testing.T) {
	body, _ := json.Marshal(Workout{ID: 1, Name: "Push"})

	if !strings.Contains(string(body), `"description":null`) {
		t.Errorf("description not explicit null: %s", body)
	}

	item, _ := attributevalue.MarshalMap(Workout{ID: 1, Name: "Push", Description: ptr("")})
	if _, ok := item["description"].(*types.AttributeValueMemberS); !ok {
		t.Errorf("empty description should still be stored: %#v", item["description"])
	}
}

func TestSetsAlwaysAnArray(t *testing.T) {
	body, _ := json.Marshal(ExerciseSession{ID: 1, Sets: []SetSession{}})

	if !strings.Contains(string(body), `"sets":[]`) {
		t.Errorf("sets not an empty array: %s", body)
	}

	// Denormalized and aggregate fields never reach the wire.
	for _, hidden := range []string{"userId", "sessionDate", "setCount", "weightSum"} {
		if strings.Contains(string(body), hidden) {
			t.Errorf("%s leaked into JSON: %s", hidden, body)
		}
	}
}

func TestWorkoutOmittedFromSessionWhenAbsent(t *testing.T) {
	body, _ := json.Marshal(WorkoutSession{ID: 1})

	if strings.Contains(string(body), `"workout"`) {
		t.Errorf("absent workout serialized: %s", body)
	}
}

func TestAggregatesPresenceDistinguishesLegacyRows(t *testing.T) {
	item, _ := attributevalue.MarshalMap(ExerciseSession{ID: 1, Aggregates: Aggregates{SetCount: ptr(0)}})

	if _, ok := item["setCount"].(*types.AttributeValueMemberN); !ok {
		t.Fatalf("setCount 0 not stored: %#v", item["setCount"])
	}

	delete(item, "setCount")

	var legacy ExerciseSession
	if err := attributevalue.UnmarshalMap(item, &legacy); err != nil {
		t.Fatal(err)
	}

	if legacy.HasAggregates() {
		t.Error("row without setCount reported aggregates")
	}
}

func TestAveragesAlwaysHasFourKeys(t *testing.T) {
	body, _ := json.Marshal(Averages{Weight: ptr(1.5)})

	if string(body) != `{"weight":1.5,"reps":null,"duration":null,"distance":null}` {
		t.Errorf("averages = %s", body)
	}
}
