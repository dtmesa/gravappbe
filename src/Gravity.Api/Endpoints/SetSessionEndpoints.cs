using System.Security.Claims;
using System.Text.Json;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>Port of src/routes/setSession.routes.ts.</summary>
public static class SetSessionEndpoints
{
	public static void MapSetSessionEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app
			.MapGroup("/workouts/{workoutId:int}/sessions/{sessionId:int}/exerciseSessions/{exerciseSessionId:int}/setSessions")
			.RequireAuthorization();

		group.MapPost("/", async (
			int sessionId,
			int exerciseSessionId,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			ExerciseRepository exercises,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(exerciseSessionId, "exerciseSessionId");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			var exerciseSession = await exerciseSessions.RequireAsync(sessionId, exerciseSessionId, ct);

			// Which metrics the new set seeds with 0 depends on the exercise.
			var exercise = await exercises.RequireAsync(exerciseSession.WorkoutId, exerciseSession.ExerciseId, ct);

			return Results.Created((string?)null, await sets.CreateAsync(exerciseSessionId, exercise, ct));
		});

		group.MapGet("/", async (
			int sessionId,
			int exerciseSessionId,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(exerciseSessionId, "exerciseSessionId");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			await exerciseSessions.RequireAsync(sessionId, exerciseSessionId, ct);

			return Results.Ok(await sets.ListAsync(exerciseSessionId, ct));
		});

		group.MapGet("/{id:int}", async (
			int sessionId,
			int exerciseSessionId,
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			await exerciseSessions.RequireAsync(sessionId, exerciseSessionId, ct);

			return Results.Ok(await sets.RequireAsync(exerciseSessionId, id, ct));
		});

		group.MapDelete("/{id:int}", async (
			int sessionId,
			int exerciseSessionId,
			int id,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			await exerciseSessions.RequireAsync(sessionId, exerciseSessionId, ct);
			await sets.RequireAsync(exerciseSessionId, id, ct);

			await sets.DeleteAsync(exerciseSessionId, id, ct);

			return Results.NoContent();
		});

		group.MapPatch("/{id:int}/{field}", async (
			int sessionId,
			int exerciseSessionId,
			int id,
			string field,
			JsonElement body,
			ClaimsPrincipal principal,
			WorkoutSessionRepository sessions,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			var value = Patch.ForSetSession(field, body);

			await sessions.RequireAsync(principal.UserId(), sessionId, ct);
			await exerciseSessions.RequireAsync(sessionId, exerciseSessionId, ct);

			return Results.Ok(await sets.UpdateFieldAsync(exerciseSessionId, id, field, value, ct));
		});
	}
}
