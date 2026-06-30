import { z } from "zod";

const ALLOWED_FIELDS = ["date"] as const;

export const workoutIdSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const idSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const dateSchema = z.object({
	date: z.coerce.date().optional(),
});

export const patchSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const patchBodySchema = z.object({
	date: z.coerce.date().optional(),
});
