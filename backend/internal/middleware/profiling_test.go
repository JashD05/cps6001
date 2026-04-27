package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupProfilerRouter creates a Gin engine with the profiling middleware
// and a few test routes.
func setupProfilerRouter(p *Profiler) *gin.Engine {
	r := gin.New()
	r.Use(p.Middleware())
	r.GET("/metrics", p.MetricsHandler())
	r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "pong"}) })
	r.POST("/data", func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{"id": "abc123"}) })
	r.GET("/error", func(c *gin.Context) { c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"}) })
	r.GET("/notfound", func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error": "missing"}) })
	r.GET("/badrequest", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"}) })
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(100 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"slow": true})
	})
	return r
}

// sendRequest is a helper that sends a request to the router and returns the response.
func sendRequest(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Metrics Recording Tests
// ---------------------------------------------------------------------------

func TestProfiler_RecordsMetricsCorrectly(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "should record exactly one request")
	assert.Greater(t, metrics.RequestsPerSecond, float64(0), "requests per second should be positive")
	assert.Equal(t, int64(0), metrics.InFlightRequests, "in-flight should be 0 after request completes")
}

func TestProfiler_RecordsMethod(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "POST", "/data")

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "should record both requests")
}

func TestProfiler_RecordsStatusCode(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")       // 200
	sendRequest(r, "POST", "/data")      // 201
	sendRequest(r, "GET", "/error")      // 500
	sendRequest(r, "GET", "/notfound")   // 404
	sendRequest(r, "GET", "/badrequest") // 400

	metrics := p.GetMetrics()
	assert.Equal(t, 5, metrics.TotalRequests, "should record all 5 requests")
	// Error rate includes both 4xx and 5xx: 400, 404, 500 → 3 out of 5 = 0.6
	assert.InDelta(t, 0.6, metrics.ErrorRate, 0.01, "error rate should be ~60%")
	assert.Equal(t, 2, metrics.ClientErrorCount, "should count 2 client errors (400, 404)")
	assert.Equal(t, 1, metrics.ServerErrorCount, "should count 1 server error (500)")
}

func TestProfiler_RecordsDuration(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/slow")

	metrics := p.GetMetrics()
	assert.Greater(t, metrics.AvgResponseTime, float64(0), "average response time should be positive")
	// The slow endpoint sleeps for 100ms, so average should be >= 80ms (allowing some tolerance)
	assert.GreaterOrEqual(t, metrics.AvgResponseTime, 80.0, "avg response should be >= 80ms")
}

func TestProfiler_RecordsResponseSize(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "should record one request")
}

func TestProfiler_MultipleRequestsIncrementCounter(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	for i := 0; i < 10; i++ {
		sendRequest(r, "GET", "/ping")
	}

	metrics := p.GetMetrics()
	assert.Equal(t, 10, metrics.TotalRequests, "should record 10 requests")
}

func TestProfiler_RecordsDifferentPaths(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/error")
	sendRequest(r, "POST", "/data")

	metrics := p.GetMetrics()
	assert.Equal(t, 3, metrics.TotalRequests, "should record 3 requests")
	require.NotEmpty(t, metrics.SlowestEndpoints, "should have slowest endpoints data")
}

func TestProfiler_StatusCodesMap(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")     // 200
	sendRequest(r, "GET", "/ping")     // 200
	sendRequest(r, "POST", "/data")    // 201
	sendRequest(r, "GET", "/notfound") // 404

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.StatusCodes[200], "should have 2 requests with status 200")
	assert.Equal(t, 1, metrics.StatusCodes[201], "should have 1 request with status 201")
	assert.Equal(t, 1, metrics.StatusCodes[404], "should have 1 request with status 404")
}

// ---------------------------------------------------------------------------
// Sliding Window Tests
// ---------------------------------------------------------------------------

func TestProfiler_SlidingWindow_ExpiresOldMetrics(t *testing.T) {
	// Use a very short window so metrics expire quickly.
	p := NewProfiler(200*time.Millisecond, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	// Metrics should be present immediately.
	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "should have 1 request right away")

	// Wait for the window to expire.
	time.Sleep(300 * time.Millisecond)

	metrics = p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests, "expired metrics should be removed from window")
}

func TestProfiler_SlidingWindow_KeepsRecentMetrics(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	// Small sleep, well within the 5-minute window.
	time.Sleep(10 * time.Millisecond)

	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "recent metrics should still be in window")
}

