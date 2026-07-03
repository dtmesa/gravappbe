import { Redis } from "ioredis";

export const redis = new Redis(process.env.REDIS_URL ?? "redis://localhost:6379", {
	lazyConnect: false,
	connectTimeout: 5000,
	maxRetriesPerRequest: 1,
});
