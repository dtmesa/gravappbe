import { RateLimiterRedis } from "rate-limiter-flexible";
import { redis } from "./client.js";

export const loginLimiter = new RateLimiterRedis({
	storeClient: redis,
	keyPrefix: "login",
	points: 5,
	duration: 300,
	blockDuration: 300,
});

export const passwordLimiter = new RateLimiterRedis({
	storeClient: redis,
	keyPrefix: "password",
	points: 5,
	duration: 300,
	blockDuration: 300,
});

export const registerLimiter = new RateLimiterRedis({
	storeClient: redis,
	keyPrefix: "register",
	points: 10,
	duration: 300,
	blockDuration: 600,
});
