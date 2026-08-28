using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

/// <summary>
/// Replaces Postgres autoincrement. The frontend types every id as `number`
/// (workout-app/src/types/*.ts), so ids must stay small sequential ints rather
/// than becoming GUIDs.
/// </summary>
public class IdGenerator
{
	private readonly IAmazonDynamoDB _db;

	public IdGenerator(IAmazonDynamoDB db) => _db = db;

	public async Task<int> NextAsync(string entity, CancellationToken ct = default)
	{
		var response = await _db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.Counters,
			Key = new Dictionary<string, AttributeValue> { ["entity"] = Dyn.S(entity) },
			UpdateExpression = "ADD #n :one",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#n"] = "value" },
			ExpressionAttributeValues = new Dictionary<string, AttributeValue> { [":one"] = Dyn.N(1) },
			ReturnValues = ReturnValue.UPDATED_NEW,
		}, ct);

		return response.Attributes.GetInt("value");
	}

	public static class Entities
	{
		public const string User = "User";
		public const string Workout = "Workout";
		public const string Exercise = "Exercise";
		public const string WorkoutSession = "WorkoutSession";
		public const string ExerciseSession = "ExerciseSession";
		public const string SetSession = "SetSession";
	}
}
