package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chaos-sec/backend/internal/attack"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/database"
	"github.com/chaos-sec/backend/internal/experiment"
	"github.com/chaos-sec/backend/internal/kubernetes"
	"github.com/chaos-sec/backend/internal/router"
	"github.com/chaos-sec/backend/internal/siem"
	"github.com/chaos-sec/backend/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Build-time variables set via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Determine subcommand: "migrate" to run DB migrations, "serve" (default) to start the server.
	subcommand := "serve"
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	// Load configuration from environment variables.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Build the structured logger based on configuration.
	logger, err := cfg.BuildLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create logger: %v\n", err)
		os.Exit(1)
	}

	// Replace the global zap logger so libraries using zap.L() also get our config.
	zap.ReplaceGlobals(logger)

	// Log startup information.
	logger.Info("starting Chaos-Sec backend",
		zap.String("version", Version),
		zap.String("commit", Commit),
		zap.String("build_date", BuildDate),
		zap.String("subcommand", subcommand),
		zap.String("environment", func() string {
			if cfg.IsDevelopment() {
				return "development"
			}
			return "production"
		}()),
	)

	switch subcommand {
	case "migrate":
		if err := runMigrations(cfg, logger); err != nil {
			logger.Fatal("migration failed", zap.Error(err))
		}
		logger.Info("migrations completed successfully")

	case "serve":
		if err := serve(cfg, logger); err != nil {
			logger.Fatal("server exited with error", zap.Error(err))
		}

	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\nUsage: %s [migrate|serve]\n", subcommand, os.Args[0])
		os.Exit(1)
	}
}

