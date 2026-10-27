// Command lambda runs the API on AWS Lambda behind an API Gateway HTTP API
// (payload format 2.0). Everything before lambda.Start runs once, in the init
// phase; each invocation is then just the HTTP handler.
//
// Build for the provided.al2023 runtime, which invokes a binary named
// bootstrap:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap ./cmd/lambda
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/dtmesa/gravappbe/internal/api"
	"github.com/dtmesa/gravappbe/internal/config"
)

func main() {
	// JSON, so CloudWatch Logs Insights can query fields directly.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	handler, _, err := api.Wire(context.Background(), cfg)
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}

	lambda.Start(httpadapter.NewV2(handler).ProxyWithContext)
}
