import { z } from "zod";

export const workoutSessionParamsSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const workoutSessionIdParamsSchema = z.object({
	id: z.coerce.number().int().positive(),
});
