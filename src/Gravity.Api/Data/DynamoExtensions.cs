using System.Globalization;
using Amazon.DynamoDBv2.Model;

namespace Gravity.Api.Data;

/// <summary>
/// Attribute helpers. Null-valued fields are omitted from the item and read
/// back as null, which keeps the JSON contract (`weight: null`) intact without
/// storing NULL attributes.
/// </summary>
public static class Dyn
{
	// Dates round-trip as ISO-8601 UTC so they sort lexicographically, which is
	// what the date-keyed indexes rely on.
	private const string DateFormat = "yyyy-MM-ddTHH:mm:ss.fffZ";

	public static AttributeValue N(int value) => new() { N = value.ToString(CultureInfo.InvariantCulture) };

	public static AttributeValue N(double value) => new() { N = value.ToString("R", CultureInfo.InvariantCulture) };

	public static AttributeValue S(string value) => new() { S = value };

	public static AttributeValue Bool(bool value) => new() { BOOL = value };

	public static AttributeValue Date(DateTime value) =>
		new() { S = value.ToUniversalTime().ToString(DateFormat, CultureInfo.InvariantCulture) };

	public static int GetInt(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.N is not null
			? int.Parse(v.N, CultureInfo.InvariantCulture)
			: 0;

	public static int? GetIntOrNull(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.N is not null
			? int.Parse(v.N, CultureInfo.InvariantCulture)
			: null;

	public static double? GetDoubleOrNull(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.N is not null
			? double.Parse(v.N, CultureInfo.InvariantCulture)
			: null;

	public static string GetString(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.S is not null ? v.S : string.Empty;

	public static string? GetStringOrNull(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.S is not null ? v.S : null;

	public static bool GetBool(this Dictionary<string, AttributeValue> item, string key) =>
		item.TryGetValue(key, out var v) && v.IsBOOLSet && v.BOOL == true;

	public static DateTime GetDate(this Dictionary<string, AttributeValue> item, string key)
	{
		var raw = item.GetStringOrNull(key);

		if (raw is null) return default;

		return DateTime.Parse(raw, CultureInfo.InvariantCulture, DateTimeStyles.AdjustToUniversal | DateTimeStyles.AssumeUniversal);
	}

	/// <summary>Adds the attribute only when the value is present.</summary>
	public static void SetIfNotNull(this Dictionary<string, AttributeValue> item, string key, double? value)
	{
		if (value.HasValue) item[key] = N(value.Value);
	}

	public static void SetIfNotNull(this Dictionary<string, AttributeValue> item, string key, int? value)
	{
		if (value.HasValue) item[key] = N(value.Value);
	}

	public static void SetIfNotNull(this Dictionary<string, AttributeValue> item, string key, string? value)
	{
		if (value is not null) item[key] = S(value);
	}
}
