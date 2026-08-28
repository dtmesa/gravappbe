using System.Security.Claims;
using System.Text.Json;
using FluentValidation;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>Port of src/routes/exercise.routes.ts.</summary>
public static class ExerciseEndpoints
{
	public static void MapExerciseEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/workouts/{workoutId:int}/exercises").RequireAuthorization();

		group.MapPost("/", async (
			int workoutId,
			CreateNamedRequest? body,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			CancellationToken ct) =>
		{
			Validate.PositiveId(workoutId, "workoutId");
			var request = Validate.Check(Validate.Required(body));

			await workouts.RequireAsync(principal.UserId(), workoutId, ct);

			return Results.Created((string?)null, await exercises.CreateAsync(workoutId, request.Name, ct));
		});

		group.MapGet("/", async (
			int workoutId,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			CancellationToken ct) =>
		{
			Validate.PositiveId(workoutId, "workoutId");

			await workouts.RequireAsync(principal.UserId(), workoutId, ct);

			return Results.Ok(await exercises.ListAsync(workoutId, ct));
		});

		group.MapGet("/{id:int}", async (
			int workoutId,
			int id,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await workouts.RequireAsync(principal.UserId(), workoutId, ct);

			return Results.Ok(await exercises.RequireAsync(workoutId, id, ct));
		});

		group.MapDelete("/{id:int}", async (
			int workoutId,
			int id,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			CascadeService cascade,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			await workouts.RequireAsync(principal.UserId(), workoutId, ct);
			await exercises.RequireAsync(workoutId, id, ct);

			await cascade.DeleteExerciseAsync(workoutId, id, ct);

			return Results.NoContent();
		});

		group.MapPatch("/{id:int}/{field}", async (
			int workoutId,
			int id,
			string field,
			JsonElement body,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			var value = Patch.ForExercise(field, body);

			await workouts.RequireAsync(principal.UserId(), workoutId, ct);

			return Results.Ok(await exercises.UpdateFieldAsync(workoutId, id, field, value, ct));
		});

		// Averages over the last seven days.
		group.MapGet("/{id:int}/averages", (
			int workoutId,
			int id,
			int? excludeSessionId,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
			AveragesAsync(workoutId, id, excludeSessionId, DateTime.UtcNow.AddDays(-7),
				principal, workouts, exercises, exerciseSessions, sets, ct));

		// Averages over every recorded session.
		group.MapGet("/{id:int}/averages/all", (
			int workoutId,
			int id,
			int? excludeSessionId,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			ExerciseRepository exercises,
			ExerciseSessionRepository exerciseSessions,
			SetSessionRepository sets,
			CancellationToken ct) =>
			AveragesAsync(workoutId, id, excludeSessionId, null,
				principal, workouts, exercises, exerciseSessions, sets, ct));
	}

	private static async Task<IResult> AveragesAsync(
		int workoutId,
		int exerciseId,
		int? excludeSessionId,
		DateTime? since,
		ClaimsPrincipal principal,
		WorkoutRepository workouts,
		ExerciseRepository exercises,
		ExerciseSessionRepository exerciseSessions,
		SetSessionRepository sets,
		CancellationToken ct)
	{
		Validate.PositiveId(exerciseId, "id");

		// excludeIdSchema made this query param required, not optional.
		if (excludeSessionId is not > 0)
			throw Validate.Invalid("excludeSessionId", "excludeSessionId must be a positive integer");

		var userId = principal.UserId();

		await workouts.RequireAsync(userId, workoutId, ct);
		var exercise = await exercises.RequireAsync(workoutId, exerciseId, ct);

		var sessions = await exerciseSessions.ByExerciseAsync(exerciseId, userId, since, excludeSessionId.Value, ct);

		var sessionSets = await Task.WhenAll(sessions.Select(s => sets.ListAsync(s.Id, ct)));

		return Results.Ok(Averages.Calculate(sessionSets, exercise));
	}
}
