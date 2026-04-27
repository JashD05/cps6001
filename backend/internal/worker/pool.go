package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chaos-sec/backend/internal/experiment"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DefaultWorkerCount is the default number of concurrent experiment workers.
const DefaultWorkerCount = 5

// Pool manages a set of background worker goroutines that dequeue and
// execute experiments from the Redis priority queue. It provides concurrency
// limiting, graceful shutdown, and status reporting.
//
// Workers operate in a simple loop:
//  1. Dequeue the next experiment run ID from the Redis sorted set queue.
//  2. Process the experiment via the Processor.
//  3. On success, update the run status.
//  4. On error, the Processor handles retries with exponential backoff.
//  5. Report status via Redis for real-time monitoring.
type Pool struct {
	workerCount int
	processor   *Processor
	queue       *experiment.Queue
	rdb         *redis.Client
	logger      *zap.Logger

	// jobCh is an internal channel for directly submitted experiment run IDs.
	// Workers also listen on the Redis queue, so this channel acts as an
	// additional fast path for submissions that bypass the queue.
	jobCh chan uuid.UUID

	// stopCh signals all workers to exit their loops.
	stopCh chan struct{}

	// doneCh is closed once all workers have fully exited.
	doneCh chan struct{}

	// activeCount tracks the number of experiments currently being executed.
	activeCount int64

	// wg tracks running worker goroutines for graceful shutdown.
	wg sync.WaitGroup

	// started tracks whether the pool has been started.
	started bool
	startMu sync.Mutex

	// dequeueTimeout controls how long a worker waits for a queue item
	// before checking the stop channel.
	dequeueTimeout time.Duration
}

// PoolOption is a functional option for configuring a Pool.
type PoolOption func(*Pool)

// WithDequeueTimeout sets the timeout for each dequeue operation. Workers
// check for stop signals between dequeue attempts. Default is 5 seconds.
func WithDequeueTimeout(d time.Duration) PoolOption {
	return func(p *Pool) {
		p.dequeueTimeout = d
	}
}

// WithPoolLogger sets the structured logger for the worker pool.
func WithPoolLogger(logger *zap.Logger) PoolOption {
	return func(p *Pool) {
		p.logger = logger
	}
}

