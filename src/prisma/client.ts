import { PrismaPg } from "@prisma/adapter-pg";
import { PrismaClient } from "@prisma/client";
import { AppError } from "../utils/AppError.js";

const DATABASE_URL = process.env.DATABASE_URL;
if (!DATABASE_URL) throw new AppError("Missing database URL", 500, "MISSING_DB_URL");

const adapter = new PrismaPg({
	connectionString: DATABASE_URL,
});

export const prisma = new PrismaClient({ adapter });
