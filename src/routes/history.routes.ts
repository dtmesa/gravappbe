import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import { queryMonthSchema } from "../schemas/history.schemas.js";
import { AppError } from "../utils/AppError.utils.js";

const router = Router();

router.get("/sessions", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const {
		month: { start, end },
	} = queryMonthSchema.parse(req.query);

	const sessions = await prisma.workoutSession.findMany({
		where: {
			userId,
			date: { gte: start, lt: end },
		},
		include: {
			workout: { select: { name: true } },
			exercises: {
				include: {
					exercise: { select: { name: true } },
					sets: true,
				},
			},
		},
		orderBy: { date: "asc" },
	});

	return res.json(sessions);
});

export default router;