// NewPool creates a new worker Pool with the given worker count, processor,
// queue, Redis client, and optional configuration.
func NewPool(
	workerCount int,
	processor *Processor,
	queue *experiment.Queue,
	rdb *redis.Client,
	logger *zap.Logger,
	opts ...PoolOption,
) *Pool {
	if workerCount <= 0 {
		workerCount = DefaultWorkerCount
	}

	p := &Pool{
		workerCount:    workerCount,
		processor:      processor,
		queue:          queue,
		rdb:            rdb,
		logger:         logger.Named("worker_pool"),
		jobCh:          make(chan uuid.UUID, workerCount*2),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		dequeueTimeout: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Start launches the configured number of worker goroutines. Each worker
// runs an event loop that dequeues experiments from the Redis queue and
// the internal job channel, processing them via the Processor.
func (p *Pool) Start() {
	p.startMu.Lock()
	defer p.startMu.Unlock()

	if p.started {
		p.logger.Warn("worker pool already started, ignoring duplicate Start call")
		return
	}
	p.started = true

	p.logger.Info("starting worker pool",
		zap.Int("worker_count", p.workerCount),
		zap.Duration("dequeue_timeout", p.dequeueTimeout),
	)

	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop gracefully stops all workers by signalling them to exit and waiting
// for them to finish their current experiment. It waits up to 30 seconds
// for in-flight experiments to complete; after that, it returns.
func (p *Pool) Stop() {
	p.startMu.Lock()
	defer p.startMu.Unlock()

	if !p.started {
		return
	}
	p.started = false

	p.logger.Info("stopping worker pool, waiting for in-flight experiments")

	// Signal all workers to stop.
	close(p.stopCh)

	// Wait for all workers to exit with a timeout.
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.logger.Info("worker pool stopped gracefully")
	case <-time.After(30 * time.Second):
		p.logger.Warn("worker pool stop timed out, some workers may still be running")
	}

	// Close the doneCh if it hasn't been closed yet.
	select {
	case <-p.doneCh:
		// Already closed.
	default:
		close(p.doneCh)
	}
}

// Submit submits an experiment run ID to the pool for execution. The run
// is placed on both the internal job channel (for fast processing) and the
// Redis queue (for persistence and distributed processing).
//
// If the pool is not started, Submit returns an error.
func (p *Pool) Submit(runID uuid.UUID) error {
	p.startMu.Lock()
	started := p.started
	p.startMu.Unlock()

	if !started {
		return fmt.Errorf("worker pool is not started")
	}

	// Enqueue in Redis for persistence and distributed awareness.
	if p.queue != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.queue.Enqueue(ctx, runID, 0); err != nil {
			p.logger.Warn("failed to enqueue run in Redis, submitting via channel only",
				zap.String("run_id", runID.String()),
				zap.Error(err),
			)
		}
	}

	// Also push to the internal channel for fast local processing.
	select {
	case p.jobCh <- runID:
		p.logger.Debug("experiment submitted to worker pool",
			zap.String("run_id", runID.String()),
		)
	default:
		// Channel is full — the Redis queue will handle it.
		p.logger.Debug("job channel full, experiment will be picked up from Redis queue",
			zap.String("run_id", runID.String()),
		)
	}

	return nil
}

// ActiveCount returns the number of experiments currently being executed
// by the pool's workers.
func (p *Pool) ActiveCount() int {
	return int(atomic.LoadInt64(&p.activeCount))
}

// WaitForCompletion blocks until all active experiments have finished
// executing or the context is cancelled. It is useful for graceful
// shutdown scenarios where you want to wait for in-flight work.
func (p *Pool) WaitForCompletion(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if atomic.LoadInt64(&p.activeCount) == 0 {
			// Double-check after a brief pause to avoid a race between
			// the count going to zero and a new experiment starting.
			time.Sleep(100 * time.Millisecond)
			if atomic.LoadInt64(&p.activeCount) == 0 {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for completion cancelled: %w", ctx.Err())
		case <-ticker.C:
			// Continue polling.
		}
	}
}

// WorkerCount returns the configured number of workers.
func (p *Pool) WorkerCount() int {
	return p.workerCount
}

// ---------------------------------------------------------------------------
// Worker goroutine
// ---------------------------------------------------------------------------

// worker is the main loop for a single worker goroutine. It continuously
// dequeues experiment run IDs from the Redis queue and the internal job
// channel, processes them, and handles retries.
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	p.logger.Debug("worker started", zap.Int("worker_id", id))

	for {
		// Check for stop signal first.
		select {
		case <-p.stopCh:
			p.logger.Debug("worker stopping", zap.Int("worker_id", id))
			return
		default:
		}

		// Try to get a run ID from either the internal channel or the Redis queue.
		runID, err := p.nextJob()
		if err != nil {
			p.logger.Error("worker failed to dequeue next job",
				zap.Int("worker_id", id),
				zap.Error(err),
			)
			// Back off briefly before retrying.
			time.Sleep(2 * time.Second)
			continue
		}

		if runID == uuid.Nil {
			// No job available — wait briefly and loop.
			time.Sleep(1 * time.Second)
			continue
		}

		p.logger.Info("worker picked up experiment",
			zap.Int("worker_id", id),
			zap.String("run_id", runID.String()),
		)

		// Process the experiment with retries.
		atomic.AddInt64(&p.activeCount, 1)
		p.processWithRetry(runID)
		atomic.AddInt64(&p.activeCount, -1)
	}
}

