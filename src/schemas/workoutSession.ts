import { z } from "zod";

export const workoutIdSchema = z.object({
	workoutId: z.coerce.number().int().positive(),
});

export const idSchema = z.object({
	id: z.coerce.number().int().positive(),
});

export const monthQuerySchema = z.object({
	month: z
		.string()
		.regex(/^\d{4}-(0[1-9]|1[0-2])$/)
		.transform((val) => {
			const [year, month] = val.split("-").map(Number) as [number, number];
			return {
				rawMonth: val,
				start: new Date(year, month - 1, 1),
				end: new Date(year, month, 1),
			};
		}),
});
