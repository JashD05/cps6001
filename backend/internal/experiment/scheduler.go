package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// ScheduledExperiment represents a scheduled experiment with its next
// execution time and optional cron expression for recurring runs.
type ScheduledExperiment struct {
	ExperimentID   uuid.UUID `json:"experiment_id"`
	Name           string    `json:"name"`
	NextRunAt      time.Time `json:"next_run_at"`
	CronExpression string    `json:"cron_expression,omitempty"`
	IsRecurring    bool      `json:"is_recurring"`
}

// Scheduler manages the scheduling and automatic execution of experiments.
// It polls the database for experiments that are due to run and submits them
// to the engine for execution. It supports one-time scheduled runs as well
// as recurring cron-based schedules.
type Scheduler struct {
	engine     *Engine
	db         *sql.DB
	rdb        *redis.Client
	logger     *zap.Logger
	cronParser cron.Parser
	cronMu     sync.Mutex
	cronJobs   map[uuid.UUID]cron.EntryID // experiment ID -> cron job ID
	cronRunner *cron.Cron

	// stopCh signals the scheduler polling loop to exit.
	stopCh chan struct{}
	// doneCh is closed when the polling goroutine has fully exited.
	doneCh chan struct{}

	// pollInterval is how often the scheduler checks for due experiments.
	pollInterval time.Duration

	// maxConcurrent limits how many experiments can be started in a single
	// poll cycle to avoid overwhelming the system.
	maxConcurrent int

	// started tracks whether the scheduler has been started.
	started bool
	startMu sync.Mutex
}

// SchedulerOption is a functional option for configuring a Scheduler.
type SchedulerOption func(*Scheduler)

// WithPollInterval sets the interval at which the scheduler checks for due
// experiments. The default is 10 seconds.
func WithPollInterval(d time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.pollInterval = d
	}
}

// WithMaxConcurrent sets the maximum number of experiments that can be
// started in a single poll cycle. The default is 5.
func WithMaxConcurrent(n int) SchedulerOption {
	return func(s *Scheduler) {
		s.maxConcurrent = n
	}
}

