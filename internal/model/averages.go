package model

// Averages are computed from per-exercise-session aggregates rather than by
// re-reading every set, reproducing Common/Averages.cs exactly -- including
// its two JS artifacts.
//
// Per metric, the original computed:
//
//  1. per session: the mean of that metric's non-null values across the
//     session's sets (null when the session has none for that metric);
//  2. overall: the sum of those session means, with null counted as 0, divided
//     by the number of sessions that had at least one set.
//
// Two consequences shape the storage:
//
//   - It is a sum of ratios, not a ratio of sums: sum(s_i/c_i) != sum(s_i)/sum(c_i).
//     So each exercise session keeps its own sum and count, and the division
//     happens per session before summing.
//   - The denominator counts non-empty sessions, not sessions that recorded the
//     metric. A session with sets but no weights still adds 1 to the weight
//     denominator and 0 to its numerator -- the JS `|| 0` artifact.
//
// Metrics the exercise does not track are null; tracked metrics with no data
// are 0, not null.
func ComputeAverages(sessions []Aggregates, ex Exercise) Averages {
	nonEmpty := 0
	for _, s := range sessions {
		if s.Count() > 0 {
			nonEmpty++
		}
	}

	overall := func(tracked bool, sumOf func(Aggregates) (float64, int)) *float64 {
		if !tracked {
			return nil
		}

		total := 0.0

		for _, s := range sessions {
			if s.Count() == 0 {
				continue
			}

			if sum, count := sumOf(s); count > 0 {
				total += sum / float64(count)
			}
		}

		result := 0.0
		if nonEmpty > 0 {
			result = total / float64(nonEmpty)
		}

		return &result
	}

	return Averages{
		Weight:   overall(ex.IsWeight, func(a Aggregates) (float64, int) { return a.WeightSum, a.WeightCount }),
		Reps:     overall(ex.IsReps, func(a Aggregates) (float64, int) { return a.RepsSum, a.RepsCount }),
		Duration: overall(ex.IsDuration, func(a Aggregates) (float64, int) { return a.DurationSum, a.DurationCount }),
		Distance: overall(ex.IsDistance, func(a Aggregates) (float64, int) { return a.DistanceSum, a.DistanceCount }),
	}
}

// AggregatesOf computes aggregates from scratch over a session's sets. Used to
// backfill rows written before aggregates existed.
func AggregatesOf(sets []SetSession) Aggregates {
	count := len(sets)
	a := Aggregates{SetCount: &count}

	for _, set := range sets {
		a.Add(set, 1)
	}

	return a
}

// Add folds one set into the aggregates with the given sign (+1 to add, -1 to
// remove). It does not touch SetCount. Zero is a value, not an absence: a
// freshly seeded set's 0 weight counts toward the mean, as it always did.
func (a *Aggregates) Add(set SetSession, sign float64) {
	if v := set.Value(MetricWeight); v != nil {
		a.WeightSum += sign * *v
		a.WeightCount += int(sign)
	}

	if v := set.Value(MetricReps); v != nil {
		a.RepsSum += sign * *v
		a.RepsCount += int(sign)
	}

	if v := set.Value(MetricDuration); v != nil {
		a.DurationSum += sign * *v
		a.DurationCount += int(sign)
	}

	if v := set.Value(MetricDistance); v != nil {
		a.DistanceSum += sign * *v
		a.DistanceCount += int(sign)
	}
}

// Consistent reports whether the aggregates are internally possible. A
// negative count or a per-metric count above the set count means a write was
// lost between a set update and its aggregate update; the caller recomputes.
func (a Aggregates) Consistent() bool {
	n := a.Count()

	for _, c := range []int{a.WeightCount, a.RepsCount, a.DurationCount, a.DistanceCount} {
		if c < 0 || c > n {
			return false
		}
	}

	return n >= 0
}
