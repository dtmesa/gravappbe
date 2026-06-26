import { z } from "zod";

const passwordSchema = z
	.string()
	.min(8, "Password must be at least 8 characters.")
	.max(32, "Password must be fewer than 32 characters.")
	.refine((val) => val === val.trim(), "Password cannot start or end with spaces");

const usernameSchema = z
	.string()
	.min(3, "Username must be at least 3 characters")
	.max(20, "Username must be fewer than 20 characters")
	.refine((val) => val === val.trim(), "Username cannot start or end with spaces");

export const registerSchema = z.object({
	username: usernameSchema,
	password: passwordSchema,
});

export const loginSchema = z.object({
	username: z.string().min(1),
	password: z.string().min(1),
});

export const updateUsernameSchema = z.object({
	newUsername: usernameSchema,
	password: z.string().min(1),
});

export const updatePasswordSchema = z.object({
	currentPassword: z.string().min(1),
	newPassword: passwordSchema,
});

export const deleteAccountSchema = z.object({
	password: z.string().min(1, "Password is required"),
});