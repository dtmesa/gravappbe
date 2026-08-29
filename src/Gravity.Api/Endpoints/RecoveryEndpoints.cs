using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>
/// Unauthenticated account-recovery flows: forgotten username and password
/// reset via an emailed one-time code. Every response that depends on whether
/// an email has an account is deliberately identical either way -- these
/// endpoints must never leak that signal.
/// </summary>
public static class RecoveryEndpoints
{
	// Matches AuthEndpoints' bcrypt work factor, so hashes stay mutually verifiable.
	private const int WorkFactor = 12;
	private const int MaxAttempts = 5;
	private static readonly TimeSpan CodeTtl = TimeSpan.FromMinutes(15);

	public static void MapRecoveryEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/auth");

		group.MapPost("/forgot-username", async (
			ForgotUsernameRequest? body,
			EmailRepository emails,
			UserRepository users,
			IEmailSender emailSender,
			RateLimiter rateLimiter,
			RateLimitOptions options,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var normalized = EmailRepository.Normalize(request.Email);

			// Second check on top of the per-IP filter below, keyed by the
			// *target* email -- stops an attacker rotating IPs from spamming
			// one victim's inbox.
			await rateLimiter.EnsureAllowedAsync(options.ForgotUsername, $"email:{normalized}", ct);

			var user = await emails.GetByEmailAsync(request.Email, users, ct);

			if (user is not null)
			{
				await emailSender.SendAsync(
					request.Email,
					"Your Gravity username",
					EmailTemplates.ForgotUsername(user.Username),
					ct);
			}

			return Results.Ok(new { message = "If that email is registered, we've sent the username to it." });
		}).RequireRateLimit(o => o.ForgotUsername);

		group.MapPost("/password-reset/request", async (
			RequestPasswordResetRequest? body,
			EmailRepository emails,
			UserRepository users,
			PasswordResetRepository resets,
			IEmailSender emailSender,
			RateLimiter rateLimiter,
			RateLimitOptions options,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var normalized = EmailRepository.Normalize(request.Email);

			await rateLimiter.EnsureAllowedAsync(options.PasswordResetRequest, $"email:{normalized}", ct);

			var user = await emails.GetByEmailAsync(request.Email, users, ct);

			if (user is not null)
			{
				// Send before persisting -- same reasoning as the email
				// confirmation endpoints: a failed send should never leave a
				// usable code sitting in the database that nobody received.
				var code = CodeGenerator.GenerateSixDigitCode();
				await emailSender.SendAsync(
					request.Email,
					"Your Gravity password reset code",
					EmailTemplates.PasswordReset(code),
					ct);
				await resets.PutAsync(request.Email, user.Id, CodeGenerator.Hash(code), CodeTtl, ct);
			}

			return Results.Ok(new { message = "If that email is registered, we've sent a reset code to it." });
		}).RequireRateLimit(o => o.PasswordResetRequest);

		group.MapPost("/password-reset/verify", async (
			VerifyPasswordResetRequest? body,
			PasswordResetRepository resets,
			UserRepository users,
			JwtService jwt,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));

			var code = await resets.GetAsync(request.Email, ct)
				?? throw new AppError("Invalid or expired code", 400, "INVALID_RESET_CODE");

			// Forces a fresh request rather than leaking "you were close" via a
			// distinct too-many-attempts response.
			if (code.Attempts >= MaxAttempts)
			{
				await resets.DeleteAsync(request.Email, ct);
				throw new AppError("Invalid or expired code", 400, "INVALID_RESET_CODE");
			}

			if (code.CodeHash != CodeGenerator.Hash(request.Code))
			{
				await resets.IncrementAttemptsAsync(request.Email, ct);
				throw new AppError("Invalid or expired code", 400, "INVALID_RESET_CODE");
			}

			var hashed = BCrypt.Net.BCrypt.HashPassword(request.NewPassword, WorkFactor);
			var newTokenVersion = await users.UpdatePasswordAsync(code.UserId, hashed, ct);
			await resets.DeleteAsync(request.Email, ct);

			// A fresh token so the device completing the reset stays logged in
			// even though this also invalidates every other outstanding token.
			return Results.Ok(new { token = jwt.SignToken(code.UserId, newTokenVersion) });
		}).RequireRateLimit(o => o.PasswordResetVerify);
	}
}
