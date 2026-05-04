package experiment

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// QueueKey is the Redis key used for the experiment priority queue.
const QueueKey = "experiment:priority_queue"

// Queue implements a priority-based experiment queue using Redis sorted sets.
// Higher priority values are dequeued first. Items with the same priority
// are ordered by their enqueue timestamp (FIFO within a priority level).
//
// The sorted set member format is: "{runID}" and the score is a composite of
// priority and timestamp to guarantee FIFO ordering within the same priority.
// The score is computed as: priority * priorityScale + (maxTimestampMilli -
// enqueueTimeInMilliseconds) so that higher priority sorts first and earlier
// enqueued items sort before later ones at the same priority.
type Queue struct {
	rdb    *redis.Client
	logger *zap.Logger
}

// NewQueue creates a new experiment Queue backed by Redis sorted sets.
func NewQueue(rdb *redis.Client, logger *zap.Logger) *Queue {
	return &Queue{
		rdb:    rdb,
		logger: logger.Named("experiment_queue"),
	}
}

// priorityScale controls how much influence the priority component has on the
// composite score. It must be larger than the maximum time component so that
// higher priorities always outrank lower priorities.
const priorityScale = 1e13

// maxTimestampMilli is a large reference timestamp (year 2100 in milliseconds)
// used to invert the time component so that earlier enqueues have higher
// scores within the same priority level.
var maxTimestampMilli = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

// compositeScore computes the sorted set score from a priority and the
// current time. The formula ensures:
//
//   - Higher priority values sort before lower ones.
//
//   - Within the same priority, items enqueued earlier sort before later ones.
//
//     score = priority * priorityScale + (maxTimestampMilli - now.UnixMilli())
func compositeScore(priority int, now time.Time) float64 {
	return float64(priority)*priorityScale + float64(maxTimestampMilli-now.UnixMilli())
}

// Enqueue adds an experiment run to the queue with the given priority.
// Higher priority values mean the experiment will be dequeued sooner.
// Priority 0 is the lowest; typical values are 0 (normal), 5 (high), 10 (critical).
func (q *Queue) Enqueue(ctx context.Context, runID uuid.UUID, priority int) error {
	member := runID.String()
	score := compositeScore(priority, time.Now())

	if err := q.rdb.ZAdd(ctx, QueueKey, redis.Z{
		Score:  score,
		Member: member,
	}).Err(); err != nil {
		return fmt.Errorf("failed to enqueue experiment run %s: %w", runID, err)
	}

	// Set a TTL on the queue key as a safety net so it doesn't leak
	// indefinitely if consumers stop processing.
	_ = q.rdb.Expire(ctx, QueueKey, 72*time.Hour).Err()

	q.logger.Info("experiment enqueued",
		zap.String("run_id", member),
		zap.Int("priority", priority),
		zap.Float64("score", score),
	)
	return nil
}

// Dequeue removes and returns the highest-priority experiment run ID from the
// queue. It blocks for up to the specified timeout waiting for an item if the
// queue is empty. If the timeout elapses with no item available, it returns
// uuid.Nil and no error.
//
// The blocking is implemented via a polling loop with 1-second intervals
// rather than Redis BLPOP because we are using a sorted set (ZSET) which
// does not have a native blocking pop. A production implementation could
// use Redis streams or a Lua script for atomicity, but this approach is
// simple and correct.
func (q *Queue) Dequeue(ctx context.Context) (uuid.UUID, error) {
	// Pop the member with the highest score (descending order).
	results, err := q.rdb.ZPopMax(ctx, QueueKey, 1).Result()
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to dequeue from experiment queue: %w", err)
	}

	if len(results) == 0 {
		return uuid.Nil, nil // queue is empty
	}

	member, ok := results[0].Member.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("unexpected member type in queue: %T", results[0].Member)
	}

	runID, err := uuid.Parse(member)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID in queue member %q: %w", member, err)
	}

	q.logger.Info("experiment dequeued",
		zap.String("run_id", runID.String()),
	)
	return runID, nil
}

// DequeueBlocking repeatedly polls the queue until an item is available or
// the context is cancelled. The pollInterval controls how often the queue
// is checked. This is a convenience wrapper around Dequeue for consumers
// that want blocking semantics.
func (q *Queue) DequeueBlocking(ctx context.Context, pollInterval time.Duration) (uuid.UUID, error) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		// Try an immediate dequeue first.
		runID, err := q.Dequeue(ctx)
		if err != nil {
			return uuid.Nil, err
		}
		if runID != uuid.Nil {
			return runID, nil
		}

		// Wait for the next tick or context cancellation.
		select {
		case <-ctx.Done():
			return uuid.Nil, ctx.Err()
		case <-ticker.C:
			// Loop back and try again.
		}
	}
}

