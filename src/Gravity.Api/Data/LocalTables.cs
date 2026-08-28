using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

/// <summary>
/// Creates the tables against DynamoDB Local so `dotnet run` needs no manual
/// setup. Deployed environments get their tables from template.yaml instead;
/// this only runs when DYNAMODB_ENDPOINT is set.
/// </summary>
public static class LocalTables
{
	private const string N = "N";
	private const string S = "S";

	public static async Task EnsureCreatedAsync(IAmazonDynamoDB db, ILogger logger)
	{
		var existing = (await db.ListTablesAsync()).TableNames.ToHashSet();

		foreach (var request in Definitions())
		{
			if (existing.Contains(request.TableName)) continue;

			await db.CreateTableAsync(request);
			logger.LogInformation("Created local table {Table}", request.TableName);
		}
	}

	private static IEnumerable<CreateTableRequest> Definitions()
	{
		yield return Table(Tables.Users, ("id", N));

		yield return Table(Tables.Usernames, ("username", S));

		yield return Table(Tables.Counters, ("entity", S));

		// Lookup tables enforcing name uniqueness per parent, same pattern as
		// Usernames: a conditional put on (parent, name) claims the name.
		yield return Table(Tables.WorkoutNames, ("userId", N), ("name", S));
		yield return Table(Tables.ExerciseNames, ("workoutId", N), ("name", S));

		yield return Table(Tables.Workouts, ("userId", N), ("id", N), local:
		[
			Index(Tables.Indexes.WorkoutsByOrder, "userId", N, "order", N),
		]);

		yield return Table(Tables.Exercises, ("workoutId", N), ("id", N), local:
		[
			Index(Tables.Indexes.ExercisesByOrder, "workoutId", N, "order", N),
		]);

		yield return Table(Tables.WorkoutSessions, ("userId", N), ("id", N),
			local: [Index(Tables.Indexes.SessionsByDate, "userId", N, "date", S)],
			global: [Index(Tables.Indexes.SessionsByWorkout, "workoutId", N, "date", S)]);

		yield return Table(Tables.ExerciseSessions, ("workoutSessionId", N), ("id", N),
			global: [Index(Tables.Indexes.ExerciseSessionsByExercise, "exerciseId", N, "sessionDate", S)]);

		yield return Table(Tables.SetSessions, ("exerciseSessionId", N), ("id", N));
	}

	private record IndexSpec(string Name, string PartitionKey, string PartitionType, string SortKey, string SortType);

	private static IndexSpec Index(string name, string pk, string pkType, string sk, string skType) =>
		new(name, pk, pkType, sk, skType);

	private static CreateTableRequest Table(
		string name,
		(string Name, string Type) partition,
		(string Name, string Type)? sort = null,
		IndexSpec[]? local = null,
		IndexSpec[]? global = null)
	{
		var attributes = new Dictionary<string, string> { [partition.Name] = partition.Type };

		if (sort is not null) attributes[sort.Value.Name] = sort.Value.Type;

		foreach (var index in (local ?? []).Concat(global ?? []))
		{
			attributes[index.PartitionKey] = index.PartitionType;
			attributes[index.SortKey] = index.SortType;
		}

		var request = new CreateTableRequest
		{
			TableName = name,
			BillingMode = BillingMode.PAY_PER_REQUEST,
			AttributeDefinitions = attributes
				.Select(a => new AttributeDefinition(a.Key, a.Value))
				.ToList(),
			KeySchema = Schema(partition.Name, sort?.Name),
		};

		if (local is { Length: > 0 })
		{
			request.LocalSecondaryIndexes = local.Select(i => new LocalSecondaryIndex
			{
				IndexName = i.Name,
				KeySchema = Schema(i.PartitionKey, i.SortKey),
				Projection = new Projection { ProjectionType = ProjectionType.ALL },
			}).ToList();
		}

		if (global is { Length: > 0 })
		{
			request.GlobalSecondaryIndexes = global.Select(i => new GlobalSecondaryIndex
			{
				IndexName = i.Name,
				KeySchema = Schema(i.PartitionKey, i.SortKey),
				Projection = new Projection { ProjectionType = ProjectionType.ALL },
			}).ToList();
		}

		return request;
	}

	private static List<KeySchemaElement> Schema(string partitionKey, string? sortKey)
	{
		var schema = new List<KeySchemaElement> { new(partitionKey, KeyType.HASH) };

		if (sortKey is not null) schema.Add(new KeySchemaElement(sortKey, KeyType.RANGE));

		return schema;
	}
}
