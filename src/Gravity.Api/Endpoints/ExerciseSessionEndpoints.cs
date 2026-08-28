using System.Security.Claims;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>
/// Port of src/routes/exerciseSession.routes.ts.
///
/// Deliberate tightening: the original resolved the exercise by id across every
/// workout the caller owned, so an exercise from workout A could be logged into
/// a session of workout B. Here it is resolved within the workout named in the
/// URL, which is what the client always does.
/// </summary>
public static class ExerciseSessionEndpoints
{
	public static void MapExerciseSessionEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app
			.MapGroup("/workouts/{workoutId:int}/sessions/{sessionId:int}/exerciseSessions")
			.RequireAuthorization();

		group.MapPost("/", async (
			int workoutId,
			int sessionId,
			CreateExerciseSessionRequest? body,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseRepository exercises,
			ExerciseSessionRepository exerciseSessions,
			CancellationToken ct) =>
		{
			Validate.PositiveId(sessionId, "sessionId");
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();

			var session = await sessions.RequireAsync(userId, sessionId, ct);
			await exercises.RequireAsync(workoutId, request.ExerciseId, ct);

			var created = await exerciseSessions.CreateAsync(session, request.ExerciseId, ct);
			created.Sets = [];

			return Results.Created((string?)null, created);
		});

		group.MapGet("/", async (
			int sessionId,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(sessionId, "sessionId");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);

			var items = await exerciseSessions.ListAsync(sessionId, ct);

			await Task.WhenAll(items.Select(async item => item.Sets = await sets.ListAsync(item.Id, ct)));

			return Results.Ok(items);
		});

		group.MapGet("/{id:int}", async (
			int sessionId,
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);

			var item = await exerciseSessions.RequireAsync(sessionId, id, ct);
			item.Sets = await sets.ListAsync(id, ct);

			return Results.Ok(item);
		});

		group.MapDelete("/{id:int}", async (
			int sessionId,
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			CascadeService cascade,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			await exerciseSessions.RequireAsync(sessionId, id, ct);

			await cascade.DeleteExerciseSessionAsync(sessionId, id, ct);

			return Results.NoContent();
		});

		group.MapGet("/{id:int}/previous-set-count", async (
			int sessionId,
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");
			var userId = principal.UserId();

			await sessions.RequireAsync(userId, sessionId, ct);
			var current = await exerciseSessions.RequireAsync(sessionId, id, ct);

			var previous = await exerciseSessions.PreviousAsync(
				current.ExerciseId, current.WorkoutId, userId, id, ct);

			// The original defaulted to 1, not 0, when nothing preceded this.
			var count = previous is null ? 1 : (await sets.ListAsync(previous.Id, ct)).Count;

			return Results.Ok(new { count });
		});
	}
}
