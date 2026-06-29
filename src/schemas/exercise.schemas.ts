import { z } from "zod";

const ALLOWED_FIELDS = [
	"description",
	"order",
	"isWeight",
	"isDuration",
	"isReps",
	"isDistance",
] as const;

export const createExerciseSchema = z.object({
	name: z
		.string()
		.min(1)
		.max(75)
		.refine((val) => val === val.trim(), "Name cannot start or end with spaces"),
});

export const patchExerciseSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const excludeSessionSchema = z.object({
	excludeSessionId: z.coerce.number().int().positive(),
});

export const workoutParamsSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const exerciseParamsSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
	id: z.coerce.number().int().positive(),
});
