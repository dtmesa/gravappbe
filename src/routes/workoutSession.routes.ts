import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { idSchema, workoutIdSchema } from "../schemas/workoutSession.js";
import { AppError } from "../utils/AppError.js";
import { redis } from "../utils/redis/client.js";

const router = Router({ mergeParams: true });

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { workoutId } = workoutIdSchema.parse(req.params);
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const session = await prisma.workoutSession.create({ data: { workoutId, userId } });

	const month = new Date(session.date).toISOString().slice(0, 7);
	await redis.del(`sessions:user:${userId}:month:${month}`);

	res.status(201).json(session);
});

router.get("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId } = idSchema.parse(req.params);
	const userId = req.user.userId;

	const session = await prisma.workoutSession.findFirst({
		where: { id: sessionId, userId },
		include: { workout: true },
	});
	if (!session) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	res.json(session);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId } = idSchema.parse(req.params);
	const userId = req.user.userId;

	const session = await prisma.workoutSession.findFirst({
		where: { id: sessionId, userId },
		include: { workout: true },
	});
	if (!session) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	await prisma.workoutSession.delete({ where: { id: sessionId } });

	const month = new Date(session.date).toISOString().slice(0, 7);
	await redis.del(`sessions:user:${userId}:month:${month}`);

	res.sendStatus(204);
});

export default router;
