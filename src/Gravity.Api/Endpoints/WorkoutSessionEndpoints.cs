using System.Security.Claims;
using System.Text.Json;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>
/// Port of src/routes/workoutSession.routes.ts.
///
/// Like the original, the reads here scope by session id + userId and do not
/// require the session to belong to the workout named in the URL. The {workoutId:int}
/// route constraint alone is what the client's History screen had been violating
/// by sending "undefined" in that segment.
/// </summary>
public static class WorkoutSessionEndpoints
{
	public static void MapWorkoutSessionEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/workouts/{workoutId:int}/sessions").RequireAuthorization();

		group.MapPost("/", async (
			int workoutId,
			CreateWorkoutSessionRequest? body,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			WorkoutSessionRepository sessions,
			CancellationToken ct) =>
		{
			Validate.PositiveId(workoutId, "workoutId");
			var userId = principal.UserId();

			var workout = await workouts.RequireAsync(userId, workoutId, ct);

			var session = await sessions.CreateAsync(userId, workoutId, body?.Date, ct);
			session.Workout = workout;

			return Results.Created((string?)null, session);
		});

		group.MapGet("/{id:int}", async (
			int id,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			WorkoutSessionRepository sessions,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");
			var userId = principal.UserId();

			var session = await sessions.RequireAsync(userId, id, ct);
			session.Workout = await workouts.GetAsync(userId, session.WorkoutId, ct);

			return Results.Ok(session);
		});

		group.MapDelete("/{id:int}", async (
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			CascadeService cascade,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");
			var userId = principal.UserId();

			await sessions.RequireAsync(userId, id, ct);
			await cascade.DeleteWorkoutSessionAsync(userId, id, ct);

			return Results.NoContent();
		});

		group.MapPatch("/{id:int}/{field}", async (
			int id,
			string field,
			JsonElement body,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			// "date" is the only patchable field on a session.
			if (field != "date")
				throw new FluentValidation.ValidationException($"Unsupported field '{field}'");

			// The Zod schema made `date` optional and the handler then rejected a
			// missing value with INVALID_BODY rather than VALIDATION_ERROR.
			if (body.ValueKind != JsonValueKind.Object || !body.TryGetProperty("date", out _))
				throw new AppError("Missing value for field", 400, "INVALID_BODY");

			var date = Patch.Date(body, "date");
			var userId = principal.UserId();

			await sessions.RequireAsync(userId, id, ct);

			var updated = await sessions.UpdateDateAsync(userId, id, date, ct);

			// Child exercise sessions carry a copy of the session date for the
			// averages index, so they have to move with it.
			await exerciseSessions.SyncSessionDateAsync(id, date, ct);

			return Results.Ok(updated);
		});
	}
}
