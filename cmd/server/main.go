package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"video-record/internal/config"
	"video-record/internal/httpapi"
)

func main() {
	bootstrapLogger := httpapi.NewLogger(os.Getenv("APP_ENV"), os.Stdout)
	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}

	logger := httpapi.NewLogger(
		cfg.Environment,
		os.Stdout,
		cfg.TMDBReadAccessToken,
		os.Getenv("APP_ENCRYPTION_KEY"),
	)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpapi.NewRouter(httpapi.Dependencies{Logger: logger}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server listening", slog.String("address", server.Addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}
