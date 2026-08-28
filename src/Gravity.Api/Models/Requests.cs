namespace Gravity.Api.Models;

// Request bodies. Missing JSON fields land as empty/null and are rejected by the
// validators, matching how the Zod schemas in src/schemas/ behaved.

public class RegisterRequest
{
	public string Username { get; set; } = string.Empty;
	public string Password { get; set; } = string.Empty;
}

public class LoginRequest
{
	public string Username { get; set; } = string.Empty;
	public string Password { get; set; } = string.Empty;
}

public class UpdateUsernameRequest
{
	public string NewUsername { get; set; } = string.Empty;
	public string Password { get; set; } = string.Empty;
}

public class UpdatePasswordRequest
{
	public string CurrentPassword { get; set; } = string.Empty;
	public string NewPassword { get; set; } = string.Empty;
}

public class DeleteAccountRequest
{
	public string Password { get; set; } = string.Empty;
}

public class CreateNamedRequest
{
	public string Name { get; set; } = string.Empty;
}

public class CreateWorkoutSessionRequest
{
	public DateTime? Date { get; set; }
}

public class CreateExerciseSessionRequest
{
	public int ExerciseId { get; set; }
}
