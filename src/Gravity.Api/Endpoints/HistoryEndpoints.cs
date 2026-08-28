using System.Globalization;
using System.Security.Claims;
using System.Text.RegularExpressions;
using Amazon.DynamoDBv2;
using FluentValidation;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;

namespace Gravity.Api.Endpoints;

/// <summary>
/// Port of src/routes/history.routes.ts. Prisma served this as one query with
/// nested includes; multi-table DynamoDB needs a fan-out instead: sessions for
/// the month, then their exercise sessions and sets in parallel, then two batch
/// reads to resolve workout and exercise names.
/// </summary>
public static partial class HistoryEndpoints
{
	[GeneratedRegex(@"^\d{4}-(0[1-9]|1[0-2])$")]
	private static partial Regex MonthPattern();

	private record NamedRef(string Name);

	private record HistoryExerciseSession(
		int Id,
		int Order,
		int WorkoutSessionId,
		int ExerciseId,
		DateTime CreatedAt,
		NamedRef Exercise,
		List<SetSession> Sets);

	private record HistorySession(
		int Id,
		DateTime Date,
		int UserId,
		int WorkoutId,
		DateTime CreatedAt,
		NamedRef Workout,
		List<HistoryExerciseSession> Exercises);

	public static void MapHistoryEndpoints(this IEndpointRouteBuilder app)
	{
		app.MapGet("/history/sessions", async (
			string? month,
			ClaimsPrincipal principal,
			IAmazonDynamoDB db,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			var (start, end) = ParseMonth(month);
			var userId = principal.UserId();

			var monthSessions = await sessions.ByDateRangeAsync(userId, start, end, ct);

			if (monthSessions.Count == 0) return Results.Ok(Array.Empty<HistorySession>());

			// Exercise sessions per workout session, then sets per exercise session.
			var perSession = await Task.WhenAll(monthSessions.Select(async session =>
			{
				var items = await exerciseSessions.ListAsync(session.Id, ct);

				var withSets = await Task.WhenAll(items.Select(async item =>
					(Item: item, Sets: await sets.ListAsync(item.Id, ct))));

				return (Session: session, Exercises: withSets);
			}));

			var workoutNames = await ResolveWorkoutNamesAsync(db, userId, monthSessions, ct);

			var exerciseNames = await ResolveExerciseNamesAsync(
				db,
				perSession.SelectMany(p => p.Exercises.Select(e => (e.Item.WorkoutId, e.Item.ExerciseId))),
				ct);

			var response = perSession.Select(entry => new HistorySession(
				entry.Session.Id,
				entry.Session.Date,
				entry.Session.UserId,
				entry.Session.WorkoutId,
				entry.Session.CreatedAt,
				new NamedRef(workoutNames.GetValueOrDefault(entry.Session.WorkoutId, string.Empty)),
				entry.Exercises.Select(e => new HistoryExerciseSession(
					e.Item.Id,
					e.Item.Order,
					e.Item.WorkoutSessionId,
					e.Item.ExerciseId,
					e.Item.CreatedAt,
					new NamedRef(exerciseNames.GetValueOrDefault(e.Item.ExerciseId, string.Empty)),
					e.Sets)).ToList()));

			return Results.Ok(response);
		}).RequireAuthorization();
	}

	/// <summary>
	/// Reproduces queryMonthSchema, including its use of local-time month
	/// boundaries. Switching these to UTC would silently shift which sessions
	/// land in a given month for anyone not on UTC.
	/// </summary>
	private static (DateTime Start, DateTime End) ParseMonth(string? month)
	{
		if (month is null || !MonthPattern().IsMatch(month))
			throw new ValidationException("month must be formatted YYYY-MM");

		var year = int.Parse(month[..4], CultureInfo.InvariantCulture);
		var monthNumber = int.Parse(month[5..], CultureInfo.InvariantCulture);

		var start = new DateTime(year, monthNumber, 1, 0, 0, 0, DateTimeKind.Local);

		return (start, start.AddMonths(1));
	}

	private static async Task<Dictionary<int, string>> ResolveWorkoutNamesAsync(
		IAmazonDynamoDB db,
		int userId,
		List<WorkoutSession> sessions,
		CancellationToken ct)
	{
		var keys = sessions
			.Select(s => s.WorkoutId)
			.Distinct()
			.Select(workoutId => DynamoQuery.Key("userId", userId, "id", workoutId))
			.ToList();

		var items = await db.BatchGetAsync(Tables.Workouts, keys, ct);

		return items.ToDictionary(item => item.GetInt("id"), item => item.GetString("name"));
	}

	private static async Task<Dictionary<int, string>> ResolveExerciseNamesAsync(
		IAmazonDynamoDB db,
		IEnumerable<(int WorkoutId, int ExerciseId)> references,
		CancellationToken ct)
	{
		var keys = references
			.Distinct()
			.Select(r => DynamoQuery.Key("workoutId", r.WorkoutId, "id", r.ExerciseId))
			.ToList();

		if (keys.Count == 0) return [];

		var items = await db.BatchGetAsync(Tables.Exercises, keys, ct);

		return items.ToDictionary(item => item.GetInt("id"), item => item.GetString("name"));
	}
}
