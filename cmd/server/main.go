package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"video-record/internal/assets"
	"video-record/internal/auth"
	"video-record/internal/config"
	"video-record/internal/household"
	"video-record/internal/httpapi"
	"video-record/internal/integrations"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
	"video-record/internal/records"
	statsdomain "video-record/internal/stats"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
)

type syncRuntime struct {
	candidates *syncdomain.CandidateService
	scheduler  *syncdomain.Scheduler
	accounts   *integrations.AccountRepository
	jobs       *syncdomain.Service
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := healthcheck(context.Background(), os.Getenv("APP_PORT"), &http.Client{Timeout: 3 * time.Second}); err != nil {
			os.Exit(1)
		}
		return
	}
	appContext, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	db, err := storage.Open(appContext, filepath.Join(cfg.DataDir, "video-record.db"))
	if err != nil {
		logger.Error("storage unavailable", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("storage close failed", slog.Any("error", err))
		}
	}()
	if err := storage.Migrate(appContext, db); err != nil {
		logger.Error("database migration failed", slog.Any("error", err))
		os.Exit(1)
	}
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	tmdbClient := tmdb.NewClient(tmdb.ClientOptions{
		Token:  cfg.TMDBReadAccessToken,
		Cache:  tmdb.NewCache(db, nil),
		Logger: logger,
	})
	mediaService := media.NewService(media.NewRepository(db))
	recordService := records.NewService(records.NewRepository(db))
	statsService := statsdomain.NewService(statsdomain.NewRepository(db))
	householdService := household.NewService(household.NewRepository(db))
	backupManager, err := newBackupManager(
		appContext, db, filepath.Join(cfg.DataDir, "backups"), len(cfg.EncryptionKey) > 0,
	)
	if err != nil {
		logger.Error("incomplete backup cleanup failed", slog.Any("error", err))
		os.Exit(1)
	}
	syncRuntime, err := newSyncRuntime(appContext, db, cfg.EncryptionKey)
	if err != nil {
		logger.Error("sync scheduler initialization failed", slog.Any("error", err))
		os.Exit(1)
	}
	syncDone := syncRuntime.scheduler.Start(appContext)
	go func() {
		if err := <-syncDone; err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("sync scheduler stopped", slog.Any("error", err))
		}
	}()

	apiHandler := httpapi.NewRouter(httpapi.Dependencies{
		Logger:              logger,
		Storage:             db,
		Auth:                authService,
		CookieSecure:        cfg.CookieSecure,
		TMDB:                tmdbClient,
		Media:               mediaService,
		Records:             recordService,
		Stats:               statsService,
		Household:           householdService,
		Backup:              backupManager,
		Sync:                syncRuntime.candidates,
		IntegrationAccounts: syncRuntime.accounts,
		SyncJobs:            syncRuntime.jobs,
	})
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           assets.NewHandler(apiHandler),
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

func newBackupManager(
	ctx context.Context,
	db *storage.DB,
	backupsDir string,
	encryptionKeyAvailable bool,
) (*storage.BackupManager, error) {
	manager := storage.NewBackupManager(db, storage.BackupOptions{
		BackupsDir: backupsDir, EncryptionKeyAvailable: encryptionKeyAvailable,
	})
	if err := manager.CleanupIncomplete(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func newSyncRuntime(ctx context.Context, db *storage.DB, encryptionKey []byte) (syncRuntime, error) {
	candidates := syncdomain.NewCandidateService(db, syncdomain.CandidateServiceOptions{})
	accounts := integrations.NewAccountRepository(
		db,
		integrations.NewCredentialCipher(encryptionKey),
		integrations.AccountRepositoryOptions{},
	)
	service := syncdomain.NewService(db, syncdomain.ServiceOptions{})
	if err := service.EnsureEnabledAccountJobs(ctx); err != nil {
		return syncRuntime{}, err
	}
	runner := syncdomain.NewProviderRunner(
		db,
		accounts,
		candidates,
		syncdomain.NewDefaultProviderFactory(syncdomain.DefaultProviderFactoryOptions{}),
		syncdomain.ProviderRunnerOptions{},
	)
	return syncRuntime{
		candidates: candidates,
		scheduler:  syncdomain.NewScheduler(service, runner, syncdomain.SchedulerOptions{}),
		accounts:   accounts,
		jobs:       service,
	}, nil
}
