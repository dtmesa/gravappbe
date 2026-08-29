using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

/// <summary>
/// Email uniqueness mirrors UserRepository's username-uniqueness pattern
/// exactly: a conditional Put on a dedicated lookup table, claimed inside a
/// transaction. Only *confirmed* emails are ever written to Tables.Emails --
/// a pending/unconfirmed target lives solely on EmailConfirmationCodesTable
/// until confirmed, so a typo'd or contested pending email can never block
/// anyone else from eventually claiming it.
/// </summary>
public class EmailRepository
{
	private readonly IAmazonDynamoDB _db;

	public EmailRepository(IAmazonDynamoDB db) => _db = db;

	public async Task<User?> GetByEmailAsync(string email, UserRepository users, CancellationToken ct = default)
	{
		var lookup = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Emails,
			Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(Normalize(email)) },
			ConsistentRead = true,
		}, ct);

		if (!lookup.IsItemSet) return null;

		return await users.GetByIdAsync(lookup.Item.GetInt("userId"), ct);
	}

	/// <summary>
	/// Claims newEmail, releases currentEmail (if any), stamps the confirmed
	/// email onto Users, and clears the pending-confirmation row -- all
	/// atomically. Reaching EMAIL_TAKEN here requires having already proven
	/// mailbox ownership via the correct OTP, so it isn't an enumeration leak.
	/// </summary>
	public async Task ConfirmAsync(int userId, string? currentEmail, string newEmail, CancellationToken ct = default)
	{
		var items = new List<TransactWriteItem>
		{
			new()
			{
				Put = new Put
				{
					TableName = Tables.Emails,
					Item = new Dictionary<string, AttributeValue>
					{
						["email"] = Dyn.S(Normalize(newEmail)),
						["userId"] = Dyn.N(userId),
					},
					ConditionExpression = "attribute_not_exists(email)",
				},
			},
			new()
			{
				Update = new Update
				{
					TableName = Tables.Users,
					Key = new Dictionary<string, AttributeValue> { ["id"] = Dyn.N(userId) },
					UpdateExpression = "SET email = :e, emailConfirmed = :t",
					ExpressionAttributeValues = new Dictionary<string, AttributeValue>
					{
						[":e"] = Dyn.S(newEmail),
						[":t"] = Dyn.Bool(true),
					},
				},
			},
			new()
			{
				Delete = new Delete
				{
					TableName = Tables.EmailConfirmationCodes,
					Key = new Dictionary<string, AttributeValue> { ["userId"] = Dyn.N(userId) },
				},
			},
		};

		if (currentEmail is not null)
		{
			items.Insert(1, new TransactWriteItem
			{
				Delete = new Delete
				{
					TableName = Tables.Emails,
					Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(Normalize(currentEmail)) },
				},
			});
		}

		try
		{
			await _db.TransactWriteItemsAsync(new TransactWriteItemsRequest { TransactItems = items }, ct);
		}
		catch (TransactionCanceledException ex)
			when (ex.CancellationReasons.Any(r => r.Code == "ConditionalCheckFailed"))
		{
			throw new AppError("Email already in use", 409, "EMAIL_TAKEN");
		}
	}

	public static string Normalize(string email) => email.Trim().ToLowerInvariant();
}
