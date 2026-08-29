using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

public record EmailConfirmationCode(string Email, string CodeHash, int Attempts, long ExpiresAt);

/// <summary>
/// One outstanding pending-email confirmation per user, serving both add-email
/// and change-email (both just overwrite this row with a new target). TTL
/// expiry silently clears "pending" state -- GET /auth/me derives
/// pendingEmail purely from whether this row exists, so there's nothing else
/// to clean up anywhere.
/// </summary>
public class EmailConfirmationRepository
{
	private readonly IAmazonDynamoDB _db;

	public EmailConfirmationRepository(IAmazonDynamoDB db) => _db = db;

	public Task PutAsync(int userId, string pendingEmail, string codeHash, TimeSpan ttl, CancellationToken ct = default) =>
		_db.PutItemAsync(new PutItemRequest
		{
			TableName = Tables.EmailConfirmationCodes,
			Item = new Dictionary<string, AttributeValue>
			{
				["userId"] = Dyn.N(userId),
				["email"] = Dyn.S(EmailRepository.Normalize(pendingEmail)),
				["codeHash"] = Dyn.S(codeHash),
				["attempts"] = Dyn.N(0),
				["expiresAt"] = Dyn.N((int)DateTimeOffset.UtcNow.Add(ttl).ToUnixTimeSeconds()),
			},
		}, ct);

	public async Task<EmailConfirmationCode?> GetAsync(int userId, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.EmailConfirmationCodes,
			Key = new Dictionary<string, AttributeValue> { ["userId"] = Dyn.N(userId) },
			ConsistentRead = true,
		}, ct);

		if (!response.IsItemSet) return null;

		var expiresAt = response.Item.GetInt("expiresAt");

		// DynamoDB Local's TTL setting doesn't actually expire items (see
		// LocalTables.cs), so an expired-but-undeleted row is treated as gone.
		if (expiresAt <= DateTimeOffset.UtcNow.ToUnixTimeSeconds()) return null;

		return new EmailConfirmationCode(
			response.Item.GetString("email"),
			response.Item.GetString("codeHash"),
			response.Item.GetInt("attempts"),
			expiresAt);
	}

	public Task IncrementAttemptsAsync(int userId, CancellationToken ct = default) =>
		_db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.EmailConfirmationCodes,
			Key = new Dictionary<string, AttributeValue> { ["userId"] = Dyn.N(userId) },
			UpdateExpression = "ADD attempts :one",
			ExpressionAttributeValues = new Dictionary<string, AttributeValue> { [":one"] = Dyn.N(1) },
		}, ct);

	public Task DeleteAsync(int userId, CancellationToken ct = default) =>
		_db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.EmailConfirmationCodes,
			Key = new Dictionary<string, AttributeValue> { ["userId"] = Dyn.N(userId) },
		}, ct);
}
