import { z } from "zod";

const ALLOWED_FIELDS = ["weight", "reps", "distance", "duration"] as const;

export const exerciseSessionIdSchema = z.object({
	exerciseSessionId: z.coerce.number().int().positive(),
});

export const idParamsSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const patchSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const patchBodySchema = z.discriminatedUnion("field", [
	z.object({ field: z.literal("reps"), reps: z.number().int().nonnegative() }),
	z.object({ field: z.literal("weight"), weight: z.number().nonnegative() }),
	z.object({ field: z.literal("duration"), duration: z.number().nonnegative() }),
	z.object({ field: z.literal("distance"), distance: z.number().nonnegative() }),
]);
