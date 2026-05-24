import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

const ALLOWED_FIELDS = ["description", "order", "isWeight", "isDuration", "isReps", "isDistance"];

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;
	const workoutId = Number(req.params.workoutId);
	const { name } = req.body;

	if (!name) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const lastExercise = await prisma.exercise.findFirst({
		where: { workoutId },
		orderBy: { order: "desc" },
		select: { order: true },
	});

	const exercise = await prisma.exercise.create({
		data: {
			name,
			workoutId,
			order: (lastExercise?.order ?? 0) + 1,
		},
	});

	res.status(201).json(exercise);
});

router.get("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;
	const workoutId = Number(req.params.workoutId);

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

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

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	res.json(exercise);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	const exerciseId = Number(req.params.id);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;
	const workoutId = Number(req.params.workoutId);

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	await prisma.exercise.delete({
		where: { id: exerciseId, workoutId },
	});

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;
	const workoutId = Number(req.params.workoutId);
	const exerciseId = Number(req.params.id);
	const field = req.params.field as string;

	if (!ALLOWED_FIELDS.includes(field)) throw new AppError("Invalid field", 400, "INVALID_FIELD");

	const value = req.body[field];

	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
	if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

	const updated = await prisma.exercise.update({
		where: { id: exerciseId },
		data: { [field]: value },
	});

	res.json(updated);
});

export default router;
