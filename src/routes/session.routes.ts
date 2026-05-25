import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

const ALLOWED_FIELDS = [""];

router.post("/", authMiddleware, async (req, res) => {
	const workoutId = Number(req.params.workoutId);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
	if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const session = await prisma.workoutSession.create({ data: { workoutId, userId } });

	res.status(201).json(session);
});

router.get("/", authMiddleware, async (req, res) => {
    const workoutSessionId = Number(req.params.sessionId);

    if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
    const userId = req.user.userId;

    const workoutSession = await prisma.workoutSession.findFirst({
        where: { id: workoutSessionId, userId },
    });
    if (!workoutSession) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

    const sessions = await prisma.exerciseSession.findMany({
        where: { workoutSessionId },
        orderBy: { order: "asc" },
    });

    res.json(sessions);
});

router.get("/:id", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const session = await prisma.workoutSession.findFirst({ where: { id: sessionId, userId } });
	if (!session) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	res.json(session);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const session = await prisma.workoutSession.findFirst({ where: { id: sessionId, userId } });
	if (!session) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	await prisma.workoutSession.delete({ where: { id: sessionId } });

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	const sessionId = Number(req.params.id);
	const field = req.params.field as string;
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	if (!ALLOWED_FIELDS.includes(field)) throw new AppError("Invalid field", 400, "INVALID_FIELD");

	const value = req.body[field];
	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const session = await prisma.workoutSession.findFirst({ where: { id: sessionId, userId } });
	if (!session) throw new AppError("Workout session not found", 404, "WORKOUTSESSION_NOT_FOUND");

	const updated = await prisma.workoutSession.update({
		where: { id: sessionId },
		data: { [field]: value },
	});

	res.json(updated);
});

export default router;
