using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Partitioned by workoutSessionId. Items carry a denormalized copy of the
/// parent session's userId, workoutId and date, because /averages and
/// /previous-set-count query across sessions by exerciseId -- a relational join
/// in Prisma, and a table scan here without the GSI those fields feed.
/// </summary>
public class ExerciseSessionRepository
{
	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public ExerciseSessionRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<List<ExerciseSession>> ListAsync(int workoutSessionId, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			KeyConditionExpression = "workoutSessionId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutSessionId) },
		}, ct);

		return items.Select(ExerciseSession.FromItem).ToList();
	}

	public async Task<ExerciseSession?> GetAsync(int workoutSessionId, int id, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.ExerciseSessions,
			Key = DynamoQuery.Key("workoutSessionId", workoutSessionId, "id", id),
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? ExerciseSession.FromItem(response.Item) : null;
	}

	public async Task<ExerciseSession> RequireAsync(int workoutSessionId, int id, CancellationToken ct = default) =>
		await GetAsync(workoutSessionId, id, ct)
			?? throw AppError.NotFound("Exercise session", "EXERCISESESSION_NOT_FOUND");

	public async Task<ExerciseSession> CreateAsync(WorkoutSession session, int exerciseId, CancellationToken ct = default)
	{
		var exerciseSession = new ExerciseSession
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.ExerciseSession, ct),
			// The Prisma create never set `order`, so it always defaulted to 0.
			Order = 0,
			WorkoutSessionId = session.Id,
			ExerciseId = exerciseId,
			CreatedAt = Clock.UtcNow(),
			UserId = session.UserId,
			WorkoutId = session.WorkoutId,
			SessionDate = session.Date,
		};

		await _db.PutItemAsync(new PutItemRequest
		{
			TableName = Tables.ExerciseSessions,
			Item = exerciseSession.ToItem(),
		}, ct);

		return exerciseSession;
	}

	/// <summary>
	/// Backs /averages and /averages/all. `since` is null for the lifetime
	/// variant. excludeWorkoutSessionId drops the session in progress, matching
	/// `workoutSession: { id: { not: excludeSessionId } }`.
	/// </summary>
	public async Task<List<ExerciseSession>> ByExerciseAsync(
		int exerciseId,
		int userId,
		DateTime? since,
		int excludeWorkoutSessionId,
		CancellationToken ct = default)
	{
		var values = new Item
		{
			[":e"] = Dyn.N(exerciseId),
			[":u"] = Dyn.N(userId),
			[":exclude"] = Dyn.N(excludeWorkoutSessionId),
		};

		var keyCondition = "exerciseId = :e";

		if (since.HasValue)
		{
			keyCondition += " AND sessionDate >= :since";
			values[":since"] = Dyn.Date(since.Value);
		}

		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			IndexName = Tables.Indexes.ExerciseSessionsByExercise,
			KeyConditionExpression = keyCondition,
			FilterExpression = "userId = :u AND workoutSessionId <> :exclude",
			ExpressionAttributeValues = values,
		}, ct);

		return items.Select(ExerciseSession.FromItem).ToList();
	}

	/// <summary>
	/// The most recent logging of this exercise within the same workout,
	/// excluding the current exercise session. Ordered by the parent session's
	/// date descending, as the Prisma `orderBy: { workoutSession: { date } }` did.
	/// </summary>
	public async Task<ExerciseSession?> PreviousAsync(
		int exerciseId,
		int workoutId,
		int userId,
		int excludeExerciseSessionId,
		CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			IndexName = Tables.Indexes.ExerciseSessionsByExercise,
			KeyConditionExpression = "exerciseId = :e",
			FilterExpression = "workoutId = :w AND userId = :u AND id <> :exclude",
			ExpressionAttributeValues = new Item
			{
				[":e"] = Dyn.N(exerciseId),
				[":w"] = Dyn.N(workoutId),
				[":u"] = Dyn.N(userId),
				[":exclude"] = Dyn.N(excludeExerciseSessionId),
			},
			ScanIndexForward = false,
		}, ct);

		return items.Count > 0 ? ExerciseSession.FromItem(items[0]) : null;
	}

	/// <summary>
	/// Keeps the denormalized sessionDate in step when PATCH /sessions/:id/date
	/// moves the parent session.
	/// </summary>
	public async Task SyncSessionDateAsync(int workoutSessionId, DateTime date, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.ExerciseSessions,
			KeyConditionExpression = "workoutSessionId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutSessionId) },
			ProjectionExpression = "workoutSessionId, id",
		}, ct);

		await Task.WhenAll(items.Select(item => _db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.ExerciseSessions,
			Key = DynamoQuery.Key("workoutSessionId", workoutSessionId, "id", item.GetInt("id")),
			UpdateExpression = "SET sessionDate = :d",
			ExpressionAttributeValues = new Item { [":d"] = Dyn.Date(date) },
		}, ct)));
	}
}