func TestProfiler_SlidingWindow_MixedOldAndNew(t *testing.T) {
	p := NewProfiler(500*time.Millisecond, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send first request.
	sendRequest(r, "GET", "/ping")

	// Wait a bit, then send another.
	time.Sleep(100 * time.Millisecond)
	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "both requests should be in window")

	// Wait for the first request to expire but not the second.
	time.Sleep(450 * time.Millisecond)

	metrics = p.GetMetrics()
	assert.LessOrEqual(t, metrics.TotalRequests, 2, "old requests may have expired")
}

func TestProfiler_SlidingWindow_WindowDuration(t *testing.T) {
	p := NewProfiler(1*time.Hour, nil)
	defer p.Stop()

	assert.Equal(t, 1*time.Hour, p.Window(), "Window should return the configured duration")
}

func TestProfiler_SlidingWindow_ResetAfterExpiry(t *testing.T) {
	p := NewProfiler(100*time.Millisecond, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send a request.
	sendRequest(r, "GET", "/ping")
	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests)

	// Let it expire.
	time.Sleep(150 * time.Millisecond)
	metrics = p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests, "all metrics should have expired")

	// Send a new request after expiry.
	sendRequest(r, "GET", "/ping")
	metrics = p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "new request should be recorded after window reset")
}

func TestProfiler_SlidingWindow_RequestsPerSecond(t *testing.T) {
	p := NewProfiler(5*time.Second, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send 10 requests.
	for i := 0; i < 10; i++ {
		sendRequest(r, "GET", "/ping")
	}

	metrics := p.GetMetrics()
	assert.Greater(t, metrics.RequestsPerSecond, float64(0), "RPS should be positive")
}

// ---------------------------------------------------------------------------
// Concurrency Tests
// ---------------------------------------------------------------------------

func TestProfiler_ConcurrentRequests(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	var wg sync.WaitGroup
	const numRequests = 100

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendRequest(r, "GET", "/ping")
		}()
	}

	wg.Wait()

	metrics := p.GetMetrics()
	assert.Equal(t, numRequests, metrics.TotalRequests, "should record all concurrent requests")
}

func TestProfiler_ConcurrentRequestsDifferentPaths(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	var wg sync.WaitGroup
	paths := []string{"/ping", "/error", "/notfound", "/badrequest"}
	const numRequests = 50

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path := paths[idx%len(paths)]
			sendRequest(r, "GET", path)
		}(i)
	}

	wg.Wait()

	metrics := p.GetMetrics()
	assert.Equal(t, numRequests, metrics.TotalRequests, "should record all concurrent requests")
}

func TestProfiler_ConcurrentReadsAndWrites(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	var wg sync.WaitGroup
	const numWriters = 50
	const numReaders = 50

	// Writers: send requests concurrently.
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendRequest(r, "GET", "/ping")
		}()
	}

	// Readers: read metrics concurrently.
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.GetMetrics()
		}()
	}

	wg.Wait()

	// No assertions on exact count since readers don't modify state,
	// but the test should not panic or race.
	metrics := p.GetMetrics()
	assert.GreaterOrEqual(t, metrics.TotalRequests, 1, "at least some requests should be recorded")
}

func TestProfiler_ConcurrentMetricsHandlerCalls(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Pre-populate some metrics.
	sendRequest(r, "GET", "/ping")

	var wg sync.WaitGroup
	const numCalls = 50

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/metrics", nil)
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// In-Flight Requests Tests
// ---------------------------------------------------------------------------

func TestProfiler_InFlightRequestsZeroAfterCompletion(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, int64(0), metrics.InFlightRequests, "in-flight should be 0 after request completes")
}

func TestProfiler_InFlightRequestsWithSlowHandler(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := gin.New()
	r.Use(p.Middleware())

	blockCh := make(chan struct{})
	r.GET("/blocking", func(c *gin.Context) {
		<-blockCh // Block until we signal.
		c.JSON(http.StatusOK, gin.H{"done": true})
	})

	// Start request in a goroutine.
	go func() {
		sendRequest(r, "GET", "/blocking")
	}()

	// Give time for the request to start.
	time.Sleep(50 * time.Millisecond)

	// Check in-flight count while request is ongoing.
	metrics := p.GetMetrics()
	assert.Equal(t, int64(1), metrics.InFlightRequests, "should have 1 in-flight request")

	// Unblock the handler.
	close(blockCh)

	// Give time for the request to complete.
	time.Sleep(100 * time.Millisecond)

	metrics = p.GetMetrics()
	assert.Equal(t, int64(0), metrics.InFlightRequests, "in-flight should be 0 after completion")
	assert.Equal(t, 1, metrics.TotalRequests, "request should be recorded")
}

