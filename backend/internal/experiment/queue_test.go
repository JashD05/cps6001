package experiment

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// helper: create a Queue connected to Redis (skip if unavailable)
// ---------------------------------------------------------------------------

func helperNewQueue(t *testing.T) *Queue {
	t.Helper()
	addr := os.Getenv("REDIS_URL")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping: Redis not available at %s: %v", addr, err)
	}
	// Clean up the test key before we start.
	rdb.Del(ctx, QueueKey)
	t.Cleanup(func() {
		rdb.Del(context.Background(), QueueKey)
		rdb.Close()
	})
	return NewQueue(rdb, zap.NewNop())
}

// ---------------------------------------------------------------------------
// Unit tests: compositeScore (no Redis needed)
// ---------------------------------------------------------------------------

func TestCompositeScore_HigherPriorityHasHigherScore(t *testing.T) {
	now := time.Now()
	low := compositeScore(1, now)
	high := compositeScore(10, now)
	if high <= low {
		t.Errorf("higher priority should have higher score: got low=%v high=%v", low, high)
	}
}

func TestCompositeScore_SamePriorityEarlierTimeHasHigherScore(t *testing.T) {
	earlier := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	scoreEarlier := compositeScore(5, earlier)
	scoreLater := compositeScore(5, later)
	if scoreEarlier <= scoreLater {
		t.Errorf("earlier time at same priority should have higher score: earlier=%v later=%v", scoreEarlier, scoreLater)
	}
}

func TestCompositeScore_PriorityDominatesTimeDifference(t *testing.T) {
	// A low-priority item enqueued much earlier should still score lower
	// than a high-priority item enqueued much later.
	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	lowPriorityEarly := compositeScore(1, early)
	highPriorityLate := compositeScore(10, late)
	if highPriorityLate <= lowPriorityEarly {
		t.Errorf("priority should dominate time: low(1,early)=%v high(10,late)=%v", lowPriorityEarly, highPriorityLate)
	}
}

func TestCompositeScore_ZeroPriority(t *testing.T) {
	now := time.Now()
	score := compositeScore(0, now)
	if score < 0 {
		t.Errorf("zero priority score should be non-negative: got %v", score)
	}
}

func TestCompositeScore_NegativePriority(t *testing.T) {
	now := time.Now()
	neg := compositeScore(-1, now)
	zero := compositeScore(0, now)
	if neg >= zero {
		t.Errorf("negative priority should score lower than zero: neg=%v zero=%v", neg, zero)
	}
}

