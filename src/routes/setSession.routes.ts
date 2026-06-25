import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import {
	patchSetSessionBodySchema,
	patchSetSessionSchema,
	setIDParamsSchema,
	setSessionParamsSchema,
} from "../schemas/setSession.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

router.post("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { exerciseSessionId } = setSessionParamsSchema.parse(req.params);

	const exerciseSession = await prisma.exerciseSession.findFirst({
		where: {
			id: exerciseSessionId,
			workoutSession: { userId: req.user.userId },
		},
		include: { exercise: true },
	});
	if (!exerciseSession)
		throw new AppError("Exercise session not found", 404, "EXERCISESESSION_NOT_FOUND");

	const { exercise } = exerciseSession;

	const setSession = await prisma.setSession.create({
		data: {
			exerciseSessionId,
			weight: exercise.isWeight ? 0 : null,
			reps: exercise.isReps ? 0 : null,
			duration: exercise.isDuration ? 0 : null,
			distance: exercise.isDistance ? 0 : null,
		},
	});
	res.status(201).json(setSession);
});

router.get("/", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { exerciseSessionId } = setSessionParamsSchema.parse(req.params);

	const sets = await prisma.setSession.findMany({
		where: {
			exerciseSessionId,
			exerciseSession: { workoutSession: { userId: req.user.userId } },
		},
		orderBy: { id: "asc" },
	});
	res.json(sets);
});

router.get("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id } = setIDParamsSchema.parse(req.params);
	const userId = req.user.userId;

	const set = await prisma.setSession.findFirst({
		where: {
			id,
			exerciseSession: {
				workoutSession: { userId },
			},
		},
	});
	if (!set) throw new AppError("Set session not found", 404, "SETSESSION_NOT_FOUND");

	res.json(set);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id } = setIDParamsSchema.parse(req.params);
	const userId = req.user.userId;

	const deleted = await prisma.setSession.deleteMany({
		where: {
			id,
			exerciseSession: { workoutSession: { userId } },
		},
	});
	if (deleted.count === 0) throw new AppError("Set session not found", 404, "SETSESSION_NOT_FOUND");

	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { id } = setIDParamsSchema.parse(req.params);
	const { field } = patchSetSessionSchema.parse(req.params);
	const userId = req.user.userId;

	const body = patchSetSessionBodySchema.parse({ field, ...req.body });
	const value = body[field as keyof typeof body];

	const updated = await prisma.setSession.update({
		where: {
			id,
			exerciseSession: { workoutSession: { userId } },
		},
		data: { [field]: value },
	});
	res.json(updated);
});

export default router;
