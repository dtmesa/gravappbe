using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Replaces Prisma's `onDelete: Cascade`. DynamoDB has no referential cascade,
/// so the fan-out is explicit.
///
/// Every method removes the entity itself (and releases its name claim, where
/// it has one) BEFORE touching its children. This makes a concurrent read or
/// create against that entity see it as gone immediately and consistently,
/// rather than possibly observing a parent that still exists with some
/// children already removed.
///
/// Known limitation: unlike the Postgres version this is not atomic, so a
/// mid-cascade failure can leave orphaned children. Parent-first ordering also
/// means a retry is NOT a safe way to resume an interrupted cascade -- the
/// endpoint's own existence check on the parent will now 404 before the retry
/// ever reaches this service, even though children may remain. Accepted
/// trade-off: immediate read consistency was judged more valuable than
/// resumability for a single-user app; orphaned children age out silently
/// rather than causing visible harm.
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
		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.ExerciseSessions,
			Key = DynamoQuery.Key("workoutSessionId", workoutSessionId, "id", exerciseSessionId),
		}, ct);

		await DeleteSetsAsync([exerciseSessionId], ct);
	}

	/// <summary>Deletes one workout session, its exercise sessions and their sets.</summary>
	public async Task DeleteWorkoutSessionAsync(int userId, int workoutSessionId, CancellationToken ct = default)
	{
		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.WorkoutSessions,
			Key = DynamoQuery.Key("userId", userId, "id", workoutSessionId),
		}, ct);

		var exerciseSessions = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			KeyConditionExpression = "workoutSessionId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutSessionId) },
			ProjectionExpression = "workoutSessionId, id",
		}, ct);

		await DeleteSetsAsync(exerciseSessions.Select(e => e.GetInt("id")), ct);
		await _db.DeleteAllAsync(Tables.ExerciseSessions, exerciseSessions, ct);
	}

	/// <summary>Deletes one exercise plus every exercise session recorded against it.</summary>
	public async Task DeleteExerciseAsync(int workoutId, int exerciseId, CancellationToken ct = default)
	{
		// Releasing the name claim reads the row, so it has to happen before
		// the row itself is removed -- but both still land before any children.
		await ReleaseExerciseNameAsync(workoutId, exerciseId, ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Exercises,
			Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
		}, ct);

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
	}

	/// <summary>Frees the exercise's claimed name so a new exercise can reuse it.</summary>
	private async Task ReleaseExerciseNameAsync(int workoutId, int exerciseId, CancellationToken ct)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Exercises,
			Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
			ProjectionExpression = "#n",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#n"] = "name" },
		}, ct);

		if (!response.IsItemSet) return;

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.ExerciseNames,
			Key = new Item { ["workoutId"] = Dyn.N(workoutId), ["name"] = response.Item["name"] },
		}, ct);
	}

	/// <summary>Deletes a workout, its exercises, its sessions, and everything beneath them.</summary>
	public async Task DeleteWorkoutAsync(int userId, int workoutId, CancellationToken ct = default)
	{
		await ReleaseWorkoutNameAsync(userId, workoutId, ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Workouts,
			Key = DynamoQuery.Key("userId", userId, "id", workoutId),
		}, ct);

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
	}

	/// <summary>Frees the workout's claimed name so a new workout can reuse it.</summary>
	private async Task ReleaseWorkoutNameAsync(int userId, int workoutId, CancellationToken ct)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Workouts,
			Key = DynamoQuery.Key("userId", userId, "id", workoutId),
			ProjectionExpression = "#n",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#n"] = "name" },
		}, ct);

		if (!response.IsItemSet) return;

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.WorkoutNames,
			Key = new Item { ["userId"] = Dyn.N(userId), ["name"] = response.Item["name"] },
		}, ct);
	}

	/// <summary>Deletes an account's data. The User row itself is removed by the caller first.</summary>
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
