namespace Gravity.Api.Common;

public record RateLimitPolicy(string Name, int MaxRequests, int WindowSeconds);

/// <summary>
/// Thresholds are read once at startup from env vars so they can be tuned per
/// environment without a code change. Auth endpoints are deliberately much
/// stricter than the general per-IP volume cap -- General guards against
/// generic API spam, the others against credential-stuffing / brute force.
/// </summary>
public class RateLimitOptions
{
	public bool Enabled { get; }

	public RateLimitPolicy General { get; }   // all routes, per IP
	public RateLimitPolicy Login { get; }     // POST /auth/login, per IP
	public RateLimitPolicy Register { get; }  // POST /auth/register, per IP
	public RateLimitPolicy AuthMutate { get; } // username/password/delete, per IP

	// Account-recovery policies: applied per IP via .RequireRateLimit(), and a
	// second time inside the handler keyed by the target email, so an attacker
	// rotating IPs can't spam one victim's inbox.
	public RateLimitPolicy ForgotUsername { get; }
	public RateLimitPolicy PasswordResetRequest { get; }
	public RateLimitPolicy PasswordResetVerify { get; }
	public RateLimitPolicy EmailMutate { get; } // add/change email, resend, confirm

	public RateLimitOptions()
	{
		Enabled = Bool("RATE_LIMIT_ENABLED", true);

		General = Policy("GENERAL", maxRequests: 300, windowSeconds: 300);   // 300 req / 5 min
		Login = Policy("LOGIN", maxRequests: 5, windowSeconds: 900);         // 5 attempts / 15 min
		Register = Policy("REGISTER", maxRequests: 5, windowSeconds: 3600);  // 5 attempts / 1 hr
		AuthMutate = Policy("AUTH_MUTATE", maxRequests: 10, windowSeconds: 900); // 10 attempts / 15 min

		ForgotUsername = Policy("FORGOT_USERNAME", maxRequests: 5, windowSeconds: 3600);           // 5 / hr
		PasswordResetRequest = Policy("PASSWORD_RESET_REQUEST", maxRequests: 5, windowSeconds: 3600); // 5 / hr
		PasswordResetVerify = Policy("PASSWORD_RESET_VERIFY", maxRequests: 10, windowSeconds: 900);   // 10 / 15 min
		EmailMutate = Policy("EMAIL_MUTATE", maxRequests: 10, windowSeconds: 900);                    // 10 / 15 min
	}

	private static RateLimitPolicy Policy(string name, int maxRequests, int windowSeconds) => new(
		name,
		Int($"RATE_LIMIT_{name}_MAX", maxRequests),
		Int($"RATE_LIMIT_{name}_WINDOW_SECONDS", windowSeconds));

	private static int Int(string variable, int fallback) =>
		int.TryParse(Environment.GetEnvironmentVariable(variable), out var value) ? value : fallback;

	private static bool Bool(string variable, bool fallback) =>
		bool.TryParse(Environment.GetEnvironmentVariable(variable), out var value) ? value : fallback;
}
