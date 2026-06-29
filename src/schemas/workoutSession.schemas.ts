import { z } from "zod";

export const workoutIdSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const idSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const dateSchema = z.object({
	date: z.coerce.date().optional(),
});
