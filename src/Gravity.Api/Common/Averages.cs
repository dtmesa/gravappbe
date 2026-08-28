using Gravity.Api.Models;

namespace Gravity.Api.Common;

/// <summary>
/// Verbatim port of calculateAverages in src/utils/exercise.utils.ts, including
/// its two quirks: sessions with no sets are dropped before averaging, and the
/// JS `|| 0` fallback turns an empty result (a NaN division) into 0 rather than
/// null. Metrics the exercise does not track stay null.
/// </summary>
public static class Averages
{
	public record Result(double? Weight, double? Reps, double? Duration, double? Distance);

	public static Result Calculate(IEnumerable<List<SetSession>> sessionSets, Exercise exercise)
	{
		var perSession = sessionSets
			.Where(sets => sets.Count > 0)
			.Select(sets => new
			{
				Weight = exercise.IsWeight ? Mean(sets.Select(s => s.Weight)) : null,
				Reps = exercise.IsReps ? Mean(sets.Select(s => (double?)s.Reps)) : null,
				Duration = exercise.IsDuration ? Mean(sets.Select(s => (double?)s.Duration)) : null,
				Distance = exercise.IsDistance ? Mean(sets.Select(s => s.Distance)) : null,
			})
			.ToList();

		var count = perSession.Count;

		return new Result(
			exercise.IsWeight ? Overall(perSession.Select(s => s.Weight), count) : null,
			exercise.IsReps ? Overall(perSession.Select(s => s.Reps), count) : null,
			exercise.IsDuration ? Overall(perSession.Select(s => s.Duration), count) : null,
			exercise.IsDistance ? Overall(perSession.Select(s => s.Distance), count) : null);
	}

	/// <summary>Average across one session's sets, ignoring nulls.</summary>
	private static double? Mean(IEnumerable<double?> values)
	{
		var present = values.Where(v => v.HasValue).Select(v => v!.Value).ToList();

		return present.Count > 0 ? present.Sum() / present.Count : null;
	}

	/// <summary>Average of the per-session averages; nulls count as 0, as in the original.</summary>
	private static double Overall(IEnumerable<double?> sessionAverages, int count) =>
		count > 0 ? sessionAverages.Sum(v => v ?? 0) / count : 0;
}
