package middleware

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Constants and defaults
// ---------------------------------------------------------------------------

const (
	// DefaultWindow is the default sliding window duration for metrics retention.
	DefaultWindow = 5 * time.Minute

	// DefaultMaxEndpoints is the default number of slowest endpoints to track.
	DefaultMaxEndpoints = 10
)

// ---------------------------------------------------------------------------
// requestMetric
// ---------------------------------------------------------------------------

// requestMetric holds timing and metadata for a single HTTP request.
type requestMetric struct {
	timestamp    time.Time
	method       string
	path         string
	statusCode   int
	duration     time.Duration
	responseSize int
}

// ---------------------------------------------------------------------------
// EndpointStats
// ---------------------------------------------------------------------------

// EndpointStats holds aggregated statistics for a single endpoint (method+path).
type EndpointStats struct {
	Method      string  `json:"method"`
	Path        string  `json:"path"`
	Count       int     `json:"count"`
	AvgDuration float64 `json:"avg_duration_ms"`
	MaxDuration float64 `json:"max_duration_ms"`
	ErrorCount  int     `json:"error_count"`
}

// ---------------------------------------------------------------------------
// MetricsSnapshot
// ---------------------------------------------------------------------------

// MetricsSnapshot represents a point-in-time view of all profiling metrics.
// It is returned by GetMetrics and serialized by the metrics handler.
type MetricsSnapshot struct {
	WindowSeconds     float64         `json:"window_seconds"`
	TotalRequests     int             `json:"total_requests"`
	RequestsPerSecond float64         `json:"requests_per_second"`
	AvgResponseTime   float64         `json:"avg_response_time_ms"`
	P50Latency        float64         `json:"p50_latency_ms"`
	P95Latency        float64         `json:"p95_latency_ms"`
	P99Latency        float64         `json:"p99_latency_ms"`
	MinLatency        float64         `json:"min_latency_ms"`
	MaxLatency        float64         `json:"max_latency_ms"`
	ErrorRate         float64         `json:"error_rate"`
	ClientErrorCount  int             `json:"client_error_count"`
	ServerErrorCount  int             `json:"server_error_count"`
	SlowestEndpoints  []EndpointStats `json:"slowest_endpoints"`
	InFlightRequests  int64           `json:"in_flight_requests"`
	StatusCodes       map[int]int     `json:"status_codes"`
	ComputedAt        time.Time       `json:"computed_at"`
}

// ---------------------------------------------------------------------------
// Profiler
// ---------------------------------------------------------------------------

// Profiler collects and aggregates HTTP request metrics using a sliding window.
// It is safe for concurrent use via sync.RWMutex.
type Profiler struct {
	mu        sync.RWMutex
	logger    *zap.Logger
	window    time.Duration
	metrics   []requestMetric
	inFlight  atomic.Int64
	startTime time.Time
	stopCh    chan struct{}
}

