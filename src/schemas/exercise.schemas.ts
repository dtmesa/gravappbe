import { z } from "zod";

const ALLOWED_FIELDS = [
	"description",
	"order",
	"name",
	"isWeight",
	"isDuration",
	"isReps",
	"isDistance",
] as const;

export const excludeIdSchema = z.object({
	excludeSessionId: z.coerce.number().int().positive(),
});

export const workoutIdSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const exerciseIdsSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
	id: z.coerce.number().int().positive(),
});

export const createSchema = z.object({
	name: z
		.string()
		.min(1)
		.max(75)
		.refine((val) => val === val.trim(), "Name cannot start or end with spaces"),
});

export const patchSchema = z.object({
	field: z.enum(ALLOWED_FIELDS),
});

export const patchBodySchema = z.discriminatedUnion("field", [
	z.object({ field: z.literal("description"), description: z.string().max(500) }),
	z.object({ field: z.literal("order"), order: z.number().int().nonnegative() }),
	z.object({ field: z.literal("isWeight"), isWeight: z.boolean() }),
	z.object({ field: z.literal("isDuration"), isDuration: z.boolean() }),
	z.object({ field: z.literal("isReps"), isReps: z.boolean() }),
	z.object({ field: z.literal("isDistance"), isDistance: z.boolean() }),
	z.object({ field: z.literal("name"), name: createSchema.shape.name }),
]);
