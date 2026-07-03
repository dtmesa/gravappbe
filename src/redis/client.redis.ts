import { Redis } from "ioredis";

export const redis = new Redis(process.env.REDIS_URL ?? "redis://localhost:6379", {
	lazyConnect: false,
	connectTimeout: 5000,
	maxRetriesPerRequest: 1,
});

redis.on("connect", () => console.log("Redis connecting"));
redis.on("ready", () => console.log("Redis ready"));
redis.on("error", (err) => console.log("Redis error", err));
redis.on("close", () => console.log("Redis closed"));
