import jwt from "jsonwebtoken";

export const JWT_SECRET = process.env.JWT_SECRET;

if (!JWT_SECRET) throw new Error("JWT_SECRET is missing");

export const signToken = (userId: number) => {
	return jwt.sign({ userId }, JWT_SECRET, {
		expiresIn: "7d",
	});
};

export const verifyToken = (token: string) => {
	return jwt.verify(token, JWT_SECRET);
};
