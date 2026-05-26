import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

const ALLOWED_FIELDS = [""];

router.post("/", authMiddleware, async (req, res) => {
	const workoutId = Number(req.params.workoutId);
	const workoutSessionId = Number(req.params.sessionId);
	const { exerciseId } = req.body;

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const workoutSession = await prisma.workoutSession.findFirst({
		where: { id: workoutSessionId, userId },
	});
	if (!workoutSession)
		throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	const session = await prisma.exerciseSession.create({ data: { workoutSessionId, exerciseId } });

	res.status(201).json(session);
});

router.get("/", authMiddleware, async (req, res) => {
	const exerciseSessionId = Number(req.params.exerciseSessionId);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const sets = await prisma.setSession.findMany({
		where: { exerciseSessionId },
		orderBy: { order: "asc" },
	});
	res.json(sets);
});

router.get("/:id", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);
	const workoutSessionId = Number(req.params.sessionId);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const workoutSession = await prisma.workoutSession.findFirst({
		where: { id: workoutSessionId, userId },
	});
	if (!workoutSession)
		throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const session = await prisma.exerciseSession.findFirst({
		where: { id: sessionId, workoutSessionId },
	});
	if (!session) throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	res.json(session);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);
	const workoutSessionId = Number(req.params.sessionId);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const workoutSession = await prisma.workoutSession.findFirst({
		where: { id: workoutSessionId, userId },
	});
	if (!workoutSession)
		throw new AppError("Workout Session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const session = await prisma.exerciseSession.findFirst({
		where: { id: sessionId, workoutSessionId },
	});
	if (!session) throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	await prisma.exerciseSession.delete({ where: { id: sessionId } });

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);
	const field = req.params.field as string;
	const workoutSessionId = Number(req.params.sessionId);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	if (!ALLOWED_FIELDS.includes(field)) throw new AppError("Invalid field", 400, "INVALID_FIELD");

	const value = req.body[field];
	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const workoutSession = await prisma.workoutSession.findFirst({
		where: { id: workoutSessionId, userId },
	});
	if (!workoutSession)
		throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const session = await prisma.exerciseSession.findFirst({
		where: { id: sessionId, workoutSessionId },
	});
	if (!session) throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	const updated = await prisma.exerciseSession.update({
		where: { id: sessionId },
		data: { [field]: value },
	});

	res.json(updated);
});

export default router;
