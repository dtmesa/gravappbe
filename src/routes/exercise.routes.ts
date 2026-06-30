import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import { redis } from "../redis/client.redis.js";
import {
	createSchema,
	excludeIdSchema,
	exerciseIdsSchema,
	patchBodySchema,
	patchSchema,
	workoutIdSchema,
} from "../schemas/exercise.schemas.js";
import { AppError } from "../utils/AppError.utils.js";
import {
	assertExerciseAccess,
	assertWorkoutAccess,
	calculateAverages,
} from "../utils/exercise.utils.js";

const router = Router({ mergeParams: true });
const CACHE_TTL = 60 * 15;

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId } = workoutIdSchema.parse(req.params);
	const { name } = createSchema.parse(req.body);

	if (!name) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	await assertWorkoutAccess(workoutId, userId);

	const exercise = await prisma.$transaction(async (tx) => {
		const last = await tx.exercise.findFirst({
			where: { workoutId },
			orderBy: { order: "desc" },
			select: { order: true },
		});
		return tx.exercise.create({
			data: { name, workoutId, order: (last?.order ?? 0) + 1 },
		});
	});

	res.status(201).json(exercise);
});

router.get("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId } = workoutIdSchema.parse(req.params);

	await assertWorkoutAccess(workoutId, userId);

	const exercises = await prisma.exercise.findMany({
		where: { workoutId },
		orderBy: { order: "asc" },
	});

	res.json(exercises);
});

router.get("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const exerciseId = Number(req.params.id);
	const workoutId = Number(req.params.workoutId);

	await assertWorkoutAccess(workoutId, userId);

	const exercise = await assertExerciseAccess(exerciseId, userId);

	res.json(exercise);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId, id: exerciseId } = exerciseIdsSchema.parse(req.params);

	const deleted = await prisma.exercise.deleteMany({
		where: { id: exerciseId, workoutId, workout: { userId } },
	});
	if (deleted.count === 0) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId, id: exerciseId } = exerciseIdsSchema.parse(req.params);
	const { field } = patchSchema.parse(req.params);
	const body = patchBodySchema.parse({ field, ...req.body });
	const value = body[field as keyof typeof body];

	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const updated = await prisma.exercise.update({
		where: { id: exerciseId, workoutId, workout: { userId } },
		data: { [field]: value },
	});

	res.json(updated);
});

router.get("/:id/averages", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId, id: exerciseId } = exerciseIdsSchema.parse(req.params);
	const { excludeSessionId } = excludeIdSchema.parse(req.query);

	await assertWorkoutAccess(workoutId, userId);

	const exercise = await assertExerciseAccess(exerciseId, userId);

	const cacheKey = `averages:weekly:${exerciseId}:exclude:${excludeSessionId ?? "none"}`;
	const cached = await redis.get(cacheKey);
	if (cached) return res.json(JSON.parse(cached));

	const oneWeekAgo = new Date();
	oneWeekAgo.setDate(oneWeekAgo.getDate() - 7);

	const recentSessions = await prisma.exerciseSession.findMany({
		where: {
			exerciseId,
			workoutSession: {
				date: { gte: oneWeekAgo },
				userId: userId,
				id: { not: excludeSessionId },
			},
		},
		include: {
			sets: {
				select: {
					weight: true,
					reps: true,
					duration: true,
					distance: true,
				},
			},
		},
	});
	const result = calculateAverages(recentSessions, exercise);

	await redis.set(cacheKey, JSON.stringify(result), "EX", CACHE_TTL);
	return res.json(result);
});

router.get("/:id/averages/all", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;
	const { workoutId, id: exerciseId } = exerciseIdsSchema.parse(req.params);
	const { excludeSessionId } = excludeIdSchema.parse(req.query);

	await assertWorkoutAccess(workoutId, userId);

	const exercise = await assertExerciseAccess(exerciseId, userId);

	const cacheKey = `averages:all:${exerciseId}:exclude:${excludeSessionId ?? "none"}`;
	const cached = await redis.get(cacheKey);
	if (cached) return res.json(JSON.parse(cached));

	const pastSessions = await prisma.exerciseSession.findMany({
		where: {
			exerciseId,
			workoutSession: {
				userId,
				id: { not: excludeSessionId },
			},
		},
		include: {
			sets: {
				select: {
					weight: true,
					reps: true,
					duration: true,
					distance: true,
				},
			},
		},
	});
	const result = calculateAverages(pastSessions, exercise);

	await redis.set(cacheKey, JSON.stringify(result), "EX", CACHE_TTL);
	return res.json(result);
});

export default router;
