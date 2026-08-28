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

	private readonly SymmetricSecurityKey _key;

	public JwtService(string secret)
	{
		_key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(secret));
	}

	public SymmetricSecurityKey SigningKey => _key;

	public string SignToken(int userId)
	{
		var creds = new SigningCredentials(_key, SecurityAlgorithms.HmacSha256);

		// ClaimValueTypes.Integer64 makes the claim serialize as a JSON number,
		// matching what jsonwebtoken produced for { userId }.
		var token = new JwtSecurityToken(
			claims: [new Claim(UserIdClaim, userId.ToString(), ClaimValueTypes.Integer64)],
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
}
