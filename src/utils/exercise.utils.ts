import { prisma } from "../prisma/client.prisma.js";
import { AppError } from "./AppError.utils.js";

type SessionWithSets = {
	sets: {
		weight: number | null;
		reps: number | null;
		duration: number | null;
		distance: number | null;
	}[];
};

export function calculateAverages(
	sessions: SessionWithSets[],
	exercise: { isWeight: boolean; isReps: boolean; isDuration: boolean; isDistance: boolean },
) {
	const sessionAverages = sessions
		.filter((s) => s.sets.length > 0)
		.map((s) => {
			const avg = (key: "weight" | "reps" | "duration" | "distance") => {
				const valid = s.sets.filter((set) => set[key] !== null);
				return valid.length > 0
					? valid.reduce((sum, set) => sum + (set[key] as number), 0) / valid.length
					: null;
			};

			return {
				weight: exercise.isWeight ? avg("weight") : null,
				reps: exercise.isReps ? avg("reps") : null,
				duration: exercise.isDuration ? avg("duration") : null,
				distance: exercise.isDistance ? avg("distance") : null,
			};
		});

	const count = sessionAverages.length;

	return {
		weight: exercise.isWeight
			? sessionAverages.reduce((sum, s) => sum + (s.weight ?? 0), 0) / count || 0
			: null,
		reps: exercise.isReps
			? sessionAverages.reduce((sum, s) => sum + (s.reps ?? 0), 0) / count || 0
			: null,
		duration: exercise.isDuration
			? sessionAverages.reduce((sum, s) => sum + (s.duration ?? 0), 0) / count || 0
			: null,
		distance: exercise.isDistance
			? sessionAverages.reduce((sum, s) => sum + (s.distance ?? 0), 0) / count || 0
			: null,
	};
}

export async function assertWorkoutAccess(workoutId: number, userId: number) {
	const workout = await prisma.workout.findFirst({
		where: { id: workoutId, userId },
		select: { id: true },
	});

	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");
}

export async function assertExerciseAccess(exerciseId: number, userId: number) {
	const exercise = await prisma.exercise.findFirst({
		where: {
			id: exerciseId,
			workout: {
				userId,
			},
		},
	});

	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	return exercise;
}