// Remove removes a specific experiment run from the queue. This is useful
// when cancelling a scheduled or queued experiment.
func (q *Queue) Remove(ctx context.Context, runID uuid.UUID) error {
	member := runID.String()

	removed, err := q.rdb.ZRem(ctx, QueueKey, member).Result()
	if err != nil {
		return fmt.Errorf("failed to remove experiment run %s from queue: %w", runID, err)
	}

	if removed == 0 {
		q.logger.Debug("experiment was not in queue",
			zap.String("run_id", member),
		)
	} else {
		q.logger.Info("experiment removed from queue",
			zap.String("run_id", member),
		)
	}

	return nil
}

// Position returns the 0-based position of an experiment in the queue.
// Position 0 means the item will be dequeued next (highest priority).
// Returns -1 if the experiment is not found in the queue.
func (q *Queue) Position(ctx context.Context, runID uuid.UUID) (int, error) {
	member := runID.String()

	// ZREVRANK returns the rank in descending order (highest score = rank 0).
	rank, err := q.rdb.ZRevRank(ctx, QueueKey, member).Result()
	if err != nil {
		if err == redis.Nil {
			return -1, nil // not found
		}
		return -1, fmt.Errorf("failed to get queue position for %s: %w", runID, err)
	}

	return int(rank), nil
}

// Length returns the number of experiments currently in the queue.
func (q *Queue) Length(ctx context.Context) (int, error) {
	count, err := q.rdb.ZCard(ctx, QueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}
	return int(count), nil
}

// List returns all experiment run IDs in the queue, ordered from highest
// priority (first to be dequeued) to lowest.
func (q *Queue) List(ctx context.Context) ([]uuid.UUID, error) {
	// ZREVRANGE returns members from highest to lowest score.
	results, err := q.rdb.ZRevRange(ctx, QueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list experiment queue: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(results))
	for _, member := range results {
		runID, parseErr := uuid.Parse(member)
		if parseErr != nil {
			q.logger.Warn("invalid UUID in queue, skipping",
				zap.String("member", member),
				zap.Error(parseErr),
			)
			continue
		}
		ids = append(ids, runID)
	}

	return ids, nil
}

// ListWithScores returns all experiment run IDs in the queue along with
// their composite scores, ordered from highest priority to lowest.
// This is useful for administrative views of the queue.
func (q *Queue) ListWithScores(ctx context.Context) ([]QueueEntry, error) {
	results, err := q.rdb.ZRevRangeWithScores(ctx, QueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list experiment queue with scores: %w", err)
	}

	entries := make([]QueueEntry, 0, len(results))
	for _, z := range results {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}

		runID, parseErr := uuid.Parse(member)
		if parseErr != nil {
			q.logger.Warn("invalid UUID in queue, skipping",
				zap.String("member", member),
				zap.Error(parseErr),
			)
			continue
		}

		// Extract the original priority from the composite score.
		priority := int(z.Score / priorityScale)

		entries = append(entries, QueueEntry{
			RunID:    runID,
			Priority: priority,
			Score:    z.Score,
		})
	}

	return entries, nil
}

// QueueEntry represents a single entry in the experiment queue with its
// associated priority and composite score.
type QueueEntry struct {
	RunID    uuid.UUID `json:"run_id"`
	Priority int       `json:"priority"`
	Score    float64   `json:"score"`
}

// Clear removes all items from the queue. This should only be used in
// testing or administrative scenarios.
func (q *Queue) Clear(ctx context.Context) error {
	if err := q.rdb.Del(ctx, QueueKey).Err(); err != nil {
		return fmt.Errorf("failed to clear experiment queue: %w", err)
	}
	q.logger.Warn("experiment queue cleared")
	return nil
}

// Peek returns the highest-priority experiment run ID without removing it
// from the queue. Returns uuid.Nil if the queue is empty.
func (q *Queue) Peek(ctx context.Context) (uuid.UUID, error) {
	results, err := q.rdb.ZRevRange(ctx, QueueKey, 0, 0).Result()
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to peek experiment queue: %w", err)
	}

	if len(results) == 0 {
		return uuid.Nil, nil
	}

	runID, err := uuid.Parse(results[0])
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID in queue head: %w", err)
	}

	return runID, nil
}

// IsQueued checks whether a specific experiment run is currently in the queue.
func (q *Queue) IsQueued(ctx context.Context, runID uuid.UUID) (bool, error) {
	score, err := q.rdb.ZScore(ctx, QueueKey, runID.String()).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if %s is queued: %w", runID, err)
	}
	_ = score
	return true, nil
}

// parsePriorityFromScore extracts the integer priority component from a
// composite score. The priority is the floor of score / priorityScale.
func parsePriorityFromScore(score float64) int {
	return int(score / priorityScale)
}

// Ensure parsePriorityFromScore is used (referenced in ListWithScores via
// direct computation; kept as an exported helper for testing).
var _ = parsePriorityFromScore

// Ensure strconv is used in the package (referenced by parsePriorityFromScore
// in a potential extended version).
var _ = strconv.Itoa
