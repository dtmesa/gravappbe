import bcrypt from "bcrypt";
import { Router } from "express";
import { authMiddleware } from "../middleware/auth.js";
import { prisma } from "../prisma/client.js";
import { AppError } from "../utils/AppError.js";
import { signToken } from "../utils/jwt.js";

const router = Router();

router.post("/register", async (req, res) => {
	const { username, password } = req.body;

	const hashed = await bcrypt.hash(password, 12);

	const user = await prisma.user.create({
		data: { username, password: hashed },
	});

	res.json(user);
});

router.post("/login", async (req, res) => {
	const { username, password } = req.body;

	const user = await prisma.user.findUnique({ where: { username } });
	if (!user) throw new AppError("User not found", 404, "USER_NOT_FOUND");

	const same = await bcrypt.compare(password, user.password);
	if (!same) throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

	const token = signToken(user.id);

	res.json({ token });
});

router.put("/username", authMiddleware, async (req, res) => {
	const { newUsername, password } = req.body;

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const user = await prisma.user.findUnique({ where: { id: userId } });

	if (!user) throw new AppError("User not found", 404, "USER_NOT_FOUND");

	const valid = await bcrypt.compare(password, user.password);

	if (!valid) throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

	const updated = await prisma.user.update({
		where: { id: userId },
		data: { username: newUsername },
	});

	return res.json({ username: updated.username });
});

router.put("/password", authMiddleware, async (req, res) => {
	const { currentPassword, newPassword } = req.body;

	if (!req.user) throw new AppError("Unauthorized", 401, "UNAUTHORIZED");
	const userId = req.user.userId;

	const user = await prisma.user.findUnique({ where: { id: userId } });

	if (!user) throw new AppError("User not found", 404, "USER_NOT_FOUND");

	const valid = await bcrypt.compare(currentPassword, user.password);
	if (!valid) throw new AppError("Incorrect password", 401, "INVALID_PASSWORD");

	const hashed = await bcrypt.hash(newPassword, 12);

	await prisma.user.update({
		where: { id: userId },
		data: { password: hashed },
	});

	res.sendStatus(204);
});

export default router;
