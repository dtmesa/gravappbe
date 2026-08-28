using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>Partitioned by userId, with an LSI on date for the history calendar.</summary>
public class WorkoutSessionRepository
{
	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public WorkoutSessionRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<WorkoutSession?> GetAsync(int userId, int sessionId, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.WorkoutSessions,
			Key = DynamoQuery.Key("userId", userId, "id", sessionId),
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? WorkoutSession.FromItem(response.Item) : null;
	}

	public async Task<WorkoutSession> RequireAsync(int userId, int sessionId, CancellationToken ct = default) =>
		await GetAsync(userId, sessionId, ct)
			?? throw AppError.NotFound("Workout session", "WORKOUTSESSION_NOT_FOUND");

	public async Task<WorkoutSession> CreateAsync(int userId, int workoutId, DateTime? date, CancellationToken ct = default)
	{
		var session = new WorkoutSession
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.WorkoutSession, ct),
			UserId = userId,
			WorkoutId = workoutId,
			Date = date?.ToUniversalTime() ?? DateTime.UtcNow,
			CreatedAt = DateTime.UtcNow,
		};

		await _db.PutItemAsync(new PutItemRequest
		{
			TableName = Tables.WorkoutSessions,
			Item = session.ToItem(),
		}, ct);

		return session;
	}

	/// <summary>Sessions falling inside [start, end), ascending -- the history month window.</summary>
	public async Task<List<WorkoutSession>> ByDateRangeAsync(int userId, DateTime start, DateTime end, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.WorkoutSessions,
			IndexName = Tables.Indexes.SessionsByDate,
			KeyConditionExpression = "userId = :u AND #d BETWEEN :start AND :end",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#d"] = "date" },
			ExpressionAttributeValues = new Item
			{
				[":u"] = Dyn.N(userId),
				[":start"] = Dyn.Date(start),
				// BETWEEN is inclusive; the Prisma filter was `lt: end`, so step
				// back a millisecond to keep the upper bound exclusive.
				[":end"] = Dyn.Date(end.AddMilliseconds(-1)),
			},
			ScanIndexForward = true,
		}, ct);

		return items.Select(WorkoutSession.FromItem).ToList();
	}

	public async Task<WorkoutSession> UpdateDateAsync(int userId, int sessionId, DateTime date, CancellationToken ct = default)
	{
		var response = await _db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.WorkoutSessions,
			Key = DynamoQuery.Key("userId", userId, "id", sessionId),
			UpdateExpression = "SET #d = :v",
			ConditionExpression = "attribute_exists(id)",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#d"] = "date" },
			ExpressionAttributeValues = new Item { [":v"] = Dyn.Date(date) },
			ReturnValues = ReturnValue.ALL_NEW,
		}, ct);

		return WorkoutSession.FromItem(response.Attributes);
	}
}
