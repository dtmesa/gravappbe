using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Data;

namespace Gravity.Api.Common;

public record RateLimitResult(bool Allowed, int RetryAfterSeconds);

/// <summary>
/// Fixed-window counter backed by DynamoDB so limits hold across concurrent
/// Lambda instances, not just within one process. Each policy+identifier+window
/// gets its own item, atomically incremented via UpdateItem's ADD, and expires
/// on its own through the table's TTL attribute -- nothing to clean up.
/// </summary>
public class RateLimiter
{
	private readonly IAmazonDynamoDB _db;

	public RateLimiter(IAmazonDynamoDB db) => _db = db;

	public async Task<RateLimitResult> CheckAsync(RateLimitPolicy policy, string identifier, CancellationToken ct)
	{
		var now = new DateTimeOffset(Clock.UtcNow()).ToUnixTimeSeconds();
		var window = policy.WindowSeconds;
		var bucket = now / window;
		var windowEnd = (bucket + 1) * window;
		var key = $"{policy.Name}#{identifier}#{bucket}";

		var response = await _db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.RateLimits,
			Key = new Dictionary<string, AttributeValue> { ["key"] = Dyn.S(key) },
			UpdateExpression = "ADD #c :one SET #ttl = if_not_exists(#ttl, :ttl)",
			ExpressionAttributeNames = new Dictionary<string, string> { ["#c"] = "count", ["#ttl"] = "expiresAt" },
			ExpressionAttributeValues = new Dictionary<string, AttributeValue>
			{
				[":one"] = Dyn.N(1),
				// Slack past the window end so an in-flight request near the
				// boundary never has its counter item vanish mid-check.
				[":ttl"] = Dyn.N((int)(windowEnd + window)),
			},
			ReturnValues = ReturnValue.UPDATED_NEW,
		}, ct);

		var count = response.Attributes.GetInt("count");

		return new RateLimitResult(count <= policy.MaxRequests, (int)(windowEnd - now));
	}

	/// <summary>
	/// Convenience for the account-recovery endpoints, which check a policy a
	/// second time keyed by the *target* email (in addition to the per-IP check
	/// .RequireRateLimit() already does) -- so an attacker rotating IPs can't
	/// spam one victim's inbox.
	/// </summary>
	public async Task EnsureAllowedAsync(RateLimitPolicy policy, string identifier, CancellationToken ct)
	{
		var result = await CheckAsync(policy, identifier, ct);

		if (!result.Allowed) throw AppError.TooManyRequests(result.RetryAfterSeconds);
	}
}
