import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import {
	exerciseIdSchema,
	sessionsIdsSchema,
	workoutSessionIdSchema,
} from "../schemas/exerciseSession.schemas.js";
import { AppError } from "../utils/AppError.utils.js";

const router = Router({ mergeParams: true });

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { sessionId: workoutSessionId } = workoutSessionIdSchema.parse(req.params);
	const { exerciseId } = exerciseIdSchema.parse(req.body);
	const userId = req.user.userId;

	const workoutSession = await prisma.workoutSession.findFirst({
		where: { id: workoutSessionId, userId },
	});
	if (!workoutSession)
		throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({
		where: { id: exerciseId, workout: { userId } },
	});
	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	const session = await prisma.exerciseSession.create({ data: { workoutSessionId, exerciseId } });

	res.status(201).json(session);
});

router.get("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId, sessionId: workoutSessionId } = sessionsIdsSchema.parse(req.params);
	const userId = req.user.userId;

	const session = await prisma.exerciseSession.findFirst({
		where: {
			id: sessionId,
			workoutSessionId,
			workoutSession: { userId },
		},
	});
	if (!session) throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	res.json(session);
});

router.get("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { sessionId: workoutSessionId } = workoutSessionIdSchema.parse(req.params);
	const userId = req.user.userId;

	const sessions = await prisma.exerciseSession.findMany({
		where: {
			workoutSessionId,
			workoutSession: { userId },
		},
	});

	res.json(sessions);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId, sessionId: workoutSessionId } = sessionsIdsSchema.parse(req.params);
	const userId = req.user.userId;

	const deleted = await prisma.exerciseSession.deleteMany({
		where: {
			id: sessionId,
			workoutSessionId,
			workoutSession: { userId },
		},
	});
	if (deleted.count === 0)
		throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	res.sendStatus(204);
});

router.get("/:id/previous-set-count", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: sessionId, sessionId: workoutSessionId } = sessionsIdsSchema.parse(req.params);
	const userId = req.user.userId;

	const session = await prisma.exerciseSession.findFirst({
		where: {
			id: sessionId,
			workoutSessionId,
			workoutSession: { userId },
		},
		include: { workoutSession: true },
	});
	if (!session) throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	const previous = await prisma.exerciseSession.findFirst({
		where: {
			exerciseId: session.exerciseId,
			workoutSession: { workoutId: session.workoutSession.workoutId, userId },
			id: { not: sessionId },
		},
		orderBy: { workoutSession: { date: "desc" } },
		include: { _count: { select: { sets: true } } },
	});

	res.json({ count: previous?._count.sets ?? 1 });
});

export default router;
