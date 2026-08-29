using System.Text.Json;
using FluentValidation;

namespace Gravity.Api.Common;

/// <summary>
/// Port of src/middleware/error.middleware.ts. The response envelope is part of
/// the client contract: the app reads `.error` off the body (see
/// workout-app/src/api/error.api.ts) and branches on codes like USERNAME_TAKEN.
/// </summary>
public class ExceptionMiddleware
{
	private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

	private readonly RequestDelegate _next;
	private readonly ILogger<ExceptionMiddleware> _logger;

	public ExceptionMiddleware(RequestDelegate next, ILogger<ExceptionMiddleware> logger)
	{
		_next = next;
		_logger = logger;
	}

	public async Task InvokeAsync(HttpContext context)
	{
		try
		{
			await _next(context);
		}
		catch (ValidationException ex)
		{
			await Write(context, 400, new
			{
				error = "VALIDATION_ERROR",
				issues = ex.Errors.Select(e => new { path = e.PropertyName, message = e.ErrorMessage }),
			});
		}
		catch (AppError ex)
		{
			if (ex.RetryAfterSeconds is { } retryAfter)
				context.Response.Headers["Retry-After"] = retryAfter.ToString();

			await Write(context, ex.StatusCode, new { error = ex.Code ?? ex.Message });
		}
		catch (Exception ex)
		{
			_logger.LogError(ex, "Unhandled exception on {Method} {Path}", context.Request.Method, context.Request.Path);
			await Write(context, 500, new { error = "INTERNAL_SERVER_ERROR" });
		}
	}

	private static async Task Write(HttpContext context, int status, object body)
	{
		if (context.Response.HasStarted) return;

		context.Response.Clear();
		context.Response.StatusCode = status;
		context.Response.ContentType = "application/json; charset=utf-8";
		await context.Response.WriteAsync(JsonSerializer.Serialize(body, JsonOptions));
	}
}
