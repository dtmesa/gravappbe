using System.Security.Cryptography;
using System.Text;

namespace Gravity.Api.Common;

/// <summary>
/// Shared by password-reset and email-confirmation codes. SHA-256 rather than
/// BCrypt is deliberate: these are single-use, short-TTL, low-entropy numeric
/// codes checked on every attempt -- BCrypt's slowness adds latency, not
/// security, here. The real brute-force defense is the per-row attempt cap
/// plus rate limiting.
/// </summary>
public static class CodeGenerator
{
	public static string GenerateSixDigitCode() =>
		RandomNumberGenerator.GetInt32(0, 1_000_000).ToString("D6");

	public static string Hash(string code) =>
		Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(code)));
}
