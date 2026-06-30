import { z } from "zod";

const ALLOWED_FIELDS = ["order", "description", "name"] as const;

export const idSchema = z.object({
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
	z.object({ field: z.literal("order"), order: z.number().int().nonnegative() }),
	z.object({ field: z.literal("description"), description: z.string().max(500) }),
	z.object({ field: z.literal("name"), name: createSchema.shape.name }),
]);