// NewProfiler creates a new Profiler with the given sliding window duration
// and logger. If window <= 0, DefaultWindow is used. If logger is nil,
// a no-op logger is used.
func NewProfiler(window time.Duration, logger *zap.Logger) *Profiler {
	if window <= 0 {
		window = DefaultWindow
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	p := &Profiler{
		window:    window,
		logger:    logger,
		metrics:   make([]requestMetric, 0, 1024),
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
	}

	// Background goroutine to periodically trim stale metrics from the window.
	go p.trimLoop()

	return p
}

// Window returns the configured sliding window duration.
func (p *Profiler) Window() time.Duration {
	return p.window
}

// Stop signals the background trimming goroutine to exit and waits for it to
// finish. It is safe to call Stop multiple times. After Stop is called, the
// Profiler will no longer trim old metrics automatically (though recording
// will still work and record() still trims inline).
func (p *Profiler) Stop() {
	select {
	case <-p.stopCh:
		// Already closed.
	default:
		close(p.stopCh)
	}
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// Middleware returns a Gin handler function that records timing and metadata
// for every request that passes through. It increments in-flight counters
// on entry and decrements them on exit, records duration, and stores the
// metric for later aggregation.
func (p *Profiler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		p.inFlight.Add(1)
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		p.inFlight.Add(-1)

		metric := requestMetric{
			timestamp:    start,
			method:       c.Request.Method,
			path:         c.FullPath(), // use the matched route, not the raw path
			statusCode:   c.Writer.Status(),
			duration:     duration,
			responseSize: c.Writer.Size(),
		}

		// If FullPath() returns empty (no matched route), fall back to the
		// request path so that unmatched/404 routes are still tracked.
		if metric.path == "" {
			metric.path = c.Request.URL.Path
		}

		p.record(metric)

		p.logger.Debug("request_profiled",
			zap.String("method", metric.method),
			zap.String("path", metric.path),
			zap.Int("status", metric.statusCode),
			zap.Duration("duration", metric.duration),
			zap.Int("response_size", metric.responseSize),
		)
	}
}

// record appends a metric to the sliding window and trims any stale entries.
func (p *Profiler) record(m requestMetric) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metrics = append(p.metrics, m)
	p.trimLocked()
}

// ---------------------------------------------------------------------------
// Sliding window management
// ---------------------------------------------------------------------------

// trimLocked removes metrics that have fallen outside the sliding window.
// Caller must hold p.mu for writing.
func (p *Profiler) trimLocked() {
	cutoff := time.Now().Add(-p.window)
	i := 0
	for i < len(p.metrics) && p.metrics[i].timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		p.metrics = p.metrics[i:]
	}
}

// trimLoop runs a background goroutine that periodically removes stale metrics
// to prevent unbounded memory growth even when traffic is low. It exits when
// the stopCh channel is closed.
func (p *Profiler) trimLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			p.trimLocked()
			p.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// GetMetrics
// ---------------------------------------------------------------------------

// GetMetrics computes and returns a snapshot of all profiling metrics within
// the current sliding window. It is safe to call from any goroutine.
func (p *Profiler) GetMetrics() *MetricsSnapshot {
	p.mu.Lock()
	// Trim first so the snapshot reflects the active sliding window even if
	// the background cleanup goroutine has not run yet.
	p.trimLocked()
	// Copy metrics slice to avoid holding the lock during computation.
	metrics := make([]requestMetric, len(p.metrics))
	copy(metrics, p.metrics)
	p.mu.Unlock()

	snapshot := &MetricsSnapshot{
		WindowSeconds:    p.window.Seconds(),
		TotalRequests:    len(metrics),
		InFlightRequests: p.inFlight.Load(),
		StatusCodes:      make(map[int]int),
		SlowestEndpoints: []EndpointStats{},
		ComputedAt:       time.Now(),
	}

	if len(metrics) == 0 {
		return snapshot
	}

	// ---- Aggregate basic stats ----
	totalDuration := time.Duration(0)
	clientErrors := 0
	serverErrors := 0

	durations := make([]time.Duration, 0, len(metrics))
	endpointMap := make(map[string]*EndpointStats) // key: "METHOD path"

	for i := range metrics {
		m := &metrics[i]
		totalDuration += m.duration
		durations = append(durations, m.duration)

		snapshot.StatusCodes[m.statusCode]++

		if m.statusCode >= 400 && m.statusCode < 500 {
			clientErrors++
		}
		if m.statusCode >= 500 {
			serverErrors++
		}

		// Aggregate per-endpoint statistics.
		key := m.method + " " + m.path
		stats, ok := endpointMap[key]
		if !ok {
			stats = &EndpointStats{
				Method: m.method,
				Path:   m.path,
			}
			endpointMap[key] = stats
		}
		stats.Count++
		stats.AvgDuration += float64(m.duration.Microseconds()) / 1000.0 // ms
		if float64(m.duration.Microseconds())/1000.0 > stats.MaxDuration {
			stats.MaxDuration = float64(m.duration.Microseconds()) / 1000.0
		}
		if m.statusCode >= 400 {
			stats.ErrorCount++
		}
	}

	// ---- Finalize averages per endpoint ----
	for _, stats := range endpointMap {
		stats.AvgDuration /= float64(stats.Count)
	}

	// ---- Requests per second ----
	elapsed := snapshot.ComputedAt.Sub(metrics[0].timestamp).Seconds()
	if elapsed <= 0 {
		elapsed = p.window.Seconds()
	}
	snapshot.RequestsPerSecond = float64(len(metrics)) / elapsed

	// ---- Average response time ----
	snapshot.AvgResponseTime = float64(totalDuration.Microseconds()) / float64(len(metrics)) / 1000.0 // ms

	// ---- Client and server error counts ----
	snapshot.ClientErrorCount = clientErrors
	snapshot.ServerErrorCount = serverErrors

	// ---- Error rate ----
	snapshot.ErrorRate = float64(clientErrors+serverErrors) / float64(len(metrics))

	// ---- Percentile latency ----
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	snapshot.MinLatency = float64(durations[0].Microseconds()) / 1000.0
	snapshot.MaxLatency = float64(durations[len(durations)-1].Microseconds()) / 1000.0
	snapshot.P50Latency = percentile(durations, 0.50)
	snapshot.P95Latency = percentile(durations, 0.95)
	snapshot.P99Latency = percentile(durations, 0.99)

	// ---- Slowest endpoints (top 10 by avg duration) ----
	allEndpoints := make([]EndpointStats, 0, len(endpointMap))
	for _, stats := range endpointMap {
		allEndpoints = append(allEndpoints, *stats)
	}
	sort.Slice(allEndpoints, func(i, j int) bool {
		return allEndpoints[i].AvgDuration > allEndpoints[j].AvgDuration
	})
	limit := DefaultMaxEndpoints
	if len(allEndpoints) < limit {
		limit = len(allEndpoints)
	}
	snapshot.SlowestEndpoints = allEndpoints[:limit]

	return snapshot
}

