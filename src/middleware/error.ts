import { Prisma } from "@prisma/client";
import type { NextFunction, Request, Response } from "express";
import { AppError } from "../utils/AppError.js";

export function errorMiddleware(err: unknown, _: Request, res: Response, __: NextFunction) {
	if (err instanceof Prisma.PrismaClientKnownRequestError) {
		if (err.code === "P2002") {
			return res.status(409).json({
				error: "USERNAME_TAKEN",
			});
		}
	}

	if (err instanceof AppError) {
		return res.status(err.statusCode).json({
			error: err.code ?? err.message,
		});
	}

	return res.status(500).json({
		error: "INTERNAL_SERVER_ERROR",
	});
}
