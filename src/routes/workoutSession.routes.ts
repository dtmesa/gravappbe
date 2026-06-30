import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import { redis } from "../redis/client.redis.js";
import {
	dateSchema,
	idSchema,
	patchBodySchema,
	patchSchema,
	workoutIdSchema,
} from "../schemas/workoutSession.schemas.js";
import { AppError } from "../utils/AppError.utils.js";

const router = Router({ mergeParams: true });

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { workoutId } = workoutIdSchema.parse(req.params);
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const { date } = dateSchema.parse(req.body);

	const session = await prisma.workoutSession.create({
		data: {
			workoutId,
			userId,
			...(date && { date: new Date(date) }),
		},
		include: { workout: true },
	});

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

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId } = idSchema.parse(req.params);
	const { field } = patchSchema.parse(req.params);
	const userId = req.user.userId;

	const body = patchBodySchema.parse({ field, ...req.body });
	const value = body[field as keyof typeof body];
	if (value === undefined) throw new AppError("Missing value for field", 400, "INVALID_BODY");

	const existing = await prisma.workoutSession.findFirst({
		where: { id: sessionId, userId },
	});
	if (!existing) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const updated = await prisma.workoutSession.update({
		where: { id: sessionId },
		data: { [field]: value },
	});

	const oldMonth = new Date(existing.date).toISOString().slice(0, 7);
	const newMonth = new Date(updated.date).toISOString().slice(0, 7);

	await redis.del(`sessions:user:${userId}:month:${oldMonth}`);
	if (newMonth !== oldMonth) {
		await redis.del(`sessions:user:${userId}:month:${newMonth}`);
	}

	res.json(updated);
});

export default router;
