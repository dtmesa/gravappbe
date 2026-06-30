import { z } from "zod";

export const workoutSessionIdSchema = z.object({
	sessionId: z.coerce.number().int().positive(),
});

export const exerciseIdSchema = z.object({
	exerciseId: z.coerce.number().int().positive(),
});

export const sessionsIdsSchema = z.object({
	id: z.coerce.number().int().positive(),
	sessionId: z.coerce.number().int().positive(),
});