// percentile computes the given percentile from a sorted slice of durations
// and returns the value in milliseconds. Uses nearest-rank interpolation.
func percentile(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return float64(sorted[0].Microseconds()) / 1000.0
	}

	// Nearest-rank method.
	rank := p * float64(len(sorted)-1)
	lowerIdx := int(math.Floor(rank))
	upperIdx := int(math.Ceil(rank))

	if lowerIdx == upperIdx || upperIdx >= len(sorted) {
		return float64(sorted[lowerIdx].Microseconds()) / 1000.0
	}

	// Linear interpolation between the two nearest ranks.
	fraction := rank - float64(lowerIdx)
	lowerVal := float64(sorted[lowerIdx].Microseconds()) / 1000.0
	upperVal := float64(sorted[upperIdx].Microseconds()) / 1000.0

	return lowerVal + fraction*(upperVal-lowerVal)
}

// ---------------------------------------------------------------------------
// MetricsHandler
// ---------------------------------------------------------------------------

// MetricsHandler returns a Gin handler that responds to GET requests with a
// JSON representation of the current metrics snapshot. Non-GET requests
// receive a 405 Method Not Allowed response.
func (p *Profiler) MetricsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.AbortWithStatusJSON(http.StatusMethodNotAllowed, gin.H{
				"error":   "method_not_allowed",
				"message": "The metrics endpoint only supports GET requests.",
				"code":    http.StatusMethodNotAllowed,
			})
			return
		}

		snapshot := p.GetMetrics()

		data, err := json.Marshal(snapshot)
		if err != nil {
			p.logger.Error("failed to marshal metrics snapshot", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Failed to serialize metrics.",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	}
}

// ---------------------------------------------------------------------------
// Reset (useful for testing)
// ---------------------------------------------------------------------------

// Reset clears all collected metrics and resets the profiler to a fresh state.
func (p *Profiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.metrics = make([]requestMetric, 0, 1024)
	p.startTime = time.Now()
	p.inFlight.Store(0)
}
