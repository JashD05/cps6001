package worker

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/chaos-sec/backend/internal/experiment"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// baseRetryDelay is the base delay for exponential backoff retry.
const baseRetryDelay = 5 * time.Second

// maxRetryDelay caps the retry delay at 5 minutes regardless of the
// exponential growth.
const maxRetryDelay = 5 * time.Minute

// Processor handles the processing of individual experiment runs. It
// loads the run from the database, delegates execution to the Engine,
// and manages retries with exponential backoff on failure.
//
// The Processor is designed to be used by the worker Pool, but can also
// be used standalone for programmatic experiment execution.
type Processor struct {
	engine *experiment.Engine
	db     *sql.DB
	rdb    *redis.Client
	logger *zap.Logger
}

// NewProcessor creates a new experiment Processor with the given dependencies.
func NewProcessor(
	engine *experiment.Engine,
	db *sql.DB,
	rdb *redis.Client,
	logger *zap.Logger,
) *Processor {
	return &Processor{
		engine: engine,
		db:     db,
		rdb:    rdb,
		logger: logger.Named("experiment_processor"),
	}
}

// Process processes a single experiment run by:
//  1. Loading the run from the database
//  2. Updating its status to "running"
//  3. Calling Engine.ExecuteExperiment with the associated experiment ID
//  4. On success: updating the run with results
//  5. On error: updating the run with the error message and incrementing the retry count
//  6. On max retries exceeded: marking the run as "failed"
func (p *Processor) Process(ctx context.Context, runID uuid.UUID) error {
	p.logger.Info("processing experiment run",
		zap.String("run_id", runID.String()),
	)

	// Step 1: Load the run from the database.
	run, err := p.loadRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to load experiment run %s: %w", runID, err)
	}

	// Skip if the run is already completed, failed, or cancelled.
	switch run.Status {
	case "completed", "failed", "cancelled":
		p.logger.Info("skipping experiment run with terminal status",
			zap.String("run_id", runID.String()),
			zap.String("status", run.Status),
		)
		return nil
	}

	// Step 2: Update the run status to "running" if it's currently "pending".
	if run.Status == "pending" {
		if err := p.updateRunStatus(ctx, runID, "running", ""); err != nil {
			p.logger.Error("failed to update run status to running",
				zap.String("run_id", runID.String()),
				zap.Error(err),
			)
			// Non-fatal: continue with execution anyway.
		}
	}

	// Step 3: Execute the experiment via the Engine.
	result, execErr := p.engine.ExecuteExperiment(ctx, run.ExperimentID, *run.TriggeredBy)

	// Step 4: Handle the result.
	if execErr != nil {
		p.logger.Error("experiment execution failed",
			zap.String("run_id", runID.String()),
			zap.String("experiment_id", run.ExperimentID.String()),
			zap.Error(execErr),
		)

		// Update the run with the error message.
		errMsg := execErr.Error()
		// Truncate long error messages to fit in the database column.
		if len(errMsg) > 2000 {
			errMsg = errMsg[:1997] + "..."
		}

		if updateErr := p.updateRunStatus(ctx, runID, "failed", errMsg); updateErr != nil {
			p.logger.Error("failed to update run status to failed",
				zap.String("run_id", runID.String()),
				zap.Error(updateErr),
			)
		}

		// Increment the retry count in Redis.
		p.incrementRetryCount(ctx, runID)

		return fmt.Errorf("experiment execution failed: %w", execErr)
	}

	// Success: update the run status.
	if result != nil {
		p.logger.Info("experiment execution completed",
			zap.String("run_id", runID.String()),
			zap.String("status", result.Status),
		)
	}

	// Clear any stale retry counters from previous failures.
	p.clearRetryCount(ctx, runID)

	return nil
}

