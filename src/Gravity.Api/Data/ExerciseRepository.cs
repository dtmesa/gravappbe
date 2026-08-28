using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Exercises are partitioned by workoutId.
///
/// Deliberate tightening: assertExerciseAccess in the Express version located an
/// exercise by id and only checked that it belonged to the caller, not that it
/// belonged to the workout named in the URL -- so
/// GET /workouts/1/exercises/99 would happily return an exercise from workout 2.
/// Everything here is scoped by workoutId, which the client always supplies.
///
/// Name uniqueness per workout, matching the Postgres @@unique([workoutId, name]),
/// is enforced via a conditional put on ExerciseNames.
/// </summary>
public class ExerciseRepository
{
	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public ExerciseRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<Exercise?> GetAsync(int workoutId, int exerciseId, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Exercises,
			Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? Exercise.FromItem(response.Item) : null;
	}

	public async Task<Exercise> RequireAsync(int workoutId, int exerciseId, CancellationToken ct = default) =>
		await GetAsync(workoutId, exerciseId, ct) ?? throw AppError.NotFound("Exercise", "EXERCISE_NOT_FOUND");

	public async Task<List<Exercise>> ListAsync(int workoutId, CancellationToken ct = default)
	{
		var items = await _db.AllAsync(new QueryRequest
		{
			TableName = Tables.Exercises,
			IndexName = Tables.Indexes.ExercisesByOrder,
			KeyConditionExpression = "workoutId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutId) },
			ScanIndexForward = true,
		}, ct);

		return items.Select(Exercise.FromItem).ToList();
	}

	/// <summary>
	/// Appends after the current highest order. Note the original computed
	/// `(last?.order ?? 0) + 1`, so the first exercise in a workout gets order 1
	/// rather than 0; that offset is preserved.
	/// </summary>
	public async Task<Exercise> CreateAsync(int workoutId, string name, CancellationToken ct = default)
	{
		var last = await _db.QueryAsync(new QueryRequest
		{
			TableName = Tables.Exercises,
			IndexName = Tables.Indexes.ExercisesByOrder,
			KeyConditionExpression = "workoutId = :w",
			ExpressionAttributeValues = new Item { [":w"] = Dyn.N(workoutId) },
			ScanIndexForward = false,
			Limit = 1,
		}, ct);

		var highest = last.Items.Count > 0 ? last.Items[0].GetInt("order") : 0;

		var exercise = new Exercise
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.Exercise, ct),
			Name = name,
			WorkoutId = workoutId,
			Order = highest + 1,
			CreatedAt = Clock.UtcNow(),
		};

		try
		{
			await _db.TransactWriteItemsAsync(new TransactWriteItemsRequest
			{
				TransactItems =
				[
					ClaimName(workoutId, name, exercise.Id),
					new TransactWriteItem { Put = new Put { TableName = Tables.Exercises, Item = exercise.ToItem() } },
				],
			}, ct);
		}
		catch (TransactionCanceledException ex)
			when (ex.CancellationReasons.Any(r => r.Code == "ConditionalCheckFailed"))
		{
			throw new AppError("Exercise name already in use", 409, "EXERCISE_NAME_TAKEN");
		}

		return exercise;
	}

	/// <summary>
	/// Renaming releases the old name claim and takes the new one atomically,
	/// so it can't go through the generic single-field update.
	/// </summary>
	public async Task<Exercise> RenameAsync(int workoutId, int exerciseId, string newName, CancellationToken ct = default)
	{
		var exercise = await RequireAsync(workoutId, exerciseId, ct);

		if (exercise.Name == newName) return exercise;

		try
		{
			await _db.TransactWriteItemsAsync(new TransactWriteItemsRequest
			{
				TransactItems =
				[
					ClaimName(workoutId, newName, exerciseId),
					new TransactWriteItem
					{
						Delete = new Delete
						{
							TableName = Tables.ExerciseNames,
							Key = new Item { ["workoutId"] = Dyn.N(workoutId), ["name"] = Dyn.S(exercise.Name) },
						},
					},
					new TransactWriteItem
					{
						Update = new Update
						{
							TableName = Tables.Exercises,
							Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
							UpdateExpression = "SET #n = :v",
							ExpressionAttributeNames = new Dictionary<string, string> { ["#n"] = "name" },
							ExpressionAttributeValues = new Item { [":v"] = Dyn.S(newName) },
						},
					},
				],
			}, ct);
		}
		catch (TransactionCanceledException ex)
			when (ex.CancellationReasons.Any(r => r.Code == "ConditionalCheckFailed"))
		{
			throw new AppError("Exercise name already in use", 409, "EXERCISE_NAME_TAKEN");
		}

		exercise.Name = newName;

		return exercise;
	}

	private static TransactWriteItem ClaimName(int workoutId, string name, int exerciseId) => new()
	{
		Put = new Put
		{
			TableName = Tables.ExerciseNames,
			Item = new Item
			{
				["workoutId"] = Dyn.N(workoutId),
				["name"] = Dyn.S(name),
				["exerciseId"] = Dyn.N(exerciseId),
			},
			ConditionExpression = "attribute_not_exists(#n)",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#n"] = "name" },
		},
	};

	public async Task<Exercise> UpdateFieldAsync(int workoutId, int exerciseId, string field, AttributeValue value, CancellationToken ct = default)
	{
		try
		{
			var response = await _db.UpdateItemAsync(new UpdateItemRequest
			{
				TableName = Tables.Exercises,
				Key = DynamoQuery.Key("workoutId", workoutId, "id", exerciseId),
				UpdateExpression = "SET #f = :v",
				ConditionExpression = "attribute_exists(id)",
				ExpressionAttributeNames = new Dictionary<string, string> { ["#f"] = field },
				ExpressionAttributeValues = new Item { [":v"] = value },
				ReturnValues = ReturnValue.ALL_NEW,
			}, ct);

			return Exercise.FromItem(response.Attributes);
		}
		catch (ConditionalCheckFailedException)
		{
			throw new AppError("Exercise not found", 404, "NOT_FOUND");
		}
	}
}
