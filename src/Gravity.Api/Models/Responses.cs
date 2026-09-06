using System.Text.Json.Serialization;

namespace Gravity.Api.Models;

// Response bodies that used to be anonymous types. Source-generated JSON needs a
// nameable type for everything it serializes, and an anonymous type cannot be
// referenced from a JsonSerializerContext. Property order here is the order they
// serialize in, so it must match what the client already receives.

public record StatusResponse(string Status);

public record RegisterResponse(int Id, string Username);

public record TokenResponse(string Token);

public record UsernameResponse(string Username);

public record MeResponse(string Username, string? Email, bool EmailConfirmed, string? PendingEmail);

public record PendingEmailResponse(string PendingEmail);

public record EmailConfirmedResponse(string Email, bool EmailConfirmed);

public record MessageResponse(string Message);

public record CountResponse(int Count);

public record ValidationIssue(string Path, string Message);

/// <summary>
/// The error envelope the app branches on (see workout-app/src/api/error.api.ts).
/// Issues is omitted entirely rather than written as null, so non-validation
/// errors keep serializing as a bare {"error":"CODE"}.
/// </summary>
public record ErrorResponse(
	string Error,
	[property: JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
	IReadOnlyList<ValidationIssue>? Issues = null);

// The /history/sessions projection. Prisma served this shape from one query with
// nested includes; it is assembled by hand now, but the wire shape is unchanged.

public record NamedRef(string Name);

public record HistoryExerciseSession(
	int Id,
	int Order,
	int WorkoutSessionId,
	int ExerciseId,
	DateTime CreatedAt,
	NamedRef Exercise,
	List<SetSession> Sets);

public record HistorySession(
	int Id,
	DateTime Date,
	int UserId,
	int WorkoutId,
	DateTime CreatedAt,
	NamedRef Workout,
	List<HistoryExerciseSession> Exercises);
