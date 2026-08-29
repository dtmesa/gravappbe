using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Text;
using Microsoft.IdentityModel.Tokens;

namespace Gravity.Api.Common;

/// <summary>
/// Port of src/utils/jwt.utils.ts. Deliberately compatible with the tokens the
/// Node backend issued: same HS256 secret, same "userId" claim, same 7d expiry,
/// so tokens already sitting in the device's SecureStore keep working.
/// </summary>
public class JwtService
{
	public const string UserIdClaim = "userId";
	public const string TokenVersionClaim = "tokenVersion";

	private readonly SymmetricSecurityKey _key;

	public JwtService(string secret)
	{
		_key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(secret));
	}

	public SymmetricSecurityKey SigningKey => _key;

	public string SignToken(int userId, int tokenVersion)
	{
		var creds = new SigningCredentials(_key, SecurityAlgorithms.HmacSha256);

		// ClaimValueTypes.Integer64 makes the claim serialize as a JSON number,
		// matching what jsonwebtoken produced for { userId }.
		var claims = new[]
		{
			new Claim(UserIdClaim, userId.ToString(), ClaimValueTypes.Integer64),
			new Claim(TokenVersionClaim, tokenVersion.ToString(), ClaimValueTypes.Integer64),
		};

		var token = new JwtSecurityToken(
			claims: claims,
			expires: DateTime.UtcNow.AddDays(7),
			signingCredentials: creds);

		return new JwtSecurityTokenHandler().WriteToken(token);
	}
}

public static class ClaimsPrincipalExtensions
{
	/// <summary>
	/// Mirrors the `if (!req.user) throw Unauthorized` guard repeated in every
	/// Express handler.
	/// </summary>
	public static int UserId(this ClaimsPrincipal principal)
	{
		var raw = principal.FindFirst(JwtService.UserIdClaim)?.Value;

		if (!int.TryParse(raw, out var userId)) throw AppError.Unauthorized();

		return userId;
	}

	/// <summary>
	/// Tokens signed before this claim existed parse as 0, matching a
	/// freshly-migrated user's default TokenVersion -- so old tokens keep
	/// working until the first password change/reset bumps the stored value.
	/// </summary>
	public static int TokenVersion(this ClaimsPrincipal principal) =>
		int.TryParse(principal.FindFirst(JwtService.TokenVersionClaim)?.Value, out var v) ? v : 0;
}