func TestCompositeScore_LargePriority(t *testing.T) {
	now := time.Now()
	score := compositeScore(1000, now)
	if score <= 0 {
		t.Errorf("large priority should produce a large positive score: got %v", score)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: parsePriorityFromScore (no Redis needed)
// ---------------------------------------------------------------------------

func TestParsePriorityFromScore_RoundTrip(t *testing.T) {
	priorities := []int{0, 1, 5, 10, 100, 999}
	now := time.Now()
	for _, p := range priorities {
		score := compositeScore(p, now)
		got := parsePriorityFromScore(score)
		if got != p {
			t.Errorf("parsePriorityFromScore(compositeScore(%d)) = %d, want %d", p, got, p)
		}
	}
}

func TestParsePriorityFromScore_ZeroScore(t *testing.T) {
	got := parsePriorityFromScore(0)
	if got != 0 {
		t.Errorf("parsePriorityFromScore(0) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Queue with Redis
// ---------------------------------------------------------------------------

func TestQueue_EnqueueAndDequeue(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	if err := q.Enqueue(ctx, id, 5); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got != id {
		t.Errorf("Dequeue returned %s, want %s", got, id)
	}

	// Queue should now be empty.
	empty, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue on empty queue failed: %v", err)
	}
	if empty != uuid.Nil {
		t.Errorf("Dequeue on empty queue returned %s, want uuid.Nil", empty)
	}
}

func TestQueue_PriorityOrdering(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	low := uuid.New()
	medium := uuid.New()
	high := uuid.New()

	// Enqueue in random priority order.
	q.Enqueue(ctx, low, 1)
	q.Enqueue(ctx, high, 10)
	q.Enqueue(ctx, medium, 5)

	// Should dequeue in priority order: high, medium, low.
	first, _ := q.Dequeue(ctx)
	second, _ := q.Dequeue(ctx)
	third, _ := q.Dequeue(ctx)

	if first != high {
		t.Errorf("first dequeue = %s, want high (%s)", first, high)
	}
	if second != medium {
		t.Errorf("second dequeue = %s, want medium (%s)", second, medium)
	}
	if third != low {
		t.Errorf("third dequeue = %s, want low (%s)", third, low)
	}
}

func TestQueue_FIFOWithinSamePriority(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	first := uuid.New()
	second := uuid.New()
	third := uuid.New()

	q.Enqueue(ctx, first, 5)
	time.Sleep(2 * time.Millisecond) // small delay to ensure different timestamps
	q.Enqueue(ctx, second, 5)
	time.Sleep(2 * time.Millisecond)
	q.Enqueue(ctx, third, 5)

	got1, _ := q.Dequeue(ctx)
	got2, _ := q.Dequeue(ctx)
	got3, _ := q.Dequeue(ctx)

	if got1 != first {
		t.Errorf("FIFO: first dequeue = %s, want %s", got1, first)
	}
	if got2 != second {
		t.Errorf("FIFO: second dequeue = %s, want %s", got2, second)
	}
	if got3 != third {
		t.Errorf("FIFO: third dequeue = %s, want %s", got3, third)
	}
}

func TestQueue_DequeueFromEmptyQueue(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue on empty queue returned error: %v", err)
	}
	if id != uuid.Nil {
		t.Errorf("Dequeue on empty queue returned %s, want uuid.Nil", id)
	}
}

func TestQueue_Remove(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	q.Enqueue(ctx, id, 5)

	if err := q.Remove(ctx, id); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	got, _ := q.Dequeue(ctx)
	if got != uuid.Nil {
		t.Errorf("Dequeue after remove returned %s, want uuid.Nil", got)
	}
}

func TestQueue_RemoveNonExistentItem(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	if err := q.Remove(ctx, id); err != nil {
		t.Errorf("Remove of non-existent item should not error: %v", err)
	}
}

func TestQueue_Position(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	low := uuid.New()
	high := uuid.New()

	q.Enqueue(ctx, low, 1)
	q.Enqueue(ctx, high, 10)

	posHigh, err := q.Position(ctx, high)
	if err != nil {
		t.Fatalf("Position failed: %v", err)
	}
	posLow, err := q.Position(ctx, low)
	if err != nil {
		t.Fatalf("Position failed: %v", err)
	}

	if posHigh != 0 {
		t.Errorf("high priority position = %d, want 0", posHigh)
	}
	if posLow != 1 {
		t.Errorf("low priority position = %d, want 1", posLow)
	}
}

func TestQueue_PositionNonExistentItem(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	pos, err := q.Position(ctx, id)
	if err != nil {
		t.Fatalf("Position failed: %v", err)
	}
	if pos != -1 {
		t.Errorf("Position of non-existent item = %d, want -1", pos)
	}
}

func TestQueue_Length(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	length, err := q.Length(ctx)
	if err != nil {
		t.Fatalf("Length on empty queue failed: %v", err)
	}
	if length != 0 {
		t.Errorf("Length of empty queue = %d, want 0", length)
	}

	q.Enqueue(ctx, uuid.New(), 1)
	q.Enqueue(ctx, uuid.New(), 2)

	length, err = q.Length(ctx)
	if err != nil {
		t.Fatalf("Length failed: %v", err)
	}
	if length != 2 {
		t.Errorf("Length = %d, want 2", length)
	}
}

func TestQueue_List(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	low := uuid.New()
	high := uuid.New()

	q.Enqueue(ctx, low, 1)
	q.Enqueue(ctx, high, 10)

	ids, err := q.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("List returned %d items, want 2", len(ids))
	}
	// Highest priority first.
	if ids[0] != high {
		t.Errorf("List[0] = %s, want high (%s)", ids[0], high)
	}
	if ids[1] != low {
		t.Errorf("List[1] = %s, want low (%s)", ids[1], low)
	}
}

func TestQueue_ListEmptyQueue(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	ids, err := q.List(ctx)
	if err != nil {
		t.Fatalf("List on empty queue failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("List on empty queue returned %d items, want 0", len(ids))
	}
}

func TestQueue_Peek(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	high := uuid.New()
	low := uuid.New()

	q.Enqueue(ctx, low, 1)
	q.Enqueue(ctx, high, 10)

	peeked, err := q.Peek(ctx)
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if peeked != high {
		t.Errorf("Peek returned %s, want %s", peeked, high)
	}

	// Peek should not remove the item.
	length, _ := q.Length(ctx)
	if length != 2 {
		t.Errorf("Length after Peek = %d, want 2", length)
	}
}

func TestQueue_PeekEmptyQueue(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id, err := q.Peek(ctx)
	if err != nil {
		t.Fatalf("Peek on empty queue returned error: %v", err)
	}
	if id != uuid.Nil {
		t.Errorf("Peek on empty queue returned %s, want uuid.Nil", id)
	}
}

func TestQueue_IsQueued(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	q.Enqueue(ctx, id, 5)

	queued, err := q.IsQueued(ctx, id)
	if err != nil {
		t.Fatalf("IsQueued failed: %v", err)
	}
	if !queued {
		t.Error("IsQueued returned false for enqueued item")
	}

	other := uuid.New()
	queued, err = q.IsQueued(ctx, other)
	if err != nil {
		t.Fatalf("IsQueued failed: %v", err)
	}
	if queued {
		t.Error("IsQueued returned true for non-enqueued item")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	q.Enqueue(ctx, uuid.New(), 1)
	q.Enqueue(ctx, uuid.New(), 2)
	q.Enqueue(ctx, uuid.New(), 3)

	if err := q.Clear(ctx); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	length, _ := q.Length(ctx)
	if length != 0 {
		t.Errorf("Length after Clear = %d, want 0", length)
	}
}

func TestQueue_ListWithScores(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	low := uuid.New()
	high := uuid.New()

	q.Enqueue(ctx, low, 1)
	q.Enqueue(ctx, high, 10)

	entries, err := q.ListWithScores(ctx)
	if err != nil {
		t.Fatalf("ListWithScores failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListWithScores returned %d items, want 2", len(entries))
	}

	// Should be ordered by priority (high first).
	if entries[0].Priority != 10 {
		t.Errorf("entries[0].Priority = %d, want 10", entries[0].Priority)
	}
	if entries[1].Priority != 1 {
		t.Errorf("entries[1].Priority = %d, want 1", entries[1].Priority)
	}
}

func TestQueue_DequeueBlocking_ContextCancellation(t *testing.T) {
	q := helperNewQueue(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := q.DequeueBlocking(ctx, 50*time.Millisecond)
	if err == nil {
		t.Error("DequeueBlocking should return error on context cancellation")
	}
}

func TestQueue_DequeueBlocking_ImmediateItem(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	id := uuid.New()
	q.Enqueue(ctx, id, 5)

	got, err := q.DequeueBlocking(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("DequeueBlocking with item failed: %v", err)
	}
	if got != id {
		t.Errorf("DequeueBlocking returned %s, want %s", got, id)
	}
}

func TestQueue_MultiplePriorities(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	ids := make(map[int]uuid.UUID)
	priorities := []int{0, 3, 7, 5, 1, 10, 2}
	for _, p := range priorities {
		id := uuid.New()
		ids[p] = id
		q.Enqueue(ctx, id, p)
		time.Sleep(time.Millisecond) // ensure timestamp ordering
	}

	// Dequeue all and collect priorities in order.
	var dequeuedPriorities []int
	for i := 0; i < len(priorities); i++ {
		id, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("Dequeue %d failed: %v", i, err)
		}
		// Find the priority for this ID.
		for p, pid := range ids {
			if pid == id {
				dequeuedPriorities = append(dequeuedPriorities, p)
				break
			}
		}
	}

	// Verify priorities are in descending order.
	if !sort.IntsAreSorted(dequeuedPriorities) {
		t.Errorf("priorities not sorted descending: %v", dequeuedPriorities)
	}
	// Actually, we want descending order, so reverse-sorted means each
	// element should be >= the next.
	for i := 0; i < len(dequeuedPriorities)-1; i++ {
		if dequeuedPriorities[i] < dequeuedPriorities[i+1] {
			t.Errorf("priority %d at position %d is less than priority %d at position %d",
				dequeuedPriorities[i], i, dequeuedPriorities[i+1], i+1)
		}
	}
}

func TestQueue_EnqueueMany(t *testing.T) {
	q := helperNewQueue(t)
	ctx := context.Background()

	const count = 100
	ids := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		ids[i] = uuid.New()
		q.Enqueue(ctx, ids[i], i%10)
	}

	length, err := q.Length(ctx)
	if err != nil {
		t.Fatalf("Length failed: %v", err)
	}
	if length != count {
		t.Errorf("Length = %d, want %d", length, count)
	}
}