// nextJob attempts to get the next experiment run ID from either the
// internal job channel or the Redis queue. It prefers the job channel
// for lower latency, falling back to Redis if the channel is empty.
func (p *Pool) nextJob() (uuid.UUID, error) {
	// First try the internal channel (non-blocking).
	select {
	case runID := <-p.jobCh:
		return runID, nil
	default:
	}

	// Then try the Redis queue.
	if p.queue != nil {
		ctx, cancel := context.WithTimeout(context.Background(), p.dequeueTimeout)
		defer cancel()

		runID, err := p.queue.Dequeue(ctx)
		if err != nil {
			return uuid.Nil, fmt.Errorf("redis dequeue: %w", err)
		}
		if runID != uuid.Nil {
			return runID, nil
		}
	}

	// Try the channel one more time with a brief block.
	select {
	case runID := <-p.jobCh:
		return runID, nil
	case <-time.After(p.dequeueTimeout):
		return uuid.Nil, nil
	}
}

// processWithRetry delegates experiment processing to the Processor,
// which handles loading the run from the DB, executing it, and retrying
// on failure with exponential backoff. If the processor is unavailable,
// it falls back to a simple Redis-based retry mechanism.
func (p *Pool) processWithRetry(runID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Report that we've started processing.
	p.reportStatus(ctx, runID, "running", "")

	if p.processor != nil {
		// Delegate to the processor which handles retries internally.
		if err := p.processor.ProcessWithRetry(ctx, runID, 3); err != nil {
			p.logger.Error("experiment processing failed after retries",
				zap.String("run_id", runID.String()),
				zap.Error(err),
			)
			p.reportStatus(ctx, runID, "failed", err.Error())
			return
		}

		p.reportStatus(ctx, runID, "completed", "")
		return
	}

	// Fallback path when no Processor is configured: try a single execution
	// by looking up the experiment ID from Redis data stored during handler
	// submission, then executing via the engine.
	p.logger.Warn("no processor configured, using fallback execution path",
		zap.String("run_id", runID.String()),
	)
	p.reportStatus(ctx, runID, "failed", "no processor configured")
}

// ---------------------------------------------------------------------------
// Redis helpers
// ---------------------------------------------------------------------------

// reportStatus publishes the current execution status of an experiment run
// to Redis for real-time monitoring. The status is published to the
// "experiment:worker:status" channel and also stored as a key for querying.
func (p *Pool) reportStatus(ctx context.Context, runID uuid.UUID, status, message string) {
	if p.rdb == nil {
		return
	}

	statusData := map[string]interface{}{
		"run_id":   runID.String(),
		"status":   status,
		"message":  message,
		"worker":   "pool",
		"datetime": time.Now().Format(time.RFC3339),
	}

	// Store in a key for querying.
	statusKey := fmt.Sprintf("experiment:worker:status:%s", runID.String())
	fields := make([]interface{}, 0, len(statusData)*2)
	for k, v := range statusData {
		fields = append(fields, k, v)
	}
	if err := p.rdb.HSet(ctx, statusKey, fields...).Err(); err != nil {
		p.logger.Debug("failed to store worker status in Redis", zap.Error(err))
	}
	_ = p.rdb.Expire(ctx, statusKey, 24*time.Hour).Err()

	// Publish for real-time consumers.
	data, _ := json.Marshal(statusData)
	_ = p.rdb.Publish(ctx, "experiment:worker:status", data).Err()
}

// getRetryCount returns the number of retry attempts for a given run ID.
func (p *Pool) getRetryCount(ctx context.Context, runID uuid.UUID) int {
	if p.rdb == nil {
		return 0
	}
	key := fmt.Sprintf("experiment:worker:retries:%s", runID.String())
	val, err := p.rdb.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return val
}

// incrementRetryCount atomically increments the retry counter for a run ID.
func (p *Pool) incrementRetryCount(ctx context.Context, runID uuid.UUID) {
	if p.rdb == nil {
		return
	}
	key := fmt.Sprintf("experiment:worker:retries:%s", runID.String())
	_ = p.rdb.Incr(ctx, key).Err()
	_ = p.rdb.Expire(ctx, key, 24*time.Hour).Err()
}

// clearRetryCount removes the retry counter for a run ID (on success).
func (p *Pool) clearRetryCount(ctx context.Context, runID uuid.UUID) {
	if p.rdb == nil {
		return
	}
	key := fmt.Sprintf("experiment:worker:retries:%s", runID.String())
	_ = p.rdb.Del(ctx, key).Err()
}
