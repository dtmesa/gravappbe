using System.Text.Json;
using Amazon.DynamoDBv2.Model;
using FluentValidation;
using Gravity.Api.Data;

namespace Gravity.Api.Validation;

/// <summary>
/// The PATCH /:id/:field routes took a Zod discriminated union keyed on the
/// field name, so the body had to carry a property of that exact name with a
/// type specific to it. This reproduces that, and doubles as the allowlist that
/// keeps the raw :field route value out of the UpdateExpression.
/// </summary>
public static class Patch
{
	private static JsonElement Property(JsonElement body, string field)
	{
		if (body.ValueKind != JsonValueKind.Object || !body.TryGetProperty(field, out var value))
			throw new ValidationException($"Missing value for field '{field}'");

		return value;
	}

	public static AttributeValue Text(JsonElement body, string field, int maxLength)
	{
		var value = Property(body, field);

		if (value.ValueKind != JsonValueKind.String)
			throw new ValidationException($"'{field}' must be a string");

		var text = value.GetString()!;

		if (text.Length > maxLength)
			throw new ValidationException($"'{field}' must be at most {maxLength} characters");

		return Dyn.S(text);
	}

	/// <summary>Shared by the workout and exercise `name` field.</summary>
	public static AttributeValue Name(JsonElement body, string field)
	{
		var attribute = Text(body, field, 75);
		var text = attribute.S;

		if (text.Length < 1)
			throw new ValidationException($"'{field}' must not be empty");

		if (text != text.Trim())
			throw new ValidationException("Name cannot start or end with spaces");

		return attribute;
	}

	public static AttributeValue Integer(JsonElement body, string field, int minimum)
	{
		var value = Property(body, field);

		if (value.ValueKind != JsonValueKind.Number || !value.TryGetInt32(out var number))
			throw new ValidationException($"'{field}' must be an integer");

		if (number < minimum)
			throw new ValidationException($"'{field}' must be at least {minimum}");

		return Dyn.N(number);
	}

	public static AttributeValue Decimal(JsonElement body, string field, double minimum)
	{
		var value = Property(body, field);

		if (value.ValueKind != JsonValueKind.Number)
			throw new ValidationException($"'{field}' must be a number");

		var number = value.GetDouble();

		if (number < minimum)
			throw new ValidationException($"'{field}' must be at least {minimum}");

		return Dyn.N(number);
	}

	public static AttributeValue Flag(JsonElement body, string field)
	{
		var value = Property(body, field);

		if (value.ValueKind is not (JsonValueKind.True or JsonValueKind.False))
			throw new ValidationException($"'{field}' must be a boolean");

		return Dyn.Bool(value.GetBoolean());
	}

	public static DateTime Date(JsonElement body, string field)
	{
		var value = Property(body, field);

		if (value.ValueKind != JsonValueKind.String || !value.TryGetDateTime(out var date))
			throw new ValidationException($"'{field}' must be a date");

		return date.ToUniversalTime();
	}

	// --- Per-entity allowlists, mirroring ALLOWED_FIELDS in src/schemas/ ---

	public static AttributeValue ForWorkout(string field, JsonElement body) => field switch
	{
		"order" => Integer(body, field, 0),
		"description" => Text(body, field, 500),
		"name" => Name(body, field),
		_ => throw new ValidationException($"Unsupported field '{field}'"),
	};

	public static AttributeValue ForExercise(string field, JsonElement body) => field switch
	{
		"description" => Text(body, field, 500),
		"order" => Integer(body, field, 0),
		"name" => Name(body, field),
		"isWeight" or "isDuration" or "isReps" or "isDistance" => Flag(body, field),
		_ => throw new ValidationException($"Unsupported field '{field}'"),
	};

	/// <summary>
	/// reps and duration back Int columns, so they are required to be integers
	/// here. The Zod schema allowed any non-negative number for duration, which
	/// Prisma then rejected at write time with a 500 -- a 400 is the better
	/// answer and the client never sends fractional values.
	/// </summary>
	public static AttributeValue ForSetSession(string field, JsonElement body) => field switch
	{
		"reps" => Integer(body, field, 0),
		"duration" => Integer(body, field, 0),
		"weight" => Decimal(body, field, 0),
		"distance" => Decimal(body, field, 0),
		_ => throw new ValidationException($"Unsupported field '{field}'"),
	};
}