// NewScheduler creates a new experiment Scheduler with the given dependencies
// and optional configuration.
func NewScheduler(
	engine *Engine,
	db *sql.DB,
	rdb *redis.Client,
	logger *zap.Logger,
	opts ...SchedulerOption,
) *Scheduler {
	s := &Scheduler{
		engine:        engine,
		db:            db,
		rdb:           rdb,
		logger:        logger.Named("experiment_scheduler"),
		cronParser:    cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		cronJobs:      make(map[uuid.UUID]cron.EntryID),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		pollInterval:  10 * time.Second,
		maxConcurrent: 5,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ScheduleExperiment schedules an experiment for one-time future execution
// at the specified time. The schedule is persisted in the database so it
// survives restarts.
func (s *Scheduler) ScheduleExperiment(
	ctx context.Context,
	experimentID uuid.UUID,
	scheduleTime time.Time,
) error {
	if scheduleTime.Before(time.Now()) {
		return fmt.Errorf("schedule time %s is in the past", scheduleTime)
	}

	// Verify the experiment exists.
	var name string
	err := s.db.QueryRowContext(ctx,
		"SELECT name FROM experiments WHERE id = $1",
		experimentID,
	).Scan(&name)
	if err != nil {
		return fmt.Errorf("failed to find experiment %s: %w", experimentID, err)
	}

	// Store the schedule in Redis for fast lookup.
	scheduleKey := fmt.Sprintf("experiment:schedule:%s", experimentID.String())
	scheduleData, _ := json.Marshal(ScheduledExperiment{
		ExperimentID: experimentID,
		Name:         name,
		NextRunAt:    scheduleTime,
		IsRecurring:  false,
	})

	ttl := time.Until(scheduleTime) + 30*time.Minute // small buffer past execution time
	if ttl < 5*time.Minute {
		ttl = 5 * time.Minute
	}
	if s.rdb != nil {
		if err := s.rdb.Set(ctx, scheduleKey, scheduleData, ttl).Err(); err != nil {
			s.logger.Error("failed to persist schedule in Redis",
				zap.String("experiment_id", experimentID.String()),
				zap.Error(err),
			)
			// Continue — the DB is the source of truth.
		}
	}

	s.logger.Info("experiment scheduled for future execution",
		zap.String("experiment_id", experimentID.String()),
		zap.String("name", name),
		zap.Time("scheduled_at", scheduleTime),
	)
	return nil
}

// ScheduleRecurringExperiment sets up a recurring schedule for an experiment
// using a cron expression. The cron expression follows the standard 5-field
// format: minute hour day-of-month month day-of-week.
//
// Example expressions:
//   - "0 * * * *"     — every hour
//   - "*/30 * * * *"  — every 30 minutes
//   - "0 9 * * 1-5"  — weekdays at 9:00 AM
func (s *Scheduler) ScheduleRecurringExperiment(
	ctx context.Context,
	experimentID uuid.UUID,
	cronExpr string,
) error {
	// Validate the cron expression.
	schedule, err := s.cronParser.Parse(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}

	// Verify the experiment exists.
	var name string
	err = s.db.QueryRowContext(ctx,
		"SELECT name FROM experiments WHERE id = $1",
		experimentID,
	).Scan(&name)
	if err != nil {
		return fmt.Errorf("failed to find experiment %s: %w", experimentID, err)
	}

	// Persist the cron expression on the experiment record.
	_, err = s.db.ExecContext(ctx,
		"UPDATE experiments SET schedule_cron = $1, status = 'active', updated_at = NOW() WHERE id = $2",
		cronExpr, experimentID,
	)
	if err != nil {
		return fmt.Errorf("failed to update experiment schedule_cron: %w", err)
	}

	// Calculate next run time.
	nextRun := schedule.Next(time.Now())

	// Store the recurring schedule in Redis.
	scheduleKey := fmt.Sprintf("experiment:schedule:%s", experimentID.String())
	scheduleData, _ := json.Marshal(ScheduledExperiment{
		ExperimentID:   experimentID,
		Name:           name,
		NextRunAt:      nextRun,
		CronExpression: cronExpr,
		IsRecurring:    true,
	})

	if s.rdb != nil {
		if err := s.rdb.Set(ctx, scheduleKey, scheduleData, 7*24*time.Hour).Err(); err != nil {
			s.logger.Error("failed to persist recurring schedule in Redis",
				zap.String("experiment_id", experimentID.String()),
				zap.Error(err),
			)
		}
	}

	// Register the cron job if the scheduler is already running.
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if s.cronRunner != nil {
		s.registerCronJob(experimentID, cronExpr, name)
	}

	s.logger.Info("recurring experiment scheduled",
		zap.String("experiment_id", experimentID.String()),
		zap.String("name", name),
		zap.String("cron", cronExpr),
		zap.Time("next_run", nextRun),
	)
	return nil
}

// Start begins the scheduler's main polling loop and cron runner. The
// polling goroutine checks the database every pollInterval for experiments
// that are due to execute and submits them to the engine.
func (s *Scheduler) Start() {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.started {
		s.logger.Warn("scheduler already started, ignoring duplicate Start call")
		return
	}
	s.started = true

	// Start the cron runner.
	s.cronRunner = cron.New()
	s.cronRunner.Start()

	// Load any existing recurring schedules from the DB.
	s.loadExistingCronSchedules()

	// Start the polling goroutine.
	go s.pollLoop()

	s.logger.Info("scheduler started",
		zap.Duration("poll_interval", s.pollInterval),
		zap.Int("max_concurrent", s.maxConcurrent),
	)
}

// Stop gracefully stops the scheduler by signalling the polling loop to
// exit and stopping the cron runner. It waits for the current poll cycle
// to complete before returning.
func (s *Scheduler) Stop() {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if !s.started {
		return
	}
	s.started = false

	// Signal the polling loop to stop.
	close(s.stopCh)

	// Wait for the polling goroutine to finish.
	<-s.doneCh

	// Stop the cron runner.
	s.cronMu.Lock()
	if s.cronRunner != nil {
		ctx := s.cronRunner.Stop()
		<-ctx.Done()
		s.cronRunner = nil
	}
	s.cronMu.Unlock()

	s.logger.Info("scheduler stopped")
}

// GetPendingSchedules returns a list of upcoming scheduled experiments
// from both Redis and the database.
func (s *Scheduler) GetPendingSchedules(ctx context.Context) ([]ScheduledExperiment, error) {
	var schedules []ScheduledExperiment

	// First, try to get schedules from Redis (faster).
	if s.rdb != nil {
		pattern := "experiment:schedule:*"
		keys, err := s.rdb.Keys(ctx, pattern).Result()
		if err != nil {
			s.logger.Warn("failed to list schedule keys from Redis, falling back to DB",
				zap.Error(err),
			)
			return s.getPendingSchedulesFromDB(ctx)
		}

		for _, key := range keys {
			data, err := s.rdb.Get(ctx, key).Bytes()
			if err != nil {
				if err == redis.Nil {
					continue
				}
				s.logger.Warn("failed to read schedule from Redis",
					zap.String("key", key),
					zap.Error(err),
				)
				continue
			}

			var se ScheduledExperiment
			if err := json.Unmarshal(data, &se); err != nil {
				s.logger.Warn("failed to unmarshal schedule data",
					zap.String("key", key),
					zap.Error(err),
				)
				continue
			}

			schedules = append(schedules, se)
		}
	}

	// Also check the DB for any schedules with cron expressions that
	// may not have been loaded into Redis (e.g., after a restart).
	dbSchedules, err := s.getPendingSchedulesFromDB(ctx)
	if err != nil {
		s.logger.Warn("failed to get schedules from DB", zap.Error(err))
	} else {
		schedules = append(schedules, dbSchedules...)
	}

	return deduplicateSchedules(schedules), nil
}

// CancelSchedule cancels a scheduled experiment by removing it from Redis
// and clearing the cron expression on the experiment record.
func (s *Scheduler) CancelSchedule(ctx context.Context, experimentID uuid.UUID) error {
	// Remove from Redis.
	scheduleKey := fmt.Sprintf("experiment:schedule:%s", experimentID.String())
	if s.rdb != nil {
		if err := s.rdb.Del(ctx, scheduleKey).Err(); err != nil {
			s.logger.Warn("failed to remove schedule from Redis",
				zap.String("experiment_id", experimentID.String()),
				zap.Error(err),
			)
		}
	}

	// Clear the cron expression in the DB.
	_, err := s.db.ExecContext(ctx,
		"UPDATE experiments SET schedule_cron = NULL, updated_at = NOW() WHERE id = $1",
		experimentID,
	)
	if err != nil {
		return fmt.Errorf("failed to clear experiment schedule_cron: %w", err)
	}

	// Remove any registered cron job.
	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	if jobID, ok := s.cronJobs[experimentID]; ok && s.cronRunner != nil {
		s.cronRunner.Remove(jobID)
		delete(s.cronJobs, experimentID)
	}

	s.logger.Info("experiment schedule cancelled",
		zap.String("experiment_id", experimentID.String()),
	)
	return nil
}

// ---------------------------------------------------------------------------
// Polling loop
// ---------------------------------------------------------------------------

// pollLoop is the main scheduler loop. It periodically checks for
// experiments that are due to run and executes them.
func (s *Scheduler) pollLoop() {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.pollOnce()
		}
	}
}

