import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";

const router = Router({ mergeParams: true });

const ALLOWED_FIELDS = ["description", "order", "isWeight", "isDuration", "isReps", "isDistance"];

type SessionWithSets = {
    sets: {
        weight: number | null;
        reps: number | null;
        duration: number | null;
        distance: number | null;
    }[];
};

function calculateAverages(sessions: SessionWithSets[], exercise: { isWeight: boolean; isReps: boolean; isDuration: boolean; isDistance: boolean }) {
    const sessionAverages = sessions
        .filter((s) => s.sets.length > 0)
        .map((s) => {
            const avg = (key: "weight" | "reps" | "duration" | "distance") => {
                const valid = s.sets.filter((set) => set[key] !== null);
                return valid.length > 0
                    ? valid.reduce((sum, set) => sum + (set[key] as number), 0) / valid.length
                    : null;
            };

            return {
                weight: exercise.isWeight ? avg("weight") : null,
                reps: exercise.isReps ? avg("reps") : null,
                duration: exercise.isDuration ? avg("duration") : null,
                distance: exercise.isDistance ? avg("distance") : null,
            };
        });

    const count = sessionAverages.length;

    return {
        weight: exercise.isWeight ? sessionAverages.reduce((sum, s) => sum + (s.weight ?? 0), 0) / count || 0 : null,
        reps: exercise.isReps ? sessionAverages.reduce((sum, s) => sum + (s.reps ?? 0), 0) / count || 0 : null,
        duration: exercise.isDuration ? sessionAverages.reduce((sum, s) => sum + (s.duration ?? 0), 0) / count || 0 : null,
        distance: exercise.isDistance ? sessionAverages.reduce((sum, s) => sum + (s.distance ?? 0), 0) / count || 0 : null,
    };
}

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

router.get("/:id/averages", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
    const userId = req.user.userId;

    const exerciseId = Number(req.params.id);
    const workoutId = Number(req.params.workoutId);

	const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
    if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

	const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
    if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

    const oneWeekAgo = new Date();
    oneWeekAgo.setDate(oneWeekAgo.getDate() - 7);

	const recentSessions = await prisma.exerciseSession.findMany({
		where: {
			exerciseId: exercise.id,
			workoutSession: {
				date: { gte: oneWeekAgo },
				userId: req.user.userId,
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

	return res.json(calculateAverages(recentSessions, exercise))
});

router.get("/:id/averages/all", authMiddleware, async (req, res) => {
    if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
    const userId = req.user.userId;

    const exerciseId = Number(req.params.id);
    const workoutId = Number(req.params.workoutId);

    const workout = await prisma.workout.findFirst({ where: { id: workoutId, userId } });
    if (!workout) throw new AppError("Workout not found", 404, "WORKOUT_NOT_FOUND");

    const exercise = await prisma.exercise.findFirst({ where: { id: exerciseId, workoutId } });
    if (!exercise) throw new AppError("Exercise not found", 404, "EXERCISE_NOT_FOUND");

    const pastSessions = await prisma.exerciseSession.findMany({
        where: {
            exerciseId: exercise.id,
            workoutSession: { userId },
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

	return res.json(calculateAverages(pastSessions, exercise))
});

export default router;
