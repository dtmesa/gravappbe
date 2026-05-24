import "dotenv/config";
import { defineConfig } from "prisma/config";
import { AppError } from "./src/utils/AppError.js";

const DATABASE_URL = process.env.DATABASE_URL;
if (!DATABASE_URL) throw new AppError("Missing database URL", 500, "MISSING_DB_URL");

export default defineConfig({
	schema: "prisma/schema.prisma",
	migrations: {
		path: "prisma/migrations",
	},
	datasource: {
		url: DATABASE_URL,
	},
});
