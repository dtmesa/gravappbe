namespace Gravity.Api.Data;

/// <summary>
/// Table and index names. The prefix is supplied by SAM in deployed
/// environments and defaults to "Gravity_" for local DynamoDB.
/// </summary>
public static class Tables
{
	private static readonly string Prefix =
		Environment.GetEnvironmentVariable("TABLE_PREFIX") ?? "Gravity_";

	public static readonly string Users = $"{Prefix}Users";
	public static readonly string Usernames = $"{Prefix}Usernames";
	public static readonly string Workouts = $"{Prefix}Workouts";
	public static readonly string Exercises = $"{Prefix}Exercises";
	public static readonly string WorkoutSessions = $"{Prefix}WorkoutSessions";
	public static readonly string ExerciseSessions = $"{Prefix}ExerciseSessions";
	public static readonly string SetSessions = $"{Prefix}SetSessions";
	public static readonly string Counters = $"{Prefix}Counters";

	public static class Indexes
	{
		/// <summary>LSI on Workouts: partition userId, sort order.</summary>
		public const string WorkoutsByOrder = "order-index";

		/// <summary>LSI on Exercises: partition workoutId, sort order.</summary>
		public const string ExercisesByOrder = "order-index";

		/// <summary>LSI on WorkoutSessions: partition userId, sort date.</summary>
		public const string SessionsByDate = "date-index";

		/// <summary>GSI on WorkoutSessions: partition workoutId, sort date.</summary>
		public const string SessionsByWorkout = "workoutId-date-index";

		/// <summary>GSI on ExerciseSessions: partition exerciseId, sort sessionDate.</summary>
		public const string ExerciseSessionsByExercise = "exerciseId-sessionDate-index";
	}
}