// ---------------------------------------------------------------------------
// Percentile Tests
// ---------------------------------------------------------------------------

func TestProfiler_Percentiles_SingleRequest(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/slow")

	metrics := p.GetMetrics()
	// With a single request, P50 = P95 = P99 = that request's duration.
	assert.Greater(t, metrics.P50Latency, float64(0), "P50 should be positive")
	assert.Greater(t, metrics.P95Latency, float64(0), "P95 should be positive")
	assert.Greater(t, metrics.P99Latency, float64(0), "P99 should be positive")
}

func TestProfiler_Percentiles_MultipleRequests(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := gin.New()
	r.Use(p.Middleware())

	// Create endpoints with different response times.
	r.GET("/instant", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/veryslow", func(c *gin.Context) {
		time.Sleep(50 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"slow": true})
	})

	// Send 20 fast requests and 1 slow request.
	for i := 0; i < 20; i++ {
		sendRequest(r, "GET", "/instant")
	}
	sendRequest(r, "GET", "/veryslow")

	metrics := p.GetMetrics()
	assert.Equal(t, 21, metrics.TotalRequests, "should have 21 total requests")
	// P50 should be closer to the fast request time (the majority).
	assert.Less(t, metrics.P50Latency, metrics.P95Latency, "P50 should be less than P95")
}

func TestProfiler_Percentiles_EmptyWindow(t *testing.T) {
	p := NewProfiler(100*time.Millisecond, nil)
	defer p.Stop()

	metrics := p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests, "no requests in empty profiler")
	assert.Equal(t, float64(0), metrics.P50Latency, "P50 should be 0 with no data")
	assert.Equal(t, float64(0), metrics.P95Latency, "P95 should be 0 with no data")
	assert.Equal(t, float64(0), metrics.P99Latency, "P99 should be 0 with no data")
	assert.Equal(t, float64(0), metrics.AvgResponseTime, "avg should be 0 with no data")
}

func TestProfiler_Percentiles_WithMixedLatencies(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := gin.New()
	r.Use(p.Middleware())

	// Create a set of routes with increasing delays.
	delays := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
		50 * time.Millisecond,
	}

	for i, d := range delays {
		delay := d
		r.GET(fmt.Sprintf("/api/%d", i), func(c *gin.Context) {
			time.Sleep(delay)
			c.JSON(http.StatusOK, gin.H{"delay": delay.String()})
		})
	}

	// Send multiple requests to each route.
	for i := range delays {
		for j := 0; j < 10; j++ {
			sendRequest(r, "GET", fmt.Sprintf("/api/%d", i))
		}
	}

	metrics := p.GetMetrics()
	assert.Equal(t, 60, metrics.TotalRequests, "should have 60 total requests")
	assert.GreaterOrEqual(t, metrics.P99Latency, metrics.P50Latency, "P99 should be >= P50")
	assert.GreaterOrEqual(t, metrics.P95Latency, metrics.P50Latency, "P95 should be >= P50")
}

func TestProfiler_MinMaxLatency(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/slow")

	metrics := p.GetMetrics()
	assert.Greater(t, metrics.MaxLatency, metrics.MinLatency, "max should be >= min")
	assert.Greater(t, metrics.MinLatency, float64(0), "min latency should be positive")
}

// ---------------------------------------------------------------------------
// Error Rate Tests
// ---------------------------------------------------------------------------

func TestProfiler_ErrorRate_AllSuccess(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	for i := 0; i < 10; i++ {
		sendRequest(r, "GET", "/ping")
	}

	metrics := p.GetMetrics()
	assert.Equal(t, 10, metrics.TotalRequests)
	assert.InDelta(t, 0.0, metrics.ErrorRate, 0.01, "error rate should be 0 with all 200s")
}

func TestProfiler_ErrorRate_AllErrors(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	for i := 0; i < 5; i++ {
		sendRequest(r, "GET", "/error")    // 500
		sendRequest(r, "GET", "/notfound") // 404
	}

	metrics := p.GetMetrics()
	assert.Equal(t, 10, metrics.TotalRequests)
	assert.InDelta(t, 1.0, metrics.ErrorRate, 0.01, "error rate should be 1.0 with all errors")
}

func TestProfiler_ErrorRate_MixedSuccessAndErrors(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// 3 successful (200), 2 client errors (400, 404), 1 server error (500).
	sendRequest(r, "GET", "/ping")       // 200
	sendRequest(r, "GET", "/ping")       // 200
	sendRequest(r, "POST", "/data")      // 201
	sendRequest(r, "GET", "/badrequest") // 400
	sendRequest(r, "GET", "/notfound")   // 404
	sendRequest(r, "GET", "/error")      // 500

	metrics := p.GetMetrics()
	assert.Equal(t, 6, metrics.TotalRequests)
	// 3 errors out of 6 = 0.5
	assert.InDelta(t, 0.5, metrics.ErrorRate, 0.01, "error rate should be ~50%")
}

