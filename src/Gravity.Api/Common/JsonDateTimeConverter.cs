using System.Globalization;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace Gravity.Api.Common;

/// <summary>
/// Emits dates with exactly three fractional digits, as Prisma did
/// ("2026-08-28T06:52:31.250Z"). System.Text.Json's default round-trip format
/// trims trailing zeros, so a timestamp landing on 250ms would serialize as
/// ".25Z" -- parseable, but not byte-identical to the responses the Node
/// backend produced.
/// </summary>
public class JsonDateTimeConverter : JsonConverter<DateTime>
{
	private const string Format = "yyyy-MM-ddTHH:mm:ss.fffZ";

	public override DateTime Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options) =>
		reader.GetDateTime().ToUniversalTime();

	public override void Write(Utf8JsonWriter writer, DateTime value, JsonSerializerOptions options) =>
		writer.WriteStringValue(value.ToUniversalTime().ToString(Format, CultureInfo.InvariantCulture));
}
