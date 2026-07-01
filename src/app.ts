import cors from "cors";
import express from "express";
import { errorMiddleware } from "./middleware/error.middleware.js";
import authRoutes from "./routes/auth.routes.js";
import exerciseRoutes from "./routes/exercise.routes.js";
import exerciseSessionRoutes from "./routes/exerciseSession.routes.js";
import historyRoutes from "./routes/history.routes.js";
import setSessionRoutes from "./routes/setSession.routes.js";
import workoutRoutes from "./routes/workout.routes.js";
import workoutSessionRoutes from "./routes/workoutSession.routes.js";

const app = express();

const allowedOrigins =
	process.env.NODE_ENV === "production"
		? [process.env.CLIENT_ORIGIN as string]
		: [true];

if (process.env.NODE_ENV === "production" && !process.env.CLIENT_ORIGIN) 
	throw new Error("CLIENT_ORIGIN must be set in production");

app.use(
	cors({
		origin: allowedOrigins,
		methods: ["GET", "POST", "PATCH", "DELETE"],
	}),
);
app.use(express.json());

app.use("/auth", authRoutes);
app.use("/history", historyRoutes);
app.use("/workouts", workoutRoutes);
app.use("/workouts/:workoutId/exercises", exerciseRoutes);
app.use("/workouts/:workoutId/sessions", workoutSessionRoutes);
app.use("/workouts/:workoutId/sessions/:sessionId/exerciseSessions", exerciseSessionRoutes);
app.use(
	"/workouts/:workoutId/sessions/:sessionId/exerciseSessions/:exerciseSessionId/setSessions",
	setSessionRoutes,
);

app.use(errorMiddleware);

export default app;
