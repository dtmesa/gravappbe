using FluentValidation;
using FluentValidation.Results;
using Gravity.Api.Common;
using Gravity.Api.Models;

namespace Gravity.Api.Validation;

/// <summary>
/// Ports of src/schemas/*.ts. Messages are carried over verbatim where the Zod
/// schema supplied one; the client only reads the top-level `error` code, but
/// keeping them makes the `issues` array useful for debugging.
/// </summary>
public static class Rules
{
	public static IRuleBuilderOptions<T, string> Password<T>(this IRuleBuilder<T, string> rule) =>
		rule.MinimumLength(8).WithMessage("Password must be at least 8 characters.")
			.MaximumLength(32).WithMessage("Password must be fewer than 32 characters.")
			.Must(v => v == v.Trim()).WithMessage("Password cannot start or end with spaces");

	public static IRuleBuilderOptions<T, string> Username<T>(this IRuleBuilder<T, string> rule) =>
		rule.MinimumLength(3).WithMessage("Username must be at least 3 characters")
			.MaximumLength(20).WithMessage("Username must be fewer than 20 characters")
			.Must(v => v == v.Trim()).WithMessage("Username cannot start or end with spaces");

	/// <summary>Shared by the workout and exercise create/rename schemas.</summary>
	public static IRuleBuilderOptions<T, string> EntityName<T>(this IRuleBuilder<T, string> rule) =>
		rule.NotEmpty().MaximumLength(75)
			.Must(v => v == v.Trim()).WithMessage("Name cannot start or end with spaces");
}

public class RegisterValidator : AbstractValidator<RegisterRequest>
{
	public RegisterValidator()
	{
		RuleFor(x => x.Username).Username();
		RuleFor(x => x.Password).Password();
	}
}

public class LoginValidator : AbstractValidator<LoginRequest>
{
	public LoginValidator()
	{
		RuleFor(x => x.Username).NotEmpty();
		RuleFor(x => x.Password).NotEmpty();
	}
}

public class UpdateUsernameValidator : AbstractValidator<UpdateUsernameRequest>
{
	public UpdateUsernameValidator()
	{
		RuleFor(x => x.NewUsername).Username();
		RuleFor(x => x.Password).NotEmpty();
	}
}

public class UpdatePasswordValidator : AbstractValidator<UpdatePasswordRequest>
{
	public UpdatePasswordValidator()
	{
		RuleFor(x => x.CurrentPassword).NotEmpty();
		RuleFor(x => x.NewPassword).Password();
	}
}

public class DeleteAccountValidator : AbstractValidator<DeleteAccountRequest>
{
	public DeleteAccountValidator() =>
		RuleFor(x => x.Password).NotEmpty().WithMessage("Password is required");
}

public class CreateNamedValidator : AbstractValidator<CreateNamedRequest>
{
	public CreateNamedValidator() => RuleFor(x => x.Name).EntityName();
}

public class CreateExerciseSessionValidator : AbstractValidator<CreateExerciseSessionRequest>
{
	public CreateExerciseSessionValidator() => RuleFor(x => x.ExerciseId).GreaterThan(0);
}

/// <summary>
/// Entry point for the endpoint handlers. Throws ValidationException, which the
/// exception middleware renders as 400 VALIDATION_ERROR.
/// </summary>
public static class Validate
{
	private static readonly RegisterValidator Register = new();
	private static readonly LoginValidator Login = new();
	private static readonly UpdateUsernameValidator UpdateUsername = new();
	private static readonly UpdatePasswordValidator UpdatePassword = new();
	private static readonly DeleteAccountValidator DeleteAccount = new();
	private static readonly CreateNamedValidator CreateNamed = new();
	private static readonly CreateExerciseSessionValidator CreateExerciseSession = new();

	public static RegisterRequest Check(RegisterRequest r) { Register.ValidateAndThrow(r); return r; }
	public static LoginRequest Check(LoginRequest r) { Login.ValidateAndThrow(r); return r; }
	public static UpdateUsernameRequest Check(UpdateUsernameRequest r) { UpdateUsername.ValidateAndThrow(r); return r; }
	public static UpdatePasswordRequest Check(UpdatePasswordRequest r) { UpdatePassword.ValidateAndThrow(r); return r; }
	public static DeleteAccountRequest Check(DeleteAccountRequest r) { DeleteAccount.ValidateAndThrow(r); return r; }
	public static CreateNamedRequest Check(CreateNamedRequest r) { CreateNamed.ValidateAndThrow(r); return r; }
	public static CreateExerciseSessionRequest Check(CreateExerciseSessionRequest r) { CreateExerciseSession.ValidateAndThrow(r); return r; }

	/// <summary>
	/// Builds an exception carrying a real failure, so the `issues` array in the
	/// 400 response is populated the way Zod's issues were rather than empty.
	/// </summary>
	public static ValidationException Invalid(string property, string message) =>
		new([new ValidationFailure(property, message)]);

	/// <summary>
	/// Route ids were `z.coerce.number().int().positive()`. Route constraints
	/// already reject non-numeric segments; this enforces the positive bound.
	/// </summary>
	public static int PositiveId(int value, string name)
	{
		if (value <= 0)
			throw Invalid(name, $"{name} must be a positive integer");

		return value;
	}

	/// <summary>Body must be present at all -- Express got `{}` from express.json().</summary>
	public static T Required<T>(T? body) where T : class =>
		body ?? throw Invalid("body", "Request body is required");
}
