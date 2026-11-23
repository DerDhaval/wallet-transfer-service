package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wallet-transfer-service/internal/config"
	"wallet-transfer-service/internal/handler"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/router"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/pkg/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("shutdown", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Errors only — no request/access noise in the terminal.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}

	// main → router → middleware → handler → service → repository → pkg/database
	store := repository.New(pool)
	svc := service.New(store)
	h := handler.New(svc, pool.Ping)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router.New(h),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Println("api listening on", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
