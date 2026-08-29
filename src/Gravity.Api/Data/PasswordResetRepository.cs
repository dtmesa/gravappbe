using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;

namespace Gravity.Api.Data;

public record PasswordResetCode(int UserId, string CodeHash, int Attempts, long ExpiresAt);

/// <summary>
/// One active reset code per email at a time -- a new request just overwrites
/// the row, which naturally rotates/invalidates any previously issued code.
/// Not a uniqueness claim, so no transaction needed.
/// </summary>
public class PasswordResetRepository
{
	private readonly IAmazonDynamoDB _db;

	public PasswordResetRepository(IAmazonDynamoDB db) => _db = db;

	public Task PutAsync(string email, int userId, string codeHash, TimeSpan ttl, CancellationToken ct = default) =>
		_db.PutItemAsync(new PutItemRequest
		{
			TableName = Tables.PasswordResetCodes,
			Item = new Dictionary<string, AttributeValue>
			{
				["email"] = Dyn.S(EmailRepository.Normalize(email)),
				["userId"] = Dyn.N(userId),
				["codeHash"] = Dyn.S(codeHash),
				["attempts"] = Dyn.N(0),
				["expiresAt"] = Dyn.N((int)DateTimeOffset.UtcNow.Add(ttl).ToUnixTimeSeconds()),
			},
		}, ct);

	public async Task<PasswordResetCode?> GetAsync(string email, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.PasswordResetCodes,
			Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(EmailRepository.Normalize(email)) },
			ConsistentRead = true,
		}, ct);

		if (!response.IsItemSet) return null;

		var expiresAt = response.Item.GetInt("expiresAt");

		// DynamoDB Local's TTL setting doesn't actually expire items (see
		// LocalTables.cs), so an expired-but-undeleted row is treated as gone.
		if (expiresAt <= DateTimeOffset.UtcNow.ToUnixTimeSeconds()) return null;

		return new PasswordResetCode(
			response.Item.GetInt("userId"),
			response.Item.GetString("codeHash"),
			response.Item.GetInt("attempts"),
			expiresAt);
	}

	public Task IncrementAttemptsAsync(string email, CancellationToken ct = default) =>
		_db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.PasswordResetCodes,
			Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(EmailRepository.Normalize(email)) },
			UpdateExpression = "ADD attempts :one",
			ExpressionAttributeValues = new Dictionary<string, AttributeValue> { [":one"] = Dyn.N(1) },
		}, ct);

	public Task DeleteAsync(string email, CancellationToken ct = default) =>
		_db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.PasswordResetCodes,
			Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(EmailRepository.Normalize(email)) },
		}, ct);
}
