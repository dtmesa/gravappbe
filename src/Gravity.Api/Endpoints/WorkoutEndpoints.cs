using System.Security.Claims;
using System.Text.Json;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>Port of src/routes/workout.routes.ts.</summary>
public static class WorkoutEndpoints
{
	public static void MapWorkoutEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/workouts").RequireAuthorization();

		group.MapPost("/", async (
			CreateNamedRequest? body,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));

			var workout = await workouts.CreateAsync(principal.UserId(), request.Name, ct);

			return Results.Created((string?)null, workout);
		});

		group.MapGet("/", async (ClaimsPrincipal principal, WorkoutRepository workouts, CancellationToken ct) =>
			Results.Ok(await workouts.ListAsync(principal.UserId(), ct)));

		group.MapGet("/{id:int}", async (
			int id,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			return Results.Ok(await workouts.RequireAsync(principal.UserId(), id, ct));
		});

		group.MapDelete("/{id:int}", async (
			int id,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			CascadeService cascade,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");
			var userId = principal.UserId();

			// deleteMany returning 0 rows was the 404 signal in the Express version.
			await workouts.RequireAsync(userId, id, ct);
			await cascade.DeleteWorkoutAsync(userId, id, ct);

			return Results.NoContent();
		});

		group.MapPatch("/{id:int}/{field}", async (
			int id,
			string field,
			JsonElement body,
			ClaimsPrincipal principal,
			WorkoutRepository workouts,
			CancellationToken ct) =>
		{
			Validate.PositiveId(id, "id");

			var value = Patch.ForWorkout(field, body);

			return Results.Ok(await workouts.UpdateFieldAsync(principal.UserId(), id, field, value, ct));
		});
	}
}
