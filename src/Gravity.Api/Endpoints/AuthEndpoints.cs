using System.Security.Claims;
using Gravity.Api.Common;
using Gravity.Api.Data;
using Gravity.Api.Models;
using Gravity.Api.Validation;
using Microsoft.AspNetCore.Mvc;

namespace Gravity.Api.Endpoints;

/// <summary>Port of src/routes/auth.routes.ts.</summary>
public static class AuthEndpoints
{
	// Matches bcrypt.hash(password, 12) in the Node backend, so hashes remain
	// mutually verifiable.
	private const int WorkFactor = 12;

	public static void MapAuthEndpoints(this IEndpointRouteBuilder app)
	{
		var group = app.MapGroup("/auth");

		group.MapGet("/me", async (
			ClaimsPrincipal principal,
			UserRepository users,
			EmailConfirmationRepository pendingEmails,
			CancellationToken ct) =>
		{
			var userId = principal.UserId();

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			var pending = await pendingEmails.GetAsync(userId, ct);

			return Results.Ok(new
			{
				username = user.Username,
				email = user.Email,
				emailConfirmed = user.EmailConfirmed,
				pendingEmail = pending?.Email,
			});
		}).RequireAuthorization();

		group.MapPost("/register", async (RegisterRequest? body, UserRepository users, CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));

			var hashed = BCrypt.Net.BCrypt.HashPassword(request.Password, WorkFactor);
			var user = await users.CreateAsync(request.Username, hashed, ct);

			return Results.Created((string?)null, new { id = user.Id, username = user.Username });
		}).RequireRateLimit(o => o.Register);

		group.MapPost("/login", async (LoginRequest? body, UserRepository users, JwtService jwt, CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));

			var user = await users.GetByUsernameAsync(request.Username, ct);

			// A missing user and a bad password deliberately return the same code.
			if (user is null || !BCrypt.Net.BCrypt.Verify(request.Password, user.Password))
				throw new AppError("Invalid credentials", 401, "INVALID_CREDENTIALS");

			return Results.Ok(new { token = jwt.SignToken(user.Id, user.TokenVersion) });
		}).RequireRateLimit(o => o.Login);

		group.MapPatch("/username", async (
			UpdateUsernameRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			if (!BCrypt.Net.BCrypt.Verify(request.Password, user.Password))
				throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

			await users.UpdateUsernameAsync(userId, user.Username, request.NewUsername, ct);

			return Results.Ok(new { username = request.NewUsername });
		}).RequireAuthorization().RequireRateLimit(o => o.AuthMutate);

		group.MapPatch("/password", async (
			UpdatePasswordRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			JwtService jwt,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			if (!BCrypt.Net.BCrypt.Verify(request.CurrentPassword, user.Password))
				throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

			if (request.CurrentPassword == request.NewPassword)
				throw new AppError("New password must differ from current", 400, "SAME_PASSWORD");

			var hashed = BCrypt.Net.BCrypt.HashPassword(request.NewPassword, WorkFactor);
			var newTokenVersion = await users.UpdatePasswordAsync(userId, hashed, ct);

			// UpdatePasswordAsync bumps TokenVersion, which invalidates the
			// caller's own token too -- reissue one so this session survives.
			return Results.Ok(new { token = jwt.SignToken(userId, newTokenVersion) });
		}).RequireAuthorization().RequireRateLimit(o => o.AuthMutate);

		// [FromBody] is required: minimal APIs refuse to infer a body on DELETE,
		// and this route carries one ({ password }).
		group.MapDelete("/delete", async (
			[FromBody] DeleteAccountRequest? body,
			ClaimsPrincipal principal,
			UserRepository users,
			CascadeService cascade,
			CancellationToken ct) =>
		{
			var request = Validate.Check(Validate.Required(body));
			var userId = principal.UserId();

			var user = await users.GetByIdAsync(userId, ct)
				?? throw AppError.NotFound("User", "USER_NOT_FOUND");

			if (!BCrypt.Net.BCrypt.Verify(request.Password, user.Password))
				throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

			// User row (and the username claim) goes first, parent-before-children,
			// so /auth/me and login stop working on this account immediately.
			await users.DeleteAsync(userId, user.Username, user.Email, ct);
			await cascade.DeleteUserDataAsync(userId, ct);

			return Results.NoContent();
		}).RequireAuthorization().RequireRateLimit(o => o.AuthMutate);
	}
}
