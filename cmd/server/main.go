// Command server runs the API as a plain HTTP server for local development.
//
// It binds 0.0.0.0:3000 by default so a phone on the same network can reach it
// (point the app's EXPO_PUBLIC_API_URL at http://<lan-ip>:3000). Set HOST to
// 127.0.0.1 for loopback only. With DYNAMODB_ENDPOINT set, the tables are
// created in DynamoDB Local on startup.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dtmesa/gravappbe/internal/api"
	"github.com/dtmesa/gravappbe/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config.LoadDotEnv()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler, db, err := api.Wire(ctx, cfg)
	if err != nil {
		return err
	}

	if cfg.DynamoEndpoint != "" {
		if err := db.EnsureTables(ctx); err != nil {
			return err
		}
	}

	addr := net.JoinHostPort(env("HOST", "0.0.0.0"), env("PORT", "3000"))

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = srv.Shutdown(shutdown)
	}()

	slog.Info("listening", "addr", addr, "tables", cfg.TablePrefix, "email", cfg.EmailSender)

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return fallback
}
