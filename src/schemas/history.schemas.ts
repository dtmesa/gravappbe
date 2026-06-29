import { z } from "zod";

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