// runMigrations connects to the database and applies all pending migrations.
func runMigrations(cfg *config.Config, logger *zap.Logger) error {
	db, err := database.New(&cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	logger.Info("running database migrations",
		zap.String("migrations_path", cfg.Database.MigrationsPath),
	)

	if err := db.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Log the current migration version.
	version, dirty, err := db.MigrationVersion()
	if err != nil {
		logger.Warn("failed to get migration version after migration", zap.Error(err))
	} else {
		logger.Info("current migration version",
			zap.Uint("version", version),
			zap.Bool("dirty", dirty),
		)
	}

	return nil
}

// serve initializes all dependencies, starts the HTTP server, and handles
// graceful shutdown on SIGINT/SIGTERM signals.
func serve(cfg *config.Config, logger *zap.Logger) error {
	// ── Initialize Database ───────────────────────────────────────────────
	db, err := database.New(&cfg.Database, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer func() {
		logger.Info("closing database connection pool")
		if closeErr := db.CloseWithTimeout(10 * time.Second); closeErr != nil {
			logger.Error("error closing database", zap.Error(closeErr))
		}
	}()

	logger.Info("database connection pool initialized")

	// ── Initialize Redis ──────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.RedisAddr(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 3,
	})

	// Verify Redis connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("Redis connection failed — running without cache/session support",
			zap.Error(err),
			zap.String("addr", cfg.Redis.RedisAddr()),
		)
		// Do not fail startup: the application can function without Redis
		// (rate limiting falls back to in-memory, token blacklisting is skipped).
		rdb = nil
	} else {
		logger.Info("Redis connection established",
			zap.String("addr", cfg.Redis.RedisAddr()),
			zap.Int("db", cfg.Redis.DB),
		)
	}

	// ── Run Auto-Migration (if enabled) ──────────────────────────────────
	// In development, automatically apply pending migrations on startup.
	if cfg.IsDevelopment() {
		logger.Info("auto-running database migrations (development mode)")
		if migrateErr := db.RunMigrations(); migrateErr != nil {
			logger.Error("auto-migration failed — server starting anyway",
				zap.Error(migrateErr),
			)
		}
	}

	// ── Initialize SIEM Connector ────────────────────────────────────────
	var siemConnector siem.SIEMConnector
	var siemValidator *siem.Validator
	if cfg.SIEM.Enabled {
		siemCfg := siem.SIEMConfig{
			Endpoint:   cfg.SIEM.Endpoint,
			APIKey:     cfg.SIEM.APIKey,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		}
		provider := cfg.SIEM.Provider
		if provider == "" {
			provider = "mock"
		}
		var siemErr error
		siemConnector, siemErr = siem.NewSIEMConnector(provider, siemCfg)
		if siemErr != nil {
			logger.Warn("SIEM connector initialization failed, running without SIEM validation", zap.Error(siemErr))
		} else {
			siemValidator = siem.NewValidator(siemConnector,
				siem.WithPropagationDelay(30*time.Second),
			)
			logger.Info("SIEM connector initialized",
				zap.String("provider", provider),
				zap.String("endpoint", cfg.SIEM.Endpoint),
			)
		}
	} else {
		// Use mock SIEM in development for testing
		if cfg.IsDevelopment() {
			mockCfg := siem.SIEMConfig{Endpoint: "mock://local", Timeout: 5 * time.Second, MaxRetries: 1}
			siemConnector, _ = siem.NewSIEMConnector("mock", mockCfg)
			siemValidator = siem.NewValidator(siemConnector, siem.WithPropagationDelay(5*time.Second))
			logger.Info("mock SIEM connector initialized (development mode)")
		}
	}

	// ── Initialize SIEM Handler ──────────────────────────────────────────
	var siemHandler *siem.Handler
	if siemConnector != nil {
		siemCfg := siem.SIEMConfig{
			Endpoint:   cfg.SIEM.Endpoint,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		}
		siemHandler = siem.NewHandler(siemConnector, siemCfg, logger)
	}

	// siemHandler is wired into the router below.

	// ── Initialize Kubernetes Client Manager ─────────────────────────────
	k8sHandler := kubernetes.NewHandler(db.DB, cfg, logger)

	// ── Initialize Experiment Engine ─────────────────────────────────────
	expEngine := experiment.NewEngine(db.DB, rdb, k8sHandler.ClientManagerRef(), siemValidator, logger)
	logger.Info("experiment engine initialized")

	// ── Initialize Experiment Scheduler ──────────────────────────────────
	expScheduler := experiment.NewScheduler(expEngine, db.DB, rdb, logger,
		experiment.WithPollInterval(10*time.Second),
		experiment.WithMaxConcurrent(cfg.Kubernetes.MaxConcurrent),
	)

	// ── Initialize Attack Module Registry ──────────────────────────────────
	attackRegistry := attack.NewRegistry()
	attackHandler := attack.NewHandler(db.DB, attackRegistry, logger)
	logger.Info("attack module registry initialized",
		zap.Int("module_count", len(attackRegistry.List())),
	)

	// ── Initialize Experiment Queue ────────────────────────────────────────
	expQueue := experiment.NewQueue(rdb, logger)

	// ── Initialize Worker Pool ────────────────────────────────────────────
	expProcessor := worker.NewProcessor(expEngine, db.DB, rdb, logger)
	workerPool := worker.NewPool(cfg.Kubernetes.MaxConcurrent, expProcessor, expQueue, rdb, logger)

	// ── Initialize Router ────────────────────────────────────────────────
	r, err := router.New(cfg, db, rdb, logger,
		router.WithKubernetesHandler(k8sHandler),
		router.WithSIEMHandler(siemHandler),
		router.WithExperimentEngine(expEngine),
		router.WithScheduler(expScheduler),
		router.WithAttackHandler(attackHandler),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize router: %w", err)
	}

	engine := r.Engine()

	// ── Configure HTTP Server ────────────────────────────────────────────
	addr := cfg.Addr()
	srv := &http.Server{
		Addr:         addr,
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		// Set MaxHeaderBytes to 1MB to prevent overly large headers.
		MaxHeaderBytes: 1 << 20,
	}

	// ── Start Scheduler ──────────────────────────────────────────────────
	expScheduler.Start()
	logger.Info("experiment scheduler started")

	// ── Start Worker Pool ──────────────────────────────────────────────────
	workerPool.Start()
	logger.Info("worker pool started",
		zap.Int("worker_count", workerPool.WorkerCount()),
	)

	// ── Start Server in Goroutine ────────────────────────────────────────
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting",
			zap.String("address", addr),
			zap.String("mode", gin.Mode()),
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- fmt.Errorf("HTTP server error: %w", err)
		}
		close(serverErr)
	}()

	// ── Graceful Shutdown ────────────────────────────────────────────────
	// Wait for interrupt signal (SIGINT or SIGTERM) or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("shutdown signal received",
			zap.String("signal", sig.String()),
		)
	case err := <-serverErr:
		if err != nil {
			logger.Error("server exited unexpectedly", zap.Error(err))
			return err
		}
	}

	// Begin graceful shutdown with a 30-second deadline.
	logger.Info("shutting down gracefully — waiting for in-flight requests to complete")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown the HTTP server (stops accepting new connections, waits for
	// in-flight requests to finish up to the context deadline).
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server forced shutdown", zap.Error(err))
	}

	// Stop the experiment scheduler.
	logger.Info("stopping experiment scheduler")
	expScheduler.Stop()

	// Stop the worker pool.
	logger.Info("stopping worker pool")
	workerPool.Stop()

	// Close Kubernetes client connections.
	logger.Info("closing Kubernetes client connections")
	k8sHandler.Close()

	// Close Redis connection.
	if rdb != nil {
		if err := rdb.Close(); err != nil {
			logger.Error("error closing Redis connection", zap.Error(err))
		} else {
			logger.Info("Redis connection closed")
		}
	}

	// Close database connection pool with timeout.
	if err := db.CloseWithTimeout(10 * time.Second); err != nil {
		logger.Error("error closing database connection pool", zap.Error(err))
	} else {
		logger.Info("database connection pool closed")
	}

	// Wait briefly for any goroutines to finish.
	select {
	case <-shutdownCtx.Done():
		logger.Warn("shutdown deadline exceeded — some connections may have been dropped")
	default:
		logger.Info("shutdown completed gracefully")
	}

	return nil
}