// pollOnce performs a single poll cycle: it queries the database for
// experiments due to run and submits them for execution.
func (s *Scheduler) pollOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dueExperiments, err := s.findDueExperiments(ctx)
	if err != nil {
		s.logger.Error("failed to find due experiments", zap.Error(err))
		return
	}

	if len(dueExperiments) == 0 {
		return
	}

	s.logger.Info("found due experiments",
		zap.Int("count", len(dueExperiments)),
	)

	executed := 0
	for _, se := range dueExperiments {
		if executed >= s.maxConcurrent {
			s.logger.Warn("max concurrent limit reached for this poll cycle",
				zap.Int("limit", s.maxConcurrent),
				zap.Int("remaining", len(dueExperiments)-executed),
			)
			break
		}

		// Execute the experiment in a goroutine so we don't block the poll loop.
		go func(se ScheduledExperiment) {
			execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer execCancel()

			s.logger.Info("executing scheduled experiment",
				zap.String("experiment_id", se.ExperimentID.String()),
				zap.String("name", se.Name),
			)

			// Use a system user ID for scheduled runs (uuid.Nil indicates automated).
			_, execErr := s.engine.ExecuteExperiment(execCtx, se.ExperimentID, uuid.Nil)
			if execErr != nil {
				s.logger.Error("scheduled experiment execution failed",
					zap.String("experiment_id", se.ExperimentID.String()),
					zap.Error(execErr),
				)
			}

			// If this is a recurring experiment, re-schedule it.
			if se.IsRecurring && se.CronExpression != "" {
				s.rescheduleRecurring(ctx, se)
			} else {
				// Remove the one-time schedule from Redis.
				if s.rdb != nil {
					scheduleKey := fmt.Sprintf("experiment:schedule:%s", se.ExperimentID.String())
					_ = s.rdb.Del(ctx, scheduleKey).Err()
				}
			}
		}(se)

		executed++
	}
}

