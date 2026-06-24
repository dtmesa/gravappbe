import { z } from "zod";

const ALLOWED_FIELDS = ["weight", "reps", "distance", "duration"] as const;

export const setSessionParamsSchema = z.object({
	exerciseSessionId: z.coerce.number().int().positive(),
});

export const setIDParamsSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const patchSetSessionSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const patchSetSessionBodySchema = z.discriminatedUnion("field", [
	z.object({ field: z.literal("reps"), reps: z.number().int().nonnegative() }),
	z.object({ field: z.literal("weight"), weight: z.number().nonnegative() }),
	z.object({ field: z.literal("duration"), duration: z.number().nonnegative() }),
	z.object({ field: z.literal("distance"), distance: z.number().nonnegative() }),
]);
