import bcrypt from "bcrypt";
import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import {
	loginSchema,
	registerSchema,
	updatePasswordSchema,
	updateUsernameSchema,
} from "../schemas/auth.js";
import { AppError } from "../utils/AppError.js";
import { signToken } from "../utils/jwt.js";

const router = Router();

router.post("/register", async (req, res) => {
	const { username, password } = registerSchema.parse(req.body);

	const hashed = await bcrypt.hash(password, 12);

	const user = await prisma.user.create({
		data: { username, password: hashed },
		select: {
			id: true,
			username: true,
		},
	});

	res.status(201).json(user);
});

router.post("/login", async (req, res) => {
	const { username, password } = loginSchema.parse(req.body);

	const user = await prisma.user.findUnique({ where: { username } });
	if (!user) throw new AppError("Invalid credentials", 401, "INVALID_CREDENTIALS");

	const same = await bcrypt.compare(password, user.password);
	if (!same) throw new AppError("Invalid credentials", 401, "INVALID_CREDENTIALS");

	const token = signToken(user.id);

	res.json({ token });
});

router.patch("/username", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { newUsername, password } = updateUsernameSchema.parse(req.body);
	const userId = req.user.userId;

	const user = await prisma.user.findUnique({
		where: { id: userId },
		select: { password: true },
	});

	if (!user) throw new AppError("User not found", 404, "USER_NOT_FOUND");

	const valid = await bcrypt.compare(password, user.password);

	if (!valid) throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

	const updated = await prisma.user.update({
		where: { id: userId },
		data: { username: newUsername },
	});

	return res.json({ username: updated.username });
});

router.patch("/password", authMiddleware, async (req, res) => {
	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");

	const { currentPassword, newPassword } = updatePasswordSchema.parse(req.body);
	const userId = req.user.userId;

	const user = await prisma.user.findUnique({
		where: { id: userId },
		select: { password: true },
	});

	if (!user) throw new AppError("User not found", 404, "USER_NOT_FOUND");

	const valid = await bcrypt.compare(currentPassword, user.password);
	if (!valid) throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

	if (currentPassword === newPassword)
		throw new AppError("New password must differ from current", 400, "SAME_PASSWORD");

	const hashed = await bcrypt.hash(newPassword, 12);

	await prisma.user.update({
		where: { id: userId },
		data: { password: hashed },
	});

	res.sendStatus(204);
});

export default router;
