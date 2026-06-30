import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import { redis } from "../redis/client.redis.js";
import { queryMonthSchema } from "../schemas/history.schemas.js";
import { AppError } from "../utils/AppError.utils.js";

const router = Router();
const CACHE_TTL = 60 * 15;

router.get("/sessions", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const {
		month: { rawMonth, start, end },
	} = queryMonthSchema.parse(req.query);

	const cacheKey = `sessions:user:${userId}:month:${rawMonth}`;
	const cached = await redis.get(cacheKey);
	if (cached) return res.json(JSON.parse(cached));

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

	await redis.set(cacheKey, JSON.stringify(sessions), "EX", CACHE_TTL);
	return res.json(sessions);
});

export default router;