// ProcessWithRetry processes an experiment run with exponential backoff
// retry. If the initial processing attempt fails, it retries up to
// maxRetries times with increasing delays between attempts.
//
// The retry delay follows the formula: delay = 2^attempt * base_delay
// where base_delay is 5 seconds and the delay is capped at 5 minutes.
//
// For example, with 3 retries:
//   - Attempt 0: immediate
//   - Attempt 1: 10s delay  (2^1 * 5s)
//   - Attempt 2: 20s delay  (2^2 * 5s)
//   - Attempt 3: 40s delay  (2^3 * 5s)
func (p *Processor) ProcessWithRetry(ctx context.Context, runID uuid.UUID, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check if the context is still valid before proceeding.
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled before attempt %d: %w (last error: %v)",
				attempt, ctx.Err(), lastErr)
		}

		// Apply retry delay if this is not the first attempt.
		if attempt > 0 {
			delay := p.RetryDelay(attempt)

			p.logger.Info("retrying experiment processing after backoff",
				zap.String("run_id", runID.String()),
				zap.Int("attempt", attempt),
				zap.Int("max_retries", maxRetries),
				zap.Duration("delay", delay),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry delay: %w (last error: %v)",
					ctx.Err(), lastErr)
			case <-time.After(delay):
				// Delay elapsed, continue with the next attempt.
			}
		}

		// Attempt processing.
		err := p.Process(ctx, runID)
		if err == nil {
			return nil // Success.
		}

		lastErr = err

		p.logger.Warn("experiment processing attempt failed",
			zap.String("run_id", runID.String()),
			zap.Int("attempt", attempt),
			zap.Int("remaining_retries", maxRetries-attempt),
			zap.Error(err),
		)

		// Check if the run has been cancelled in the meantime.
		currentRun, loadErr := p.loadRun(ctx, runID)
		if loadErr == nil && currentRun.Status == "cancelled" {
			p.logger.Info("experiment run was cancelled, stopping retries",
				zap.String("run_id", runID.String()),
			)
			return nil
		}

		// Check if the run has exceeded the retry limit.
		retryCount := p.getRetryCount(ctx, runID)
		if retryCount > maxRetries {
			p.logger.Error("experiment exceeded retry limit, marking as permanently failed",
				zap.String("run_id", runID.String()),
				zap.Int("retry_count", retryCount),
				zap.Int("max_retries", maxRetries),
			)

			// Mark as permanently failed in the database.
			failMsg := fmt.Sprintf("Exceeded maximum retry limit of %d: %v", maxRetries, lastErr)
			if len(failMsg) > 2000 {
				failMsg = failMsg[:1997] + "..."
			}
			_ = p.updateRunStatus(ctx, runID, "failed", failMsg)

			return fmt.Errorf("exceeded max retries (%d): %w", maxRetries, lastErr)
		}
	}

	// All retries exhausted.
	if lastErr != nil {
		return fmt.Errorf("all %d processing attempts failed: %w", maxRetries+1, lastErr)
	}

	return fmt.Errorf("all %d processing attempts failed with unknown error", maxRetries+1)
}

// RetryDelay calculates the delay duration for a given retry attempt.
// The delay follows exponential backoff: 2^attempt * base_delay (5 seconds),
// capped at maxRetryDelay (5 minutes).
//
// Examples:
//   - attempt 1: 10s   (2^1 * 5s)
//   - attempt 2: 20s   (2^2 * 5s)
//   - attempt 3: 40s   (2^3 * 5s)
//   - attempt 4: 80s   (2^4 * 5s)
//   - attempt 5: 160s  (2^5 * 5s)
//   - attempt 6+: 300s  (capped at 5 minutes)
func (p *Processor) RetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Exponential backoff: 2^attempt * base_delay.
	exponent := math.Pow(2, float64(attempt))
	delay := time.Duration(exponent) * baseRetryDelay

	// Cap at max retry delay.
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	return delay
}

// ---------------------------------------------------------------------------
// Database operations
// ---------------------------------------------------------------------------

// runRecord is a lightweight representation of an experiment run loaded
// from the database. It contains only the fields needed by the Processor
// to decide how to handle the run.
type runRecord struct {
	ID            uuid.UUID
	ExperimentID  uuid.UUID
	Status        string
	TriggeredBy   *uuid.UUID
	RetryCount    int
	LastAttemptAt *time.Time
}

