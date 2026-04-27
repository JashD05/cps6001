package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/chaos-sec/backend/internal/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// DB wraps a sql.DB connection pool with additional metadata.
type DB struct {
	*sql.DB
	logger *zap.Logger
	config *config.DatabaseConfig
}

// New creates a new database connection pool using the provided configuration.
// It validates connectivity, configures the pool, and returns a ready-to-use DB.
func New(cfg *config.DatabaseConfig, logger *zap.Logger) (*DB, error) {
	if cfg == nil {
		return nil, errors.New("database configuration is required")
	}

	logger.Info("connecting to PostgreSQL",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Name),
		zap.String("sslmode", cfg.SSLMode),
	)

	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Validate the connection is actually working
	ctx, cancel := newTimeoutContext(10 * time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connection established",
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
	)

	return &DB{
		DB:     db,
		logger: logger,
		config: cfg,
	}, nil
}

// HealthCheck verifies the database is reachable and returns connection pool stats.
// It uses a short timeout to avoid blocking on an unresponsive database.
func (db *DB) HealthCheck() error {
	ctx, cancel := newTimeoutContext(3 * time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	return nil
}

// HealthCheckDetailed performs a health check and returns detailed pool statistics.
func (db *DB) HealthCheckDetailed() (*HealthStatus, error) {
	ctx, cancel := newTimeoutContext(3 * time.Second)
	defer cancel()

	start := time.Now()
	err := db.PingContext(ctx)
	latency := time.Since(start)

	stats := db.Stats()

	status := &HealthStatus{
		Status:       "healthy",
		Latency:      latency,
		OpenConns:    stats.OpenConnections,
		IdleConns:    stats.Idle,
		InUseConns:   stats.InUse,
		WaitCount:    stats.WaitCount,
		WaitDuration: stats.WaitDuration,
		MaxOpenConns: stats.MaxOpenConnections,
		MaxIdleConns: db.config.MaxIdleConns,
	}

	if err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
		return status, fmt.Errorf("database health check failed: %w", err)
	}

	return status, nil
}

// HealthStatus represents the detailed health status of the database connection pool.
type HealthStatus struct {
	Status       string        `json:"status"`
	Latency      time.Duration `json:"latency"`
	OpenConns    int           `json:"open_connections"`
	IdleConns    int           `json:"idle_connections"`
	InUseConns   int           `json:"in_use_connections"`
	WaitCount    int64         `json:"wait_count"`
	WaitDuration time.Duration `json:"wait_duration"`
	MaxOpenConns int           `json:"max_open_connections"`
	MaxIdleConns int           `json:"max_idle_connections"`
	Error        string        `json:"error,omitempty"`
}

// RunMigrations applies all pending database migrations using golang-migrate.
// It reads migration files from the configured migrations path.
func (db *DB) RunMigrations() error {
	db.logger.Info("running database migrations",
		zap.String("migrations_path", db.config.MigrationsPath),
	)

	m, err := migrate.New(
		db.config.MigrationsPath,
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			db.config.User,
			db.config.Password,
			db.config.Host,
			db.config.Port,
			db.config.Name,
			db.config.SSLMode,
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		db.logger.Info("no new migrations to apply")
	} else {
		db.logger.Info("migrations applied successfully")
	}

	return nil
}

// RollbackLastMigration rolls back the most recently applied migration.
func (db *DB) RollbackLastMigration() error {
	db.logger.Warn("rolling back last migration")

	m, err := migrate.New(
		db.config.MigrationsPath,
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			db.config.User,
			db.config.Password,
			db.config.Host,
			db.config.Port,
			db.config.Name,
			db.config.SSLMode,
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	db.logger.Info("migration rolled back successfully")
	return nil
}

// MigrationVersion returns the current migration version and dirty state.
func (db *DB) MigrationVersion() (uint, bool, error) {
	m, err := migrate.New(
		db.config.MigrationsPath,
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			db.config.User,
			db.config.Password,
			db.config.Host,
			db.config.Port,
			db.config.Name,
			db.config.SSLMode,
		),
	)
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

// Close gracefully closes the database connection pool.
// It first checks that all in-flight queries have completed within the timeout,
// then closes all idle connections.
func (db *DB) Close() error {
	db.logger.Info("closing database connection pool")
	return db.DB.Close()
}

// CloseWithTimeout attempts a graceful shutdown of the database connection pool.
// It waits up to the specified timeout for in-flight queries to complete.
func (db *DB) CloseWithTimeout(timeout time.Duration) error {
	db.logger.Info("closing database connection pool with timeout",
		zap.Duration("timeout", timeout),
	)

	done := make(chan error, 1)
	go func() {
		done <- db.DB.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			db.logger.Error("error closing database", zap.Error(err))
			return err
		}
		db.logger.Info("database connection pool closed")
		return nil
	case <-time.After(timeout):
		db.logger.Warn("timed out waiting for database connections to close")
		return errors.New("database close timed out")
	}
}

// Stats returns a snapshot of the database connection pool statistics.
func (db *DB) Stats() sql.DBStats {
	return db.DB.Stats()
}

// newTimeoutContext is a helper that creates a context with a timeout.
// This is extracted as a function for consistent timeout handling across the package.
// In production, request contexts should be passed through where available.
func newTimeoutContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// --- Transaction helpers ---

// InTx executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function returns nil, the transaction is committed.
// This provides a clean pattern for transactional operations.
func (db *DB) InTx(fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			db.logger.Error("failed to rollback transaction",
				zap.Error(rbErr),
				zap.NamedError("original_error", err),
			)
			return fmt.Errorf("transaction error: %w (rollback error: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// InTxWithOptions executes a function within a database transaction with custom isolation.
func (db *DB) InTxWithOptions(fn func(*sql.Tx) error, opts *sql.TxOptions) error {
	tx, err := db.BeginTx(nil, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			db.logger.Error("failed to rollback transaction",
				zap.Error(rbErr),
				zap.NamedError("original_error", err),
			)
			return fmt.Errorf("transaction error: %w (rollback error: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