// ---------------------------------------------------------------------------
// Database queries
// ---------------------------------------------------------------------------

// findDueExperiments queries the database for experiments whose scheduled
// time has passed and that should be executed now.
func (s *Scheduler) findDueExperiments(ctx context.Context) ([]ScheduledExperiment, error) {
	// Check experiments with cron expressions.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, schedule_cron
		FROM experiments
		WHERE schedule_cron IS NOT NULL
		  AND status IN ('active', 'draft')
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled experiments: %w", err)
	}
	defer rows.Close()

	var due []ScheduledExperiment
	now := time.Now()

	for rows.Next() {
		var (
			id       uuid.UUID
			name     string
			cronExpr *string
		)
		if err := rows.Scan(&id, &name, &cronExpr); err != nil {
			s.logger.Error("failed to scan scheduled experiment row", zap.Error(err))
			continue
		}

		if cronExpr == nil {
			continue
		}

		// Check if this experiment is due by evaluating the cron schedule
		// and comparing against the last run time.
		schedule, parseErr := s.cronParser.Parse(*cronExpr)
		if parseErr != nil {
			s.logger.Error("invalid cron expression for experiment",
				zap.String("experiment_id", id.String()),
				zap.String("cron", *cronExpr),
				zap.Error(parseErr),
			)
			continue
		}

		// Get the last run time for this experiment.
		var lastRunAt *time.Time
		_ = s.db.QueryRowContext(ctx, `
			SELECT MAX(started_at) FROM experiment_runs
			WHERE experiment_id = $1 AND status IN ('completed', 'running')
		`, id).Scan(&lastRunAt)

		// Determine if the experiment is due.
		nextRun := schedule.Next(time.Time{})
		if lastRunAt != nil {
			nextRun = schedule.Next(*lastRunAt)
		}

		if !nextRun.After(now) || nextRun.Before(now.Add(s.pollInterval)) {
			// The experiment was due since the last poll or will be due
			// within the next poll interval.
			due = append(due, ScheduledExperiment{
				ExperimentID:   id,
				Name:           name,
				NextRunAt:      nextRun,
				CronExpression: *cronExpr,
				IsRecurring:    true,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scheduled experiments: %w", err)
	}

	return due, nil
}

// getPendingSchedulesFromDB queries the database for all experiments with
// cron schedules as a fallback when Redis is unavailable.
func (s *Scheduler) getPendingSchedulesFromDB(ctx context.Context) ([]ScheduledExperiment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, schedule_cron
		FROM experiments
		WHERE schedule_cron IS NOT NULL
		  AND status IN ('active', 'draft')
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled experiments from DB: %w", err)
	}
	defer rows.Close()

	var schedules []ScheduledExperiment
	for rows.Next() {
		var (
			id       uuid.UUID
			name     string
			cronExpr *string
		)
		if err := rows.Scan(&id, &name, &cronExpr); err != nil {
			continue
		}

		if cronExpr == nil {
			continue
		}

		schedule, parseErr := s.cronParser.Parse(*cronExpr)
		if parseErr != nil {
			continue
		}

		nextRun := schedule.Next(time.Now())

		schedules = append(schedules, ScheduledExperiment{
			ExperimentID:   id,
			Name:           name,
			NextRunAt:      nextRun,
			CronExpression: *cronExpr,
			IsRecurring:    true,
		})
	}

	return schedules, rows.Err()
}

// ---------------------------------------------------------------------------
// Cron management
// ---------------------------------------------------------------------------

// loadExistingCronSchedules loads any experiments with cron schedules from
// the database and registers them with the cron runner. This is called on
// scheduler startup.
func (s *Scheduler) loadExistingCronSchedules() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, schedule_cron
		FROM experiments
		WHERE schedule_cron IS NOT NULL
		  AND status IN ('active', 'draft')
	`)
	if err != nil {
		s.logger.Error("failed to load existing cron schedules", zap.Error(err))
		return
	}
	defer rows.Close()

	s.cronMu.Lock()
	defer s.cronMu.Unlock()

	for rows.Next() {
		var (
			id       uuid.UUID
			name     string
			cronExpr *string
		)
		if err := rows.Scan(&id, &name, &cronExpr); err != nil {
			continue
		}

		if cronExpr == nil {
			continue
		}

		s.registerCronJob(id, *cronExpr, name)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("error iterating cron schedules", zap.Error(err))
	}
}

// registerCronJob adds a cron job to the cron runner for the given experiment.
// The cron runner must already be started. The caller must hold cronMu.
func (s *Scheduler) registerCronJob(experimentID uuid.UUID, cronExpr, name string) {
	if s.cronRunner == nil {
		return
	}

	// Remove any existing job for this experiment first.
	if existingJobID, ok := s.cronJobs[experimentID]; ok {
		s.cronRunner.Remove(existingJobID)
		delete(s.cronJobs, experimentID)
	}

	jobSpec := cronExpr
	wrappedFunc := func() {
		s.logger.Info("executing recurring experiment from cron",
			zap.String("experiment_id", experimentID.String()),
			zap.String("name", name),
			zap.String("cron", cronExpr),
		)

		execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer execCancel()

		_, err := s.engine.ExecuteExperiment(execCtx, experimentID, uuid.Nil)
		if err != nil {
			s.logger.Error("recurring experiment execution failed",
				zap.String("experiment_id", experimentID.String()),
				zap.Error(err),
			)
		}
	}

	jobID, err := s.cronRunner.AddFunc(jobSpec, wrappedFunc)
	if err != nil {
		s.logger.Error("failed to add cron job for experiment",
			zap.String("experiment_id", experimentID.String()),
			zap.String("cron", cronExpr),
			zap.Error(err),
		)
		return
	}

	s.cronJobs[experimentID] = jobID
	s.logger.Info("registered cron job for experiment",
		zap.String("experiment_id", experimentID.String()),
		zap.String("name", name),
		zap.String("cron", cronExpr),
		zap.Any("job_id", jobID),
	)
}

// rescheduleRecurring updates the next run time in Redis for a recurring
// experiment after it has been executed.
func (s *Scheduler) rescheduleRecurring(ctx context.Context, se ScheduledExperiment) {
	schedule, err := s.cronParser.Parse(se.CronExpression)
	if err != nil {
		s.logger.Error("invalid cron expression during reschedule",
			zap.String("experiment_id", se.ExperimentID.String()),
			zap.String("cron", se.CronExpression),
			zap.Error(err),
		)
		return
	}

	nextRun := schedule.Next(time.Now())
	updated := ScheduledExperiment{
		ExperimentID:   se.ExperimentID,
		Name:           se.Name,
		NextRunAt:      nextRun,
		CronExpression: se.CronExpression,
		IsRecurring:    true,
	}

	scheduleKey := fmt.Sprintf("experiment:schedule:%s", se.ExperimentID.String())
	data, _ := json.Marshal(updated)
	if s.rdb != nil {
		if err := s.rdb.Set(ctx, scheduleKey, data, 7*24*time.Hour).Err(); err != nil {
			s.logger.Error("failed to reschedule recurring experiment in Redis",
				zap.String("experiment_id", se.ExperimentID.String()),
				zap.Error(err),
			)
		}
	}

	s.logger.Debug("recurring experiment rescheduled",
		zap.String("experiment_id", se.ExperimentID.String()),
		zap.Time("next_run", nextRun),
	)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// deduplicateSchedules removes duplicate schedule entries by experiment ID,
// keeping the entry with the earliest next run time.
func deduplicateSchedules(schedules []ScheduledExperiment) []ScheduledExperiment {
	seen := make(map[uuid.UUID]ScheduledExperiment)
	for _, se := range schedules {
		if existing, ok := seen[se.ExperimentID]; !ok || se.NextRunAt.Before(existing.NextRunAt) {
			seen[se.ExperimentID] = se
		}
	}

	result := make([]ScheduledExperiment, 0, len(seen))
	for _, se := range seen {
		result = append(result, se)
	}
	return result
}
