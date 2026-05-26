import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

const ALLOWED_FIELDS = ["weight", "reps", "distance", "duration"];

router.post("/", authMiddleware, async (req, res) => {
	const exerciseSessionId = Number(req.params.exerciseSessionId);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const exerciseSession = await prisma.exerciseSession.findFirst({
		where: { id: exerciseSessionId },
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
	const exerciseSessionId = Number(req.params.exerciseSessionId);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const sets = await prisma.setSession.findMany({
		where: { exerciseSessionId },
		orderBy: { order: "asc" },
	});
	res.json(sets);
});

router.get("/:id", authMiddleware, async (req, res) => {
	const id = Number(req.params.id);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const set = await prisma.setSession.findFirst({ where: { id } });
	if (!set) throw new AppError("Set session not found", 404, "SETSESSION_NOT_FOUND");

	res.json(set);
});

router.delete("/:id", authMiddleware, async (req, res) => {
	const id = Number(req.params.id);
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const set = await prisma.setSession.findFirst({ where: { id } });
	if (!set) throw new AppError("Set session not found", 404, "SETSESSION_NOT_FOUND");

	await prisma.setSession.delete({ where: { id } });
	res.sendStatus(204);
});

router.patch("/:id/:field", authMiddleware, async (req, res) => {
	const id = Number(req.params.id);
	const field = req.params.field as string;
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	if (!ALLOWED_FIELDS.includes(field)) throw new AppError("Invalid field", 400, "INVALID_FIELD");

	const value = req.body[field];
	if (value == null) throw new AppError("Missing fields", 400, "MISSING_FIELDS");

	const set = await prisma.setSession.findFirst({ where: { id } });
	if (!set) throw new AppError("Set session not found", 404, "SETSESSION_NOT_FOUND");

	const updated = await prisma.setSession.update({ where: { id }, data: { [field]: value } });
	res.json(updated);
});

export default router;
