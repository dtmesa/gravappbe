import { Router } from "express";
import { authMiddleware } from "../middleware/auth.middleware.js";
import { prisma } from "../prisma/client.prisma.js";
import {
	createWorkoutSchema,
	patchWorkoutBodySchema,
	patchWorkoutSchema,
	workoutParamsSchema,
} from "../schemas/workout.schemas.js";
import { AppError } from "../utils/AppError.utils.js";

const router = Router();

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { name } = createWorkoutSchema.parse(req.body);
	const userId = req.user.userId;

	const workout = await prisma.$transaction(async (tx) => {
		await tx.workout.updateMany({
			where: { userId },
			data: { order: { increment: 1 } },
		});
		return tx.workout.create({ data: { name, userId, order: 0 } });
	});

	res.status(201).json(workout);
});

router.get("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const userId = req.user.userId;

	const workouts = await prisma.workout.findMany({
		where: { userId },
		orderBy: { order: "asc" },
	});

	res.json(workouts);
});

router.get("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: workoutId } = workoutParamsSchema.parse(req.params);
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({
		where: {
			id: workoutId,
			userId,
		},
	});

	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	res.json(workout);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: workoutId } = workoutParamsSchema.parse(req.params);
	const userId = req.user.userId;

	const deleted = await prisma.workout.deleteMany({ where: { id: workoutId, userId } });
	if (deleted.count === 0) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id: workoutId } = workoutParamsSchema.parse(req.params);
	const { field } = patchWorkoutSchema.parse(req.params);
	const userId = req.user.userId;

	const body = patchWorkoutBodySchema.parse({ field, ...req.body });
	const value = body[field as keyof typeof body];

	const updated = await prisma.workout.update({
		where: { id: workoutId, userId },
		data: { [field]: value },
	});

	res.json(updated);
});

export default router;
