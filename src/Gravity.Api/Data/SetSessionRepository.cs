using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Sets are partitioned by exerciseSessionId with id as the sort key, which
/// gives the `orderBy: { id: "asc" }` the Express route asked for at no cost.
/// </summary>
public class SetSessionRepository
{
	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public SetSessionRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<List<SetSession>> ListAsync(int exerciseSessionId, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.SetSessions,
			KeyConditionExpression = "exerciseSessionId = :e",
			ExpressionAttributeValues = new Item { [":e"] = Dyn.N(exerciseSessionId) },
			ScanIndexForward = true,
		}, ct);

		return items.Select(SetSession.FromItem).ToList();
	}

	public async Task<SetSession?> GetAsync(int exerciseSessionId, int id, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.SetSessions,
			Key = DynamoQuery.Key("exerciseSessionId", exerciseSessionId, "id", id),
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? SetSession.FromItem(response.Item) : null;
	}

	public async Task<SetSession> RequireAsync(int exerciseSessionId, int id, CancellationToken ct = default) =>
		await GetAsync(exerciseSessionId, id, ct) ?? throw AppError.NotFound("Set session", "SETSESSION_NOT_FOUND");

	/// <summary>
	/// A new set is seeded with 0 for whichever metrics the exercise tracks and
	/// null for the rest, matching the create in src/routes/setSession.routes.ts.
	/// </summary>
	public async Task<SetSession> CreateAsync(int exerciseSessionId, Exercise exercise, CancellationToken ct = default)
	{
		var set = new SetSession
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.SetSession, ct),
			ExerciseSessionId = exerciseSessionId,
			Weight = exercise.IsWeight ? 0 : null,
			Reps = exercise.IsReps ? 0 : null,
			Duration = exercise.IsDuration ? 0 : null,
			Distance = exercise.IsDistance ? 0 : null,
			CreatedAt = DateTime.UtcNow,
		};

		await _db.PutItemAsync(new PutItemRequest
		{
			TableName = Tables.SetSessions,
			Item = set.ToItem(),
		}, ct);

		return set;
	}

	public async Task<SetSession> UpdateFieldAsync(int exerciseSessionId, int id, string field, AttributeValue value, CancellationToken ct = default)
	{
		try
		{
			var response = await _db.UpdateItemAsync(new UpdateItemRequest
			{
				TableName = Tables.SetSessions,
				Key = DynamoQuery.Key("exerciseSessionId", exerciseSessionId, "id", id),
				UpdateExpression = "SET #f = :v",
				ConditionExpression = "attribute_exists(id)",
				ExpressionAttributeNames = new Dictionary<string, string> { ["#f"] = field },
				ExpressionAttributeValues = new Item { [":v"] = value },
				ReturnValues = ReturnValue.ALL_NEW,
			}, ct);

			return SetSession.FromItem(response.Attributes);
		}
		catch (ConditionalCheckFailedException)
		{
			throw new AppError("Set session not found", 404, "NOT_FOUND");
		}
	}

	public Task DeleteAsync(int exerciseSessionId, int id, CancellationToken ct = default) =>
		_db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.SetSessions,
			Key = DynamoQuery.Key("exerciseSessionId", exerciseSessionId, "id", id),
		}, ct);
}
