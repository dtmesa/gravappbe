namespace Gravity.Api.Common;

/// <summary>
/// Timestamps are truncated to milliseconds so the object returned from a create
/// is byte-identical to what a later read returns -- DateTime.UtcNow carries
/// 100ns ticks, which would serialize as 7 fractional digits on the create
/// response and 3 on every subsequent read. Prisma always emitted 3.
/// </summary>
public static class Clock
{
	public static DateTime UtcNow() => Truncate(DateTime.UtcNow);

	public static DateTime Truncate(DateTime value)
	{
		var utc = value.ToUniversalTime();

		return new DateTime(utc.Ticks - utc.Ticks % TimeSpan.TicksPerMillisecond, DateTimeKind.Utc);
	}
}