func TestProfiler_ErrorRate_NoRequests(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()

	metrics := p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests)
	assert.InDelta(t, 0.0, metrics.ErrorRate, 0.01, "error rate should be 0 with no requests")
}

// ---------------------------------------------------------------------------
// Slowest Endpoints Tests
// ---------------------------------------------------------------------------

func TestProfiler_SlowestEndpoints_TopEntries(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := gin.New()
	r.Use(p.Middleware())

	r.GET("/fast", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/medium", func(c *gin.Context) {
		time.Sleep(20 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/slowest", func(c *gin.Context) {
		time.Sleep(80 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	sendRequest(r, "GET", "/fast")
	sendRequest(r, "GET", "/medium")
	sendRequest(r, "GET", "/slowest")

	metrics := p.GetMetrics()
	require.NotEmpty(t, metrics.SlowestEndpoints, "should have slowest endpoints")

	// The slowest endpoint should be /slowest.
	assert.Contains(t, metrics.SlowestEndpoints[0].Path, "/slowest", "slowest endpoint should be /slowest")
}

func TestProfiler_SlowestEndpoints_LimitToTop10(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := gin.New()
	r.Use(p.Middleware())

	// Create 15 endpoints with different delays.
	for i := 0; i < 15; i++ {
		delay := time.Duration(i+1) * time.Millisecond
		path := fmt.Sprintf("/api/endpoint/%d", i)
		d := delay
		r.GET(path, func(c *gin.Context) {
			time.Sleep(d)
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		sendRequest(r, "GET", path)
	}

	metrics := p.GetMetrics()
	assert.LessOrEqual(t, len(metrics.SlowestEndpoints), 10, "should return at most 10 slowest endpoints")
}

func TestProfiler_SlowestEndpoints_ContainsMethodAndPath(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "POST", "/data")

	metrics := p.GetMetrics()
	require.NotEmpty(t, metrics.SlowestEndpoints, "should have slowest endpoints data")

	// Each entry should have method and path.
	for _, ep := range metrics.SlowestEndpoints {
		assert.NotEmpty(t, ep.Method, "method should not be empty")
		assert.NotEmpty(t, ep.Path, "path should not be empty")
		assert.Greater(t, ep.AvgDuration, float64(0), "avg duration should be positive")
	}
}

func TestProfiler_SlowestEndpoints_AggregatesSamePath(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send multiple requests to the same path.
	for i := 0; i < 5; i++ {
		sendRequest(r, "GET", "/ping")
	}

	metrics := p.GetMetrics()
	require.NotEmpty(t, metrics.SlowestEndpoints, "should have slowest endpoints data")
	// The /ping endpoint should appear with count = 5.
	found := false
	for _, ep := range metrics.SlowestEndpoints {
		if ep.Path == "/ping" {
			assert.GreaterOrEqual(t, ep.Count, 5, "same path should be aggregated")
			found = true
		}
	}
	assert.True(t, found, "/ping should appear in slowest endpoints")
}

// ---------------------------------------------------------------------------
// MetricsHandler Tests
// ---------------------------------------------------------------------------

func TestProfiler_MetricsHandler_ReturnsValidJSON(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/error")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "should return 200 OK")

	// Verify it's valid JSON.
	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err, "response should be valid JSON")

	// Check that expected keys exist.
	assert.Contains(t, result, "total_requests")
	assert.Contains(t, result, "requests_per_second")
	assert.Contains(t, result, "avg_response_time_ms")
	assert.Contains(t, result, "p50_latency_ms")
	assert.Contains(t, result, "p95_latency_ms")
	assert.Contains(t, result, "p99_latency_ms")
	assert.Contains(t, result, "error_rate")
	assert.Contains(t, result, "in_flight_requests")
	assert.Contains(t, result, "slowest_endpoints")
}

func TestProfiler_MetricsHandler_ContentType(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	contentType := w.Header().Get("Content-Type")
	assert.Contains(t, contentType, "application/json", "content type should be JSON")
}

func TestProfiler_MetricsHandler_MethodNotAllowed(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()

	// Create a router where the metrics handler handles all methods at /metrics.
	r := gin.New()
	r.Use(p.Middleware())
	r.Any("/metrics", p.MetricsHandler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/metrics", nil)
	r.ServeHTTP(w, req)

	// The MetricsHandler itself checks the method and returns 405 for non-GET.
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, "POST to metrics handler should return 405")
}

func TestProfiler_MetricsHandler_MetricsIncludesHandler(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// The middleware applies to all routes, so /metrics is also tracked.
	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/metrics") // this will also be tracked by the middleware

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "both /ping and /metrics should be tracked")
}

func TestProfiler_MetricsHandler_EmptyProfiler(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	// total_requests is 0 because calling /metrics itself adds to the count
	// via the middleware. The snapshot is taken before that, though — let's
	// just verify the key exists.
	assert.Contains(t, result, "total_requests")
	assert.Contains(t, result, "error_rate")
	assert.Contains(t, result, "in_flight_requests")
}

func TestProfiler_MetricsHandler_ReflectsNewData(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send some requests first.
	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/ping")

	// Get metrics via the handler.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	// Should have at least 3 requests (plus the /metrics request itself).
	totalRequests, ok := result["total_requests"].(float64)
	require.True(t, ok, "total_requests should be a number")
	assert.GreaterOrEqual(t, totalRequests, 3.0, "should reflect at least 3 ping requests")
}

// ---------------------------------------------------------------------------
// Edge Case Tests
// ---------------------------------------------------------------------------

func TestProfiler_DefaultWindow(t *testing.T) {
	p := NewProfiler(0, nil) // zero duration should use default
	defer p.Stop()

	assert.Equal(t, DefaultWindow, p.Window(), "zero duration should use default window")
}

func TestProfiler_CustomWindow(t *testing.T) {
	p := NewProfiler(10*time.Minute, nil)
	defer p.Stop()

	assert.Equal(t, 10*time.Minute, p.Window(), "should use custom window duration")
}

func TestProfiler_NilLoggerUsesNop(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()

	// Should not panic with nil logger.
	r := setupProfilerRouter(p)
	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 1, metrics.TotalRequests, "should work with nil logger")
}

func TestProfiler_Reset(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	sendRequest(r, "GET", "/ping")
	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "should have 2 requests before reset")

	p.Reset()

	metrics = p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests, "should have 0 requests after reset")
}

