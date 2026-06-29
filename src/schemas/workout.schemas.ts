import { z } from "zod";

const ALLOWED_FIELDS = ["order", "description"] as const;

export const workoutParamsSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const createWorkoutSchema = z.object({
	name: z
		.string()
		.min(1)
		.max(75)
		.refine((val) => val === val.trim(), "Name cannot start or end with spaces"),
});

export const patchWorkoutSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const patchWorkoutBodySchema = z.discriminatedUnion("field", [
	z.object({ field: z.literal("order"), order: z.number().int().nonnegative() }),
	z.object({ field: z.literal("description"), description: z.string().max(500) }),
]);
