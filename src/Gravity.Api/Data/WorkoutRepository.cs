using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Workouts are partitioned by userId, so ownership checks are a plain GetItem
/// rather than the `findFirst({ id, userId })` the Prisma version needed.
/// </summary>
public class WorkoutRepository
{
	// TransactWriteItems caps at 100 actions; the new workout consumes one.
	private const int MaxReorderableWorkouts = 99;

	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public WorkoutRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<Workout?> GetAsync(int userId, int workoutId, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Workouts,
			Key = DynamoQuery.Key("userId", userId, "id", workoutId),
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? Workout.FromItem(response.Item) : null;
	}

	/// <summary>Port of assertWorkoutAccess in src/utils/exercise.utils.ts.</summary>
	public async Task<Workout> RequireAsync(int userId, int workoutId, CancellationToken ct = default) =>
		await GetAsync(userId, workoutId, ct) ?? throw AppError.NotFound("Workout", "WORKOUT_NOT_FOUND");

	public async Task<List<Workout>> ListAsync(int userId, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.Workouts,
			IndexName = Tables.Indexes.WorkoutsByOrder,
			KeyConditionExpression = "userId = :u",
			ExpressionAttributeValues = new Item { [":u"] = Dyn.N(userId) },
			ScanIndexForward = true,
		}, ct);

		return items.Select(Workout.FromItem).ToList();
	}

	/// <summary>
	/// New workouts land at order 0 and push everything else down by one, which
	/// the Prisma version did as updateMany + create inside a transaction.
	/// </summary>
	public async Task<Workout> CreateAsync(int userId, string name, CancellationToken ct = default)
	{
		var existing = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.Workouts,
			KeyConditionExpression = "userId = :u",
			ExpressionAttributeValues = new Item { [":u"] = Dyn.N(userId) },
			ProjectionExpression = "userId, id",
		}, ct);

		if (existing.Count > MaxReorderableWorkouts)
			throw new AppError("Too many workouts to reorder in one transaction", 409, "TOO_MANY_WORKOUTS");

		var workout = new Workout
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.Workout, ct),
			Name = name,
			UserId = userId,
			Order = 0,
			CreatedAt = Clock.UtcNow(),
		};

		var actions = existing.Select(item => new TransactWriteItem
		{
			Update = new Update
			{
				TableName = Tables.Workouts,
				Key = DynamoQuery.Key("userId", item.GetInt("userId"), "id", item.GetInt("id")),
				UpdateExpression = "SET #o = #o + :one",
				ExpressionAttributeNames = new Dictionary<string, string> { ["#o"] = "order" },
				ExpressionAttributeValues = new Item { [":one"] = Dyn.N(1) },
			},
		}).ToList();

		actions.Add(new TransactWriteItem
		{
			Put = new Put { TableName = Tables.Workouts, Item = workout.ToItem() },
		});

		await _db.TransactWriteItemsAsync(new TransactWriteItemsRequest { TransactItems = actions }, ct);

		return workout;
	}

	/// <summary>
	/// Mirrors Prisma's `update`, which threw P2025 -> 404 NOT_FOUND when the
	/// row was missing. Note the delete route used a different code.
	/// </summary>
	public async Task<Workout> UpdateFieldAsync(int userId, int workoutId, string field, AttributeValue value, CancellationToken ct = default)
	{
		try
		{
			var response = await _db.UpdateItemAsync(new UpdateItemRequest
			{
				TableName = Tables.Workouts,
				Key = DynamoQuery.Key("userId", userId, "id", workoutId),
				UpdateExpression = "SET #f = :v",
				ConditionExpression = "attribute_exists(id)",
				ExpressionAttributeNames = new Dictionary<string, string> { ["#f"] = field },
				ExpressionAttributeValues = new Item { [":v"] = value },
				ReturnValues = ReturnValue.ALL_NEW,
			}, ct);

			return Workout.FromItem(response.Attributes);
		}
		catch (ConditionalCheckFailedException)
		{
			throw new AppError("Workout not found", 404, "NOT_FOUND");
		}
	}
}
