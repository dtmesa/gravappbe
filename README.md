# Fitness Tracker Backend

Backend for a cross-platform fitness tracking application: a Go API running on AWS Lambda (arm64) behind API Gateway, with DynamoDB for storage. Designed for fast workout logging, workout session tracking, and relevant performance reminders.

## [Frontend Repo](https://github.com/dtmesa/gravappfe)

## Features

- Create and manage workouts and their associated exercises & sets
- User authentication with JWT, with password changes invalidating other sessions
- Account recovery by email (forgotten username, password reset) and email confirmation
- Ownership checks on every request
- Rate limiting backed by DynamoDB, so limits hold across Lambda instances
- Exercise averages served from running per-session aggregates, with no per-session fan-out
- Serverless deployment via AWS SAM

## Tech Stack

- Go (standard library `net/http`, no framework)
- AWS Lambda (`provided.al2023`, arm64) + API Gateway (HTTP API)
- DynamoDB
- AWS SAM
- JWT (HS256)
- bcrypt
- SES

## Layout

```
cmd/server     local HTTP server (0.0.0.0:3000)
cmd/lambda     Lambda entrypoint
internal/api   routes, middleware and handlers
internal/store data access, one file per entity
internal/...   jsonx (timestamps), httpx (errors), valid, auth, model, dynamo, mail, ratelimit, config
```

## Running locally

DynamoDB Local stands in for the deployed tables, which are created on startup whenever `DYNAMODB_ENDPOINT` is set:

```bash
docker run -d --name gravity-dynamodb -p 8000:8000 amazon/dynamodb-local
```

Then, with a `.env` (in this directory or any parent) containing at least `JWT_SECRET` and `DYNAMODB_ENDPOINT=http://localhost:8000`:

```bash
go run ./cmd/server
```

It listens on `0.0.0.0:3000` so a phone on the same network can reach it — point `EXPO_PUBLIC_API_URL` in the frontend at `http://<your-lan-ip>:3000`. Set `HOST=127.0.0.1` to bind loopback only.

Emails are logged to the console by default (`EMAIL_SENDER=console`), so the whole recovery and confirmation flow, including reading codes, works without AWS credentials.

## Testing

```bash
go test ./...                                              # unit tests
DYNAMODB_ENDPOINT=http://localhost:8000 go test ./...      # + integration tests
```

Integration tests run the real API against DynamoDB Local, each run under its own random table prefix (dropped afterwards). They skip when `DYNAMODB_ENDPOINT` is unset.

## Deploying

The JWT signing key lives in Secrets Manager, read at deploy time. Create it once:

```bash
aws secretsmanager create-secret --name gravity/jwt-secret --secret-string "$(openssl rand -hex 32)"
```

Then build and deploy — without `sam build`; `template.yaml`'s `CodeUri` points at the build output:

```bash
./build.sh
sam deploy --template template.yaml --stack-name gravity-api --capabilities CAPABILITY_IAM --resolve-s3 \
  --parameter-overrides TablePrefix=Gravity_ Environment=development EmailSender=ses EmailFromAddress=<ses-verified-address>
```

Deploy to the **existing** stack. The app has its API URL compiled in, so a new stack (or an explicit API resource) would strand every installed copy.

Changing the secret invalidates every issued token: users are logged out once and sign back in with their existing passwords.

## Notes

- `EMAIL_SENDER=ses` needs SES out of the **sandbox** to reach real users; in the sandbox it only delivers to verified addresses. Check the SES console before assuming a delivery failure is a bug here.
- Inside Lambda, SES is the default sender — the console sender logs codes and is never selected by omission there.
- Responses are wire-compatible with the previous C# backend, which the current app build depends on: timestamps always carry three fractional digits, nullable fields are explicit `null`s, and every error is `{"error": "CODE"}`.
- Additive endpoints the current app does not use yet: sparse-body `PATCH /workouts/{id}` (and the exercise and set equivalents), `PUT /workouts/order`, `PUT /workouts/{id}/exercises/order`, `POST …/exerciseSessions/bulk`, and a `token` in the register response.

## Demo Video
https://github.com/user-attachments/assets/e7e51932-2f57-41d0-beb9-382045126062
