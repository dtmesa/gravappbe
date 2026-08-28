using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Replaces Prisma's `onDelete: Cascade`. DynamoDB has no referential cascade,
/// so the fan-out is explicit.
///
/// Known limitation: unlike the Postgres version these deletions are not atomic,
/// so a mid-cascade failure can leave orphans. Children are always removed
/// before their parent, which makes a retry of the same call safe.
/// </summary>
public class CascadeService
{
	private readonly IAmazonDynamoDB _db;

	public CascadeService(IAmazonDynamoDB db) => _db = db;

	/// <summary>Deletes every set belonging to the given exercise sessions.</summary>
	public async Task DeleteSetsAsync(IEnumerable<int> exerciseSessionIds, CancellationToken ct = default)
	{
		foreach (var exerciseSessionId in exerciseSessionIds)
		{
			var sets = await _db.AllAsync(new QueryRequest
			{
				TableName = Tables.SetSessions,
				KeyConditionExpression = "exerciseSessionId = :e",
				ExpressionAttributeValues = new Item { [":e"] = Dyn.N(exerciseSessionId) },
				ProjectionExpression = "exerciseSessionId, id",
			}, ct);

			await _db.DeleteAllAsync(Tables.SetSessions, sets, ct);
		}
	}

	/// <summary>Deletes one exercise session and its sets.</summary>
	public async Task DeleteExerciseSessionAsync(int workoutSessionId, int exerciseSessionId, CancellationToken ct = default)
	{
		await DeleteSetsAsync([exerciseSessionId], ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.ExerciseSessions,
			Key = DynamoQuery.Key("workoutSessionId", workoutSessionId, "id", exerciseSessionId),
		}, ct);
	}

	/// <summary>Deletes one workout session, its exercise sessions and their sets.</summary>
	public async Task DeleteWorkoutSessionAsync(int userId, int workoutSessionId, CancellationToken ct = default)
	{
		var exerciseSessions = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			KeyConditionExpression = "workoutSessionId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutSessionId) },
			ProjectionExpression = "workoutSessionId, id",
		}, ct);

		await DeleteSetsAsync(exerciseSessions.Select(e => e.GetInt("id")), ct);
		await _db.DeleteAllAsync(Tables.ExerciseSessions, exerciseSessions, ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.WorkoutSessions,
			Key = DynamoQuery.Key("userId", userId, "id", workoutSessionId),
		}, ct);
	}

	/// <summary>Deletes one exercise plus every exercise session recorded against it.</summary>
	public async Task DeleteExerciseAsync(int workoutId, int exerciseId, CancellationToken ct = default)
	{
		var exerciseSessions = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			IndexName = Tables.Indexes.ExerciseSessionsByExercise,
			KeyConditionExpression = "exerciseId = :e",
			ExpressionAttributeValues = new Item { [":e"] = Dyn.N(exerciseId) },
			ProjectionExpression = "workoutSessionId, id",
		}, ct);

		await DeleteSetsAsync(exerciseSessions.Select(e => e.GetInt("id")), ct);
		await _db.DeleteAllAsync(Tables.ExerciseSessions, exerciseSessions, ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Exercises,
			Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
		}, ct);
	}

	/// <summary>Deletes a workout, its exercises, its sessions, and everything beneath them.</summary>
	public async Task DeleteWorkoutAsync(int userId, int workoutId, CancellationToken ct = default)
	{
		var sessions = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.WorkoutSessions,
			IndexName = Tables.Indexes.SessionsByWorkout,
			KeyConditionExpression = "workoutId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutId) },
			ProjectionExpression = "userId, id",
		}, ct);

		foreach (var session in sessions)
			await DeleteWorkoutSessionAsync(session.GetInt("userId"), session.GetInt("id"), ct);

		var exercises = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.Exercises,
			KeyConditionExpression = "workoutId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutId) },
			ProjectionExpression = "workoutId, id",
		}, ct);

		// An exercise can be logged into a session belonging to a different
		// workout, so sweep by exerciseId rather than relying on the pass above.
		foreach (var exercise in exercises)
			await DeleteExerciseAsync(workoutId, exercise.GetInt("id"), ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Workouts,
			Key = DynamoQuery.Key("userId", userId, "id", workoutId),
		}, ct);
	}

	/// <summary>Deletes an account and every record hanging off it.</summary>
	public async Task DeleteUserDataAsync(int userId, CancellationToken ct = default)
	{
		var workouts = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.Workouts,
			KeyConditionExpression = "userId = :u",
			ExpressionAttributeValues = new Item { [":u"] = Dyn.N(userId) },
			ProjectionExpression = "userId, id",
		}, ct);

		foreach (var workout in workouts)
			await DeleteWorkoutAsync(userId, workout.GetInt("id"), ct);

		// Sessions whose workout was already removed would be missed above.
		var sessions = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.WorkoutSessions,
			KeyConditionExpression = "userId = :u",
			ExpressionAttributeValues = new Item { [":u"] = Dyn.N(userId) },
			ProjectionExpression = "userId, id",
		}, ct);

		foreach (var session in sessions)
			await DeleteWorkoutSessionAsync(userId, session.GetInt("id"), ct);
	}
}
