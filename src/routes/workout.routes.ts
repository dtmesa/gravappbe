import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router();

const ALLOWED_FIELDS = ["order", "description"];

router.post("/", authMiddleware, async (req, res) => {
	const { name } = req.body;

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	if (!name) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	await prisma.workout.updateMany({
		where: { userId },
		data: { order: { increment: 1 } },
	});

	const workout = await prisma.workout.create({
		data: { name, userId, order: 0 },
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
	const workoutId = Number(req.params.id);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
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
	const workoutId = Number(req.params.id);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	await prisma.workout.delete({ where: { id: workoutId } });

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	const workoutId = Number(req.params.id);
	const field = req.params.field as string;
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	if (!ALLOWED_FIELDS.includes(field)) throw new AppError("Invalid field", 400, "INVALID_FIELD");

	const value = req.body[field];
	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const updated = await prisma.workout.update({
		where: { id: workoutId },
		data: { [field]: value },
	});

	res.json(updated);
});

export default router;
