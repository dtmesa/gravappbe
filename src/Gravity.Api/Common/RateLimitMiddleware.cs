namespace Gravity.Api.Common;

/// <summary>
/// Applies the General policy to every request, keyed by source IP. This is
/// the blanket "abnormally high volume" guard; auth endpoints layer the
/// stricter Login/Register/AuthMutate policies on top via RequireRateLimit.
/// </summary>
public class RateLimitMiddleware
{
	private readonly RequestDelegate _next;

	public RateLimitMiddleware(RequestDelegate next) => _next = next;

	public async Task InvokeAsync(HttpContext context, RateLimiter limiter, RateLimitOptions options)
	{
		if (!options.Enabled)
		{
			await _next(context);
			return;
		}

		var ip = context.Connection.RemoteIpAddress?.ToString() ?? "unknown";
		var result = await limiter.CheckAsync(options.General, ip, context.RequestAborted);

		if (!result.Allowed) throw AppError.TooManyRequests(result.RetryAfterSeconds);

		await _next(context);
	}
}

public static class RateLimitingExtensions
{
	/// <summary>
	/// Layers a stricter, named policy on top of the general middleware for one
	/// endpoint -- e.g. login gets a much lower ceiling than general API
	/// traffic even though both count against the same client IP.
	/// </summary>
	public static RouteHandlerBuilder RequireRateLimit(
		this RouteHandlerBuilder builder,
		Func<RateLimitOptions, RateLimitPolicy> policy) =>
		builder.AddEndpointFilter(async (context, next) =>
		{
			var options = context.HttpContext.RequestServices.GetRequiredService<RateLimitOptions>();

			if (!options.Enabled) return await next(context);

			var limiter = context.HttpContext.RequestServices.GetRequiredService<RateLimiter>();
			var ip = context.HttpContext.Connection.RemoteIpAddress?.ToString() ?? "unknown";
			var result = await limiter.CheckAsync(policy(options), ip, context.HttpContext.RequestAborted);

			if (!result.Allowed) throw AppError.TooManyRequests(result.RetryAfterSeconds);

			return await next(context);
		});
}
