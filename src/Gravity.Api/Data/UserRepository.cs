using Amazon.DynamoDBv2;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Data;

/// <summary>
/// Username uniqueness was a Postgres `@unique` constraint surfaced as P2002 ->
/// 409 USERNAME_TAKEN. DynamoDB has no such constraint, so it is enforced with a
/// conditional write on a dedicated lookup table inside a transaction. This is
/// the one uniqueness rule the client actually branches on
/// (workout-app/src/screens/Register/RegisterScreen.tsx).
/// </summary>
public class UserRepository
{
	private readonly IAmazonDynamoDB _db;
	private readonly IdGenerator _ids;

	public UserRepository(IAmazonDynamoDB db, IdGenerator ids)
	{
		_db = db;
		_ids = ids;
	}

	public async Task<User?> GetByIdAsync(int id, CancellationToken ct = default)
	{
		var response = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Users,
			Key = new Dictionary<string, AttributeValue> { ["id"] = Dyn.N(id) },
			ConsistentRead = true,
		}, ct);

		return response.IsItemSet ? User.FromItem(response.Item) : null;
	}

	public async Task<User?> GetByUsernameAsync(string username, CancellationToken ct = default)
	{
		var lookup = await _db.GetItemAsync(new GetItemRequest
		{
			TableName = Tables.Usernames,
			Key = new Dictionary<string, AttributeValue> { ["username"] = Dyn.S(username) },
			ConsistentRead = true,
		}, ct);

		if (!lookup.IsItemSet) return null;

		return await GetByIdAsync(lookup.Item.GetInt("userId"), ct);
	}

	public async Task<User> CreateAsync(string username, string hashedPassword, CancellationToken ct = default)
	{
		var user = new User
		{
			Id = await _ids.NextAsync(IdGenerator.Entities.User, ct),
			Username = username,
			Password = hashedPassword,
			CreatedAt = Clock.UtcNow(),
		};

		await ExecuteUsernameTransactionAsync([
			ClaimUsername(username, user.Id),
			new TransactWriteItem { Put = new Put { TableName = Tables.Users, Item = user.ToItem() } },
		], ct);

		return user;
	}

	public async Task UpdateUsernameAsync(int userId, string currentUsername, string newUsername, CancellationToken ct = default)
	{
		if (currentUsername == newUsername) return;

		await ExecuteUsernameTransactionAsync([
			ClaimUsername(newUsername, userId),
			new TransactWriteItem
			{
				Delete = new Delete
				{
					TableName = Tables.Usernames,
					Key = new Dictionary<string, AttributeValue> { ["username"] = Dyn.S(currentUsername) },
				},
			},
			new TransactWriteItem
			{
				Update = new Update
				{
					TableName = Tables.Users,
					Key = new Dictionary<string, AttributeValue> { ["id"] = Dyn.N(userId) },
					UpdateExpression = "SET username = :u",
					ExpressionAttributeValues = new Dictionary<string, AttributeValue> { [":u"] = Dyn.S(newUsername) },
				},
			},
		], ct);
	}

	/// <summary>
	/// Bumps TokenVersion alongside the password so every other issued token is
	/// invalidated (see JwtBearerEvents.OnTokenValidated in Program.cs) -- a
	/// password change is exactly as security-sensitive as a forgotten-password
	/// reset, so both go through this one method. Returns the new version so the
	/// caller can reissue a token for its own session; otherwise the caller's
	/// own token would go stale on its very next request.
	/// </summary>
	public async Task<int> UpdatePasswordAsync(int userId, string hashedPassword, CancellationToken ct = default)
	{
		var response = await _db.UpdateItemAsync(new UpdateItemRequest
		{
			TableName = Tables.Users,
			Key = new Dictionary<string, AttributeValue> { ["id"] = Dyn.N(userId) },
			UpdateExpression = "SET password = :p ADD tokenVersion :one",
			ExpressionAttributeValues = new Dictionary<string, AttributeValue>
			{
				[":p"] = Dyn.S(hashedPassword),
				[":one"] = Dyn.N(1),
			},
			ReturnValues = ReturnValue.UPDATED_NEW,
		}, ct);

		return response.Attributes.GetInt("tokenVersion");
	}

	/// <summary>Removes the user record and releases the username and, if confirmed, the email.</summary>
	public async Task DeleteAsync(int userId, string username, string? email, CancellationToken ct = default)
	{
		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Users,
			Key = new Dictionary<string, AttributeValue> { ["id"] = Dyn.N(userId) },
		}, ct);

		await _db.DeleteItemAsync(new DeleteItemRequest
		{
			TableName = Tables.Usernames,
			Key = new Dictionary<string, AttributeValue> { ["username"] = Dyn.S(username) },
		}, ct);

		if (email is not null)
		{
			await _db.DeleteItemAsync(new DeleteItemRequest
			{
				TableName = Tables.Emails,
				Key = new Dictionary<string, AttributeValue> { ["email"] = Dyn.S(EmailRepository.Normalize(email)) },
			}, ct);
		}
	}

	private static TransactWriteItem ClaimUsername(string username, int userId) => new()
	{
		Put = new Put
		{
			TableName = Tables.Usernames,
			Item = new Dictionary<string, AttributeValue>
			{
				["username"] = Dyn.S(username),
				["userId"] = Dyn.N(userId),
			},
			ConditionExpression = "attribute_not_exists(username)",
		},
	};

	private async Task ExecuteUsernameTransactionAsync(List<TransactWriteItem> items, CancellationToken ct)
	{
		try
		{
			await _db.TransactWriteItemsAsync(new TransactWriteItemsRequest { TransactItems = items }, ct);
		}
		catch (TransactionCanceledException ex)
			when (ex.CancellationReasons.Any(r => r.Code == "ConditionalCheckFailed"))
		{
			throw new AppError("Username already taken", 409, "USERNAME_TAKEN");
		}
	}
}
