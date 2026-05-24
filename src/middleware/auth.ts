import type { RequestHandler } from "express";
import jwt from "jsonwebtoken";
import { AppError } from "../utils/AppError.js";
import { JWT_SECRET } from "../utils/jwt.js";

interface jwtPayload {
	userId: number;
}

export const authMiddleware: RequestHandler = (req, _, next) => {
	const token = req.headers.authorization?.split(" ")[1];

	if (!token) throw new AppError("Token is missing", 401, "TOKEN");

	if (!JWT_SECRET) throw new AppError("JWT_SECRET is missing", 404, "SECRET_MISSING");

	try {
		const decoded = jwt.verify(token, JWT_SECRET) as jwtPayload;
		req.user = decoded;
		next();
	} catch {
		throw new AppError("Token is missing", 401, "TOKEN");
	}
};
