using System.Security.Claims;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;

namespace Gravity.Api.Endpoints;

/// <summary>Authenticated account-email management: add, change, resend, confirm.</summary>
public static class EmailEndpoints
{
	private const int MaxAttempts = 5;
	private static readonly TimeSpan CodeTtl = TimeSpan.FromMinutes(15);

	public static void MapEmailEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/auth/email");

		group.MapPost("/", async (
			AddEmailRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			EmailConfirmationRepository confirmations,
			IEmailSender emailSender,
			RateLimiter rateLimiter,
			RateLimitOptions options,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();
			var normalized = EmailRepository.Normalize(request.Email);

			// Keyed by the target email, not just the caller's IP -- the
			// target may not be the caller's own inbox.
			await rateLimiter.EnsureAllowedAsync(options.EmailMutate, $"email:{normalized}", ct);

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			if (!BCrypt.Net.BCrypt.Verify(request.Password, user.Password))
				throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

			if (user.Email is not null)
				throw new AppError("An email is already set on this account", 409, "EMAIL_ALREADY_SET");

			// Send before persisting: if the send throws (e.g. SES rejecting an
			// unverified recipient in sandbox mode), nothing gets written --
			// otherwise a failed send would still leave a "pending" row behind
			// that a later /auth/me refetch surfaces as a code-entry screen for
			// a code that was never delivered.
			var code = CodeGenerator.GenerateSixDigitCode();
			await emailSender.SendAsync(
				request.Email,
				"Confirm your Gravity email",
				$"Your confirmation code is: {code}\nThis code expires in 15 minutes.",
				ct);
			await confirmations.PutAsync(userId, request.Email, CodeGenerator.Hash(code), CodeTtl, ct);

			return Results.Accepted((string?)null, new { pendingEmail = request.Email });
		}).RequireAuthorization().RequireRateLimit(o => o.EmailMutate);

		group.MapPost("/resend", async (
			ClaimsPrincipal principal,
			EmailConfirmationRepository confirmations,
			IEmailSender emailSender,
			CancellationToken ct) =>
		{
			var userId = principal.UserId();

			var pending = await confirmations.GetAsync(userId, ct)
				?? throw new AppError("No pending email confirmation", 404, "NO_PENDING_EMAIL");

			// Rotate rather than resend the same code, so an intercepted old
			// code stops being useful. Send before persisting the rotated code
			// -- if the send fails, the previous (still-valid) code and row are
			// left untouched rather than being clobbered by a code that never
			// went anywhere.
			var code = CodeGenerator.GenerateSixDigitCode();
			await emailSender.SendAsync(
				pending.Email,
				"Confirm your Gravity email",
				$"Your confirmation code is: {code}\nThis code expires in 15 minutes.",
				ct);
			await confirmations.PutAsync(userId, pending.Email, CodeGenerator.Hash(code), CodeTtl, ct);

			return Results.Ok(new { pendingEmail = pending.Email });
		}).RequireAuthorization().RequireRateLimit(o => o.EmailMutate);

		group.MapPost("/confirm", async (
			ConfirmEmailRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			EmailRepository emails,
			EmailConfirmationRepository confirmations,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();

			var pending = await confirmations.GetAsync(userId, ct)
				?? throw new AppError("No pending email confirmation", 400, "NO_PENDING_EMAIL");

			if (pending.Attempts >= MaxAttempts)
			{
				await confirmations.DeleteAsync(userId, ct);
				throw new AppError("No pending email confirmation", 400, "NO_PENDING_EMAIL");
			}

			if (pending.CodeHash != CodeGenerator.Hash(request.Code))
			{
				await confirmations.IncrementAttemptsAsync(userId, ct);
				throw new AppError("Invalid code", 400, "INVALID_CODE");
			}

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			// May itself throw 409 EMAIL_TAKEN -- reaching that here requires
			// having already proven mailbox ownership via the correct code,
			// so it isn't an enumeration leak.
			await emails.ConfirmAsync(userId, user.Email, pending.Email, ct);

			return Results.Ok(new { email = pending.Email, emailConfirmed = true });
		}).RequireAuthorization().RequireRateLimit(o => o.EmailMutate);

		group.MapPatch("/", async (
			ChangeEmailRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			EmailConfirmationRepository confirmations,
			IEmailSender emailSender,
			RateLimiter rateLimiter,
			RateLimitOptions options,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();
			var normalized = EmailRepository.Normalize(request.NewEmail);

			await rateLimiter.EnsureAllowedAsync(options.EmailMutate, $"email:{normalized}", ct);

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			if (!BCrypt.Net.BCrypt.Verify(request.Password, user.Password))
				throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

			if (user.Email is null)
				throw new AppError("No email is set on this account yet", 409, "NO_EMAIL_SET");

			// Overwrites any prior pending row -- switching targets mid-flow
			// just works, and the current (confirmed) email stays valid for
			// recovery until this new one is confirmed. Send before persisting,
			// same reasoning as add-email above.
			var code = CodeGenerator.GenerateSixDigitCode();
			await emailSender.SendAsync(
				request.NewEmail,
				"Confirm your Gravity email",
				$"Your confirmation code is: {code}\nThis code expires in 15 minutes.",
				ct);
			await confirmations.PutAsync(userId, request.NewEmail, CodeGenerator.Hash(code), CodeTtl, ct);

			return Results.Accepted((string?)null, new { pendingEmail = request.NewEmail });
		}).RequireAuthorization().RequireRateLimit(o => o.EmailMutate);
	}
}