// loadRun loads an experiment run's essential fields from the database.
func (p *Processor) loadRun(ctx context.Context, runID uuid.UUID) (*runRecord, error) {
	var rec runRecord
	var triggeredBy sql.NullString

	err := p.db.QueryRowContext(ctx, `
		SELECT id, experiment_id, status, triggered_by
		FROM experiment_runs
		WHERE id = $1
	`, runID).Scan(
		&rec.ID,
		&rec.ExperimentID,
		&rec.Status,
		&triggeredBy,
	)
	if err != nil {
		return nil, fmt.Errorf("query experiment run: %w", err)
	}

	if triggeredBy.Valid && triggeredBy.String != "" {
		id, parseErr := uuid.Parse(triggeredBy.String)
		if parseErr == nil {
			rec.TriggeredBy = &id
		}
	}

	// Load retry count from Redis (faster, transient storage).
	rec.RetryCount = p.getRetryCount(ctx, runID)

	return &rec, nil
}

// updateRunStatus updates the status and optionally the error message
// of an experiment run in the database.
func (p *Processor) updateRunStatus(ctx context.Context, runID uuid.UUID, status string, errMsg string) error {
	var err error
	if errMsg != "" {
		_, err = p.db.ExecContext(ctx, `
			UPDATE experiment_runs
			SET status = $1,
			    error_message = $2,
			    updated_at = NOW()
			WHERE id = $3
		`, status, errMsg, runID)
	} else {
		_, err = p.db.ExecContext(ctx, `
			UPDATE experiment_runs
			SET status = $1,
			    error_message = NULL,
			    updated_at = NOW()
			WHERE id = $2
		`, status, runID)
	}

	if err != nil {
		return fmt.Errorf("failed to update run %s status to %s: %w", runID, status, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Redis retry tracking
// ---------------------------------------------------------------------------

// retryKeyPrefix is the Redis key prefix for storing retry counts.
const retryKeyPrefix = "experiment:worker:retries:"

// retryKey returns the Redis key for a given run's retry counter.
func retryKey(runID uuid.UUID) string {
	return retryKeyPrefix + runID.String()
}

// getRetryCount returns the number of retry attempts recorded for a
// given run ID in Redis. Returns 0 if no retries have been recorded
// or if Redis is unavailable.
func (p *Processor) getRetryCount(ctx context.Context, runID uuid.UUID) int {
	if p.rdb == nil {
		return 0
	}

	val, err := p.rdb.Get(ctx, retryKey(runID)).Int()
	if err != nil {
		return 0
	}
	return val
}

// incrementRetryCount atomically increments the retry counter for a
// run ID in Redis. The key is set with a 24-hour TTL as a safety net.
func (p *Processor) incrementRetryCount(ctx context.Context, runID uuid.UUID) {
	if p.rdb == nil {
		return
	}

	key := retryKey(runID)
	_ = p.rdb.Incr(ctx, key).Err()
	_ = p.rdb.Expire(ctx, key, 24*time.Hour).Err()
}

// clearRetryCount removes the retry counter for a run ID from Redis.
// This is called on successful execution to reset the retry state.
func (p *Processor) clearRetryCount(ctx context.Context, runID uuid.UUID) {
	if p.rdb == nil {
		return
	}

	_ = p.rdb.Del(ctx, retryKey(runID)).Err()
}

// reportStatus publishes the current execution status of an experiment
// run to Redis for real-time monitoring.
func (p *Processor) reportStatus(ctx context.Context, runID uuid.UUID, status, message string) {
	if p.rdb == nil {
		return
	}

	statusKey := fmt.Sprintf("experiment:processor:status:%s", runID.String())
	fields := map[string]interface{}{
		"run_id":   runID.String(),
		"status":   status,
		"message":  message,
		"worker":   "processor",
		"datetime": time.Now().Format(time.RFC3339),
	}

	if err := p.rdb.HSet(ctx, statusKey, fields).Err(); err != nil {
		p.logger.Debug("failed to store processor status in Redis", zap.Error(err))
	}
	_ = p.rdb.Expire(ctx, statusKey, 24*time.Hour).Err()
}
