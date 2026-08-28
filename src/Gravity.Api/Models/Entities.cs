using System.Text.Json.Serialization;
using Amazon.DynamoDBv2.Model;
using Gravity.Api.Data;

namespace Gravity.Api.Models;

using Item = Dictionary<string, AttributeValue>;

/// <summary>
/// Storage + wire models. Property names serialize to camelCase, matching what
/// Prisma emitted. Scalar nulls are written explicitly (`description: null`);
/// relation properties are omitted when absent, mirroring Prisma's `include`.
/// </summary>
public class User
{
	public int Id { get; set; }
	public string Username { get; set; } = string.Empty;

	[JsonIgnore]
	public string Password { get; set; } = string.Empty;

	public DateTime CreatedAt { get; set; }

	public static User FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Username = item.GetString("username"),
		Password = item.GetString("password"),
		CreatedAt = item.GetDate("createdAt"),
	};

	public Item ToItem() => new()
	{
		["id"] = Dyn.N(Id),
		["username"] = Dyn.S(Username),
		["password"] = Dyn.S(Password),
		["createdAt"] = Dyn.Date(CreatedAt),
	};
}

public class Workout
{
	public int Id { get; set; }
	public string Name { get; set; } = string.Empty;
	public string? Description { get; set; }
	public int Order { get; set; }
	public int UserId { get; set; }
	public DateTime CreatedAt { get; set; }

	public static Workout FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Name = item.GetString("name"),
		Description = item.GetStringOrNull("description"),
		Order = item.GetInt("order"),
		UserId = item.GetInt("userId"),
		CreatedAt = item.GetDate("createdAt"),
	};

	public Item ToItem()
	{
		var item = new Item
		{
			["userId"] = Dyn.N(UserId),
			["id"] = Dyn.N(Id),
			["name"] = Dyn.S(Name),
			["order"] = Dyn.N(Order),
			["createdAt"] = Dyn.Date(CreatedAt),
		};

		item.SetIfNotNull("description", Description);

		return item;
	}
}

public class Exercise
{
	public int Id { get; set; }
	public int Order { get; set; }
	public string Name { get; set; } = string.Empty;
	public string? Description { get; set; }
	public int WorkoutId { get; set; }
	public bool IsWeight { get; set; }
	public bool IsDuration { get; set; }
	public bool IsDistance { get; set; }
	public bool IsReps { get; set; }
	public DateTime CreatedAt { get; set; }

	public static Exercise FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Order = item.GetInt("order"),
		Name = item.GetString("name"),
		Description = item.GetStringOrNull("description"),
		WorkoutId = item.GetInt("workoutId"),
		IsWeight = item.GetBool("isWeight"),
		IsDuration = item.GetBool("isDuration"),
		IsDistance = item.GetBool("isDistance"),
		IsReps = item.GetBool("isReps"),
		CreatedAt = item.GetDate("createdAt"),
	};

	public Item ToItem()
	{
		var item = new Item
		{
			["workoutId"] = Dyn.N(WorkoutId),
			["id"] = Dyn.N(Id),
			["order"] = Dyn.N(Order),
			["name"] = Dyn.S(Name),
			["isWeight"] = Dyn.Bool(IsWeight),
			["isDuration"] = Dyn.Bool(IsDuration),
			["isDistance"] = Dyn.Bool(IsDistance),
			["isReps"] = Dyn.Bool(IsReps),
			["createdAt"] = Dyn.Date(CreatedAt),
		};

		item.SetIfNotNull("description", Description);

		return item;
	}
}

public class WorkoutSession
{
	public int Id { get; set; }
	public DateTime Date { get; set; }
	public int UserId { get; set; }
	public int WorkoutId { get; set; }
	public DateTime CreatedAt { get; set; }

	/// <summary>Populated only where the Express route used `include: { workout: true }`.</summary>
	[JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
	public object? Workout { get; set; }

	public static WorkoutSession FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Date = item.GetDate("date"),
		UserId = item.GetInt("userId"),
		WorkoutId = item.GetInt("workoutId"),
		CreatedAt = item.GetDate("createdAt"),
	};

	public Item ToItem() => new()
	{
		["userId"] = Dyn.N(UserId),
		["id"] = Dyn.N(Id),
		["date"] = Dyn.Date(Date),
		["workoutId"] = Dyn.N(WorkoutId),
		["createdAt"] = Dyn.Date(CreatedAt),
	};
}

public class ExerciseSession
{
	public int Id { get; set; }
	public int Order { get; set; }
	public int WorkoutSessionId { get; set; }
	public int ExerciseId { get; set; }
	public DateTime CreatedAt { get; set; }

	[JsonIgnore(Condition = JsonIgnoreCondition.WhenWritingNull)]
	public List<SetSession>? Sets { get; set; }

	// Denormalized from the parent WorkoutSession. Without these, /averages and
	// /previous-set-count would require a table scan.
	[JsonIgnore] public int UserId { get; set; }
	[JsonIgnore] public int WorkoutId { get; set; }
	[JsonIgnore] public DateTime SessionDate { get; set; }

	public static ExerciseSession FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Order = item.GetInt("order"),
		WorkoutSessionId = item.GetInt("workoutSessionId"),
		ExerciseId = item.GetInt("exerciseId"),
		CreatedAt = item.GetDate("createdAt"),
		UserId = item.GetInt("userId"),
		WorkoutId = item.GetInt("workoutId"),
		SessionDate = item.GetDate("sessionDate"),
	};

	public Item ToItem() => new()
	{
		["workoutSessionId"] = Dyn.N(WorkoutSessionId),
		["id"] = Dyn.N(Id),
		["order"] = Dyn.N(Order),
		["exerciseId"] = Dyn.N(ExerciseId),
		["createdAt"] = Dyn.Date(CreatedAt),
		["userId"] = Dyn.N(UserId),
		["workoutId"] = Dyn.N(WorkoutId),
		["sessionDate"] = Dyn.Date(SessionDate),
	};
}

public class SetSession
{
	public int Id { get; set; }
	public double? Weight { get; set; }
	public int? Reps { get; set; }
	public int? Duration { get; set; }
	public double? Distance { get; set; }
	public int ExerciseSessionId { get; set; }
	public DateTime CreatedAt { get; set; }

	public static SetSession FromItem(Item item) => new()
	{
		Id = item.GetInt("id"),
		Weight = item.GetDoubleOrNull("weight"),
		Reps = item.GetIntOrNull("reps"),
		Duration = item.GetIntOrNull("duration"),
		Distance = item.GetDoubleOrNull("distance"),
		ExerciseSessionId = item.GetInt("exerciseSessionId"),
		CreatedAt = item.GetDate("createdAt"),
	};

	public Item ToItem()
	{
		var item = new Item
		{
			["exerciseSessionId"] = Dyn.N(ExerciseSessionId),
			["id"] = Dyn.N(Id),
			["createdAt"] = Dyn.Date(CreatedAt),
		};

		item.SetIfNotNull("weight", Weight);
		item.SetIfNotNull("reps", Reps);
		item.SetIfNotNull("duration", Duration);
		item.SetIfNotNull("distance", Distance);

		return item;
	}
}
