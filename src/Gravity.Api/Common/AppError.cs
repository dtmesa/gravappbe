namespace Gravity.Api.Common;

/// <summary>
/// Port of src/utils/AppError.utils.ts. Carries an HTTP status and the
/// machine-readable code the client branches on.
/// </summary>
public class AppError : Exception
{
	public int StatusCode { get; }
	public string? Code { get; }
	public int? RetryAfterSeconds { get; private init; }

	public AppError(string message, int statusCode = 500, string? code = null) : base(message)
	{
		StatusCode = statusCode;
		Code = code;
	}

	public static AppError Unauthorized() => new("Unauthorized", 401, "UNAUTHORIZED");

	public static AppError NotFound(string entity, string code) => new($"{entity} not found", 404, code);

	public static AppError TooManyRequests(int retryAfterSeconds) =>
		new("Too many requests", 429, "RATE_LIMITED") { RetryAfterSeconds = retryAfterSeconds };
}