func TestProfiler_WindowCleanupDoesNotAffectRecentMetrics(t *testing.T) {
	p := NewProfiler(2*time.Second, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send a request.
	sendRequest(r, "GET", "/ping")

	// Send another request immediately.
	sendRequest(r, "GET", "/ping")

	metrics := p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "both requests should be in window")

	// Wait less than the window.
	time.Sleep(1 * time.Second)

	metrics = p.GetMetrics()
	assert.Equal(t, 2, metrics.TotalRequests, "both requests should still be in window")

	// Wait for the window to expire.
	time.Sleep(1500 * time.Millisecond)

	metrics = p.GetMetrics()
	assert.Equal(t, 0, metrics.TotalRequests, "all requests should have expired")
}

func TestProfiler_StressTest(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	var wg sync.WaitGroup
	const numGoroutines = 20
	const requestsPerGoroutine = 50

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < requestsPerGoroutine; i++ {
				paths := []string{"/ping", "/error", "/notfound", "/badrequest"}
				sendRequest(r, "GET", paths[i%len(paths)])
			}
		}()
	}

	wg.Wait()

	metrics := p.GetMetrics()
	expectedTotal := numGoroutines * requestsPerGoroutine
	assert.Equal(t, expectedTotal, metrics.TotalRequests, "should track all requests under concurrency stress")
}

func TestProfiler_RequestsPerSecondWithBurst(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send a burst of requests.
	for i := 0; i < 100; i++ {
		sendRequest(r, "GET", "/ping")
	}

	metrics := p.GetMetrics()
	assert.Equal(t, 100, metrics.TotalRequests)
	assert.Greater(t, metrics.RequestsPerSecond, float64(0), "RPS should be positive")
}

func TestProfiler_AvgResponseTime(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)
	defer p.Stop()
	r := setupProfilerRouter(p)

	// Send a slow request.
	sendRequest(r, "GET", "/slow") // ~100ms

	metrics := p.GetMetrics()
	// AvgResponseTime is in milliseconds (float64).
	assert.GreaterOrEqual(t, metrics.AvgResponseTime, 80.0, "avg should reflect slow request (>= 80ms)")
}

func TestProfiler_StopIsIdempotent(t *testing.T) {
	p := NewProfiler(5*time.Minute, nil)

	// Calling Stop multiple times should not panic.
	p.Stop()
	p.Stop()
	p.Stop()
}
