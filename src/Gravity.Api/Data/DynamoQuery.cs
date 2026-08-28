using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

using Item = Dictionary<string, AttributeValue>;

public static class DynamoQuery
{
	/// <summary>Runs a query to completion, following pagination.</summary>
	public static async Task<List<Item>> AllAsync(this IAmazonDynamoDB db, QueryRequest request, CancellationToken ct = default)
	{
		var results = new List<Item>();
		Item? lastKey = null;

		do
		{
			request.ExclusiveStartKey = lastKey;

			var response = await db.QueryAsync(request, ct);

			results.AddRange(response.Items);
			lastKey = response.LastEvaluatedKey is { Count: > 0 } ? response.LastEvaluatedKey : null;
		}
		while (lastKey is not null);

		return results;
	}

	/// <summary>
	/// Deletes items in BatchWriteItem-sized chunks, retrying whatever the
	/// service reports as unprocessed.
	/// </summary>
	public static async Task DeleteAllAsync(this IAmazonDynamoDB db, string table, IEnumerable<Item> keys, CancellationToken ct = default)
	{
		foreach (var chunk in keys.Chunk(25))
		{
			var requests = chunk
				.Select(key => new WriteRequest { DeleteRequest = new DeleteRequest { Key = key } })
				.ToList();

			var pending = new Dictionary<string, List<WriteRequest>> { [table] = requests };

			for (var attempt = 0; attempt < 5 && pending.Count > 0; attempt++)
			{
				var response = await db.BatchWriteItemAsync(new BatchWriteItemRequest { RequestItems = pending }, ct);

				pending = response.UnprocessedItems?
					.Where(kv => kv.Value.Count > 0)
					.ToDictionary(kv => kv.Key, kv => kv.Value) ?? [];

				if (pending.Count > 0) await Task.Delay(50 * (attempt + 1), ct);
			}
		}
	}

	/// <summary>
	/// Fetches many items by key in 100-key batches, retrying unprocessed keys.
	/// Used to resolve workout and exercise names for the history response
	/// without denormalizing them onto every session.
	/// </summary>
	public static async Task<List<Item>> BatchGetAsync(this IAmazonDynamoDB db, string table, List<Item> keys, CancellationToken ct = default)
	{
		var results = new List<Item>();

		foreach (var chunk in keys.Chunk(100))
		{
			var pending = new Dictionary<string, KeysAndAttributes>
			{
				[table] = new() { Keys = chunk.ToList() },
			};

			for (var attempt = 0; attempt < 5 && pending.Count > 0; attempt++)
			{
				var response = await db.BatchGetItemAsync(new BatchGetItemRequest { RequestItems = pending }, ct);

				if (response.Responses.TryGetValue(table, out var items)) results.AddRange(items);

				pending = response.UnprocessedKeys?
					.Where(kv => kv.Value.Keys.Count > 0)
					.ToDictionary(kv => kv.Key, kv => kv.Value) ?? [];

				if (pending.Count > 0) await Task.Delay(50 * (attempt + 1), ct);
			}
		}

		return results;
	}

	public static Item Key(string partitionName, int partitionValue, string sortName, int sortValue) => new()
	{
		[partitionName] = Dyn.N(partitionValue),
		[sortName] = Dyn.N(sortValue),
	};
}
