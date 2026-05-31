package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chipmov/internal/app"
	"chipmov/internal/auth"
	"chipmov/internal/config"
	"chipmov/internal/easy2use"
	"chipmov/internal/storage"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := storage.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		logger.Error("failed to migrate database", "error", err)
		os.Exit(1)
	}

	if err := db.UpsertAllowedCNPJs(context.Background(), cfg.AllowedCNPJs); err != nil {
		logger.Error("failed to seed allowed CNPJs", "error", err)
		os.Exit(1)
	}
	if err := auth.NewService(cfg, db).BootstrapAdmin(context.Background()); err != nil {
		logger.Error("failed to bootstrap admin user", "error", err)
		os.Exit(1)
	}

	provider := easy2use.NewClient(cfg.Easy2UseBaseURL, cfg.Easy2UseUserToken, logger)
	server := app.NewServer(cfg, db, provider, logger)

	httpServer := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", cfg.AppAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}
