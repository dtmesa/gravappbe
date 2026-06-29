import { z } from "zod";

export const workoutSessionParamsSchema = z.object({
	sessionId: z.coerce.number().int().positive(),
});

export const createExerciseSessionSchema = z.object({
	exerciseId: z.coerce.number().int().positive(),
});

export const sessionsParamsSchema = z.object({
	id: z.coerce.number().int().positive(),
	sessionId: z.coerce.number().int().positive(),
});
