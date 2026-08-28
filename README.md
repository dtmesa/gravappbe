# Fitness Tracker Backend

Backend for a cross-platform fitness tracking application, built as an ASP.NET Core Minimal API running on AWS Lambda behind API Gateway, with DynamoDB for storage. Designed for fast workout logging, workout session tracking, and relevant performance reminders.

## [Frontend Repo](https://github.com/dtmesa/gravappfe)

## Features

- Create and manage workouts and their associated exercises & sets
- User authentication with JWT
- ID-based owner authentication for API requests
- Request validation with FluentValidation
- Secure password hashing with bcrypt
- Serverless deployment via AWS SAM
- DynamoDB storage with per-entity tables partitioned by parent
- Type-safe C# codebase

## Tech Stack

- C# / .NET 8
- ASP.NET Core Minimal APIs
- AWS Lambda + API Gateway (HTTP API)
- DynamoDB
- AWS SAM
- JWT
- CORS
- bcrypt
- FluentValidation

## Running locally

DynamoDB Local stands in for the deployed tables, which are created automatically
on startup whenever `DYNAMODB_ENDPOINT` is set:

```bash
docker run -d --name gravity-dynamodb -p 8000:8000 amazon/dynamodb-local
```

```bash
dotnet run --project src/Gravity.Api --urls "http://0.0.0.0:3000"
```

Binding `0.0.0.0` lets a physical device on the same network reach the API — point
`EXPO_PUBLIC_API_URL` in the frontend at `http://<your-lan-ip>:3000`.

Configuration comes from `.env` (see `JWT_SECRET`, `DYNAMODB_ENDPOINT`,
`AWS_REGION`, `CLIENT_ORIGIN`). Setting `NODE_ENV=production` pins CORS to
`CLIENT_ORIGIN` instead of reflecting any origin.

## Deploying

```bash
sam build && sam deploy --guided
```

`JwtSecret` must match the secret previously used to sign tokens, otherwise JWTs
already stored on devices stop validating.

## Notes

The previous Node/Express/Prisma/PostgreSQL implementation is preserved in git
history on the `redisDisabled` branch. Redis-backed caching and rate limiting
have not been carried over.

## Demo Video
https://github.com/user-attachments/assets/e7e51932-2f57-41d0-beb9-382045126062

## Plans

- Cross-platform support (Android, iOS, Web)
- Customization setting features
