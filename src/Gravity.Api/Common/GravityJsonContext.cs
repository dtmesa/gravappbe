using System.Text.Json.Serialization;
using Gravity.Api.Models;

namespace Gravity.Api.Common;

/// <summary>
/// Source-generated serializer metadata for every type crossing the wire.
/// Native AOT strips the reflection metadata System.Text.Json would otherwise
/// use, so anything missing here fails at runtime rather than at build time --
/// which is why this is registered as the only resolver (see Program.cs) even
/// while still running on the JIT, so a gap surfaces in local testing.
///
/// The options mirror JsonSerializerDefaults.Web, which is what minimal APIs
/// apply by default; they have to be restated because the generator bakes
/// property names and number handling into the generated metadata.
/// </summary>
[JsonSourceGenerationOptions(
	PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
	PropertyNameCaseInsensitive = true,
	NumberHandling = JsonNumberHandling.AllowReadingFromString,
	Converters = [typeof(JsonDateTimeConverter)])]
// Requests
[JsonSerializable(typeof(RegisterRequest))]
[JsonSerializable(typeof(LoginRequest))]
[JsonSerializable(typeof(UpdateUsernameRequest))]
[JsonSerializable(typeof(UpdatePasswordRequest))]
[JsonSerializable(typeof(DeleteAccountRequest))]
[JsonSerializable(typeof(ForgotUsernameRequest))]
[JsonSerializable(typeof(RequestPasswordResetRequest))]
[JsonSerializable(typeof(VerifyPasswordResetRequest))]
[JsonSerializable(typeof(AddEmailRequest))]
[JsonSerializable(typeof(ConfirmEmailRequest))]
[JsonSerializable(typeof(ChangeEmailRequest))]
[JsonSerializable(typeof(CreateNamedRequest))]
[JsonSerializable(typeof(CreateWorkoutSessionRequest))]
[JsonSerializable(typeof(CreateExerciseSessionRequest))]
// Responses
[JsonSerializable(typeof(StatusResponse))]
[JsonSerializable(typeof(RegisterResponse))]
[JsonSerializable(typeof(TokenResponse))]
[JsonSerializable(typeof(UsernameResponse))]
[JsonSerializable(typeof(MeResponse))]
[JsonSerializable(typeof(PendingEmailResponse))]
[JsonSerializable(typeof(EmailConfirmedResponse))]
[JsonSerializable(typeof(MessageResponse))]
[JsonSerializable(typeof(CountResponse))]
[JsonSerializable(typeof(ErrorResponse))]
// Entities and the collection shapes the endpoints return
[JsonSerializable(typeof(Workout))]
[JsonSerializable(typeof(Exercise))]
[JsonSerializable(typeof(WorkoutSession))]
[JsonSerializable(typeof(ExerciseSession))]
[JsonSerializable(typeof(SetSession))]
[JsonSerializable(typeof(List<Workout>))]
[JsonSerializable(typeof(List<Exercise>))]
[JsonSerializable(typeof(List<WorkoutSession>))]
[JsonSerializable(typeof(List<ExerciseSession>))]
[JsonSerializable(typeof(List<SetSession>))]
// History projections and averages
[JsonSerializable(typeof(NamedRef))]
[JsonSerializable(typeof(HistoryExerciseSession))]
[JsonSerializable(typeof(HistorySession))]
[JsonSerializable(typeof(HistorySession[]))]
[JsonSerializable(typeof(IEnumerable<HistorySession>))]
[JsonSerializable(typeof(Averages.Result))]
[JsonSerializable(typeof(System.Text.Json.JsonElement))]
public partial class GravityJsonContext : JsonSerializerContext;
