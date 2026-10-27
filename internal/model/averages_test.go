package model

import (
	"math"
	"math/rand/v2"
	"testing"
)

// legacyAverages is a direct port of Common/Averages.cs, operating on raw sets.
// The aggregate-based ComputeAverages must agree with it on every input.
func legacyAverages(sessionSets [][]SetSession, ex Exercise) Averages {
	type perSession struct{ vals [4]*float64 }

	mean := func(sets []SetSession, metric string) *float64 {
		sum, n := 0.0, 0

		for _, s := range sets {
			if v := s.Value(metric); v != nil {
				sum += *v
				n++
			}
		}

		if n == 0 {
			return nil
		}

		m := sum / float64(n)

		return &m
	}

	tracked := [4]bool{ex.IsWeight, ex.IsReps, ex.IsDuration, ex.IsDistance}

	var sessions []perSession

	for _, sets := range sessionSets {
		if len(sets) == 0 {
			continue
		}

		var ps perSession
		for i, metric := range Metrics {
			if tracked[i] {
				ps.vals[i] = mean(sets, metric)
			}
		}

		sessions = append(sessions, ps)
	}

	var out [4]*float64

	for i := range Metrics {
		if !tracked[i] {
			continue
		}

		total := 0.0
		for _, s := range sessions {
			if s.vals[i] != nil {
				total += *s.vals[i]
			}
		}

		r := 0.0
		if len(sessions) > 0 {
			r = total / float64(len(sessions))
		}

		out[i] = &r
	}

	return Averages{Weight: out[0], Reps: out[1], Duration: out[2], Distance: out[3]}
}

func viaAggregates(sessionSets [][]SetSession, ex Exercise) Averages {
	aggs := make([]Aggregates, len(sessionSets))
	for i, sets := range sessionSets {
		aggs[i] = AggregatesOf(sets)
	}

	return ComputeAverages(aggs, ex)
}

func weights(ws ...any) []SetSession {
	sets := make([]SetSession, len(ws))

	for i, w := range ws {
		if w != nil {
			sets[i].Weight = ptr(w.(float64))
		}
	}

	return sets
}

func approx(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}

	return math.Abs(*a-*b) < 1e-9
}

func TestAveragesFixtures(t *testing.T) {
	weightOnly := Exercise{IsWeight: true}

	cases := []struct {
		name     string
		sessions [][]SetSession
		want     float64
	}{
		// Sum of ratios, not ratio of sums: (10 + 30) / 2 = 20, not 70/3.
		{"sum of ratios", [][]SetSession{weights(10.0), weights(20.0, 40.0)}, 20},
		// A session with zero sets drops out of the denominator entirely.
		{"empty session dropped", [][]SetSession{weights(10.0), {}}, 10},
		// Sets but no weights: contributes 0 to the numerator, 1 to the denominator.
		{"all-null session counts as zero", [][]SetSession{weights(10.0), weights(nil, nil)}, 5},
		// A seeded 0 is a value and pulls the mean down.
		{"seeded zero counts", [][]SetSession{weights(10.0, 0.0)}, 5},
		// Nothing at all: 0, not null, for a tracked metric.
		{"no sessions", nil, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := viaAggregates(c.sessions, weightOnly)

			if got.Weight == nil || math.Abs(*got.Weight-c.want) > 1e-9 {
				t.Errorf("weight = %v, want %v", got.Weight, c.want)
			}

			if got.Reps != nil || got.Duration != nil || got.Distance != nil {
				t.Errorf("untracked metrics not null: %+v", got)
			}

			if legacy := legacyAverages(c.sessions, weightOnly); !approx(legacy.Weight, got.Weight) {
				t.Errorf("legacy algorithm disagrees: %v", *legacy.Weight)
			}
		})
	}
}

// Randomized equivalence against the original algorithm, across every
// combination of tracked metrics and a mix of null, zero and real values.
func TestAveragesMatchLegacyAlgorithm(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	value := func() *float64 {
		switch rng.IntN(4) {
		case 0:
			return nil
		case 1:
			return ptr(0.0)
		default:
			return ptr(float64(rng.IntN(200)) + 0.5*float64(rng.IntN(2)))
		}
	}

	intValue := func() *int {
		if v := value(); v != nil {
			return ptr(int(*v))
		}

		return nil
	}

	for trial := range 500 {
		ex := Exercise{
			IsWeight: rng.IntN(2) == 0, IsReps: rng.IntN(2) == 0,
			IsDuration: rng.IntN(2) == 0, IsDistance: rng.IntN(2) == 0,
		}

		sessions := make([][]SetSession, rng.IntN(6))
		for i := range sessions {
			sessions[i] = make([]SetSession, rng.IntN(5))
			for j := range sessions[i] {
				sessions[i][j] = SetSession{Weight: value(), Reps: intValue(), Duration: intValue(), Distance: value()}
			}
		}

		got, want := viaAggregates(sessions, ex), legacyAverages(sessions, ex)

		if !approx(got.Weight, want.Weight) || !approx(got.Reps, want.Reps) ||
			!approx(got.Duration, want.Duration) || !approx(got.Distance, want.Distance) {
			t.Fatalf("trial %d: aggregates %+v != legacy %+v", trial, got, want)
		}
	}
}

// Incremental maintenance (create, patch, delete) must land on the same
// aggregates as recomputing from the final sets.
func TestIncrementalAggregatesMatchRecompute(t *testing.T) {
	sets := []SetSession{
		{ID: 1, Weight: ptr(0.0), Reps: ptr(0)},
		{ID: 2, Weight: ptr(0.0), Reps: ptr(0)},
		{ID: 3, Weight: ptr(0.0), Reps: ptr(0)},
	}

	var a Aggregates
	n := 0
	a.SetCount = &n

	for _, s := range sets {
		*a.SetCount++
		a.Add(s, 1)
	}

	// Patch set 1 weight 0 -> 100: remove old, add new.
	a.Add(SetSession{Weight: sets[0].Weight}, -1)
	sets[0].Weight = ptr(100.0)
	a.Add(SetSession{Weight: sets[0].Weight}, 1)

	// Delete set 2.
	*a.SetCount--
	a.Add(sets[1], -1)
	sets = append(sets[:1], sets[2:]...)

	want := AggregatesOf(sets)

	if *a.SetCount != *want.SetCount || a.WeightSum != want.WeightSum || a.WeightCount != want.WeightCount ||
		a.RepsSum != want.RepsSum || a.RepsCount != want.RepsCount {
		t.Errorf("incremental %+v != recomputed %+v", a, want)
	}

	if !a.Consistent() {
		t.Error("consistent aggregates reported inconsistent")
	}

	bad := want
	bad.WeightCount = 99

	if bad.Consistent() {
		t.Error("count above setCount reported consistent")
	}
}
