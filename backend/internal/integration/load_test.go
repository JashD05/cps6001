package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------
//
// All configuration is via environment variables so the test suite can target
// any running instance without code changes:
//
//	CHAOS_SEC_API_URL         – API base URL            (default: http://localhost:8080/api/v1)
//	CHAOS_SEC_AUTH_EMAIL      – Login email             (default: admin@chaos-sec.io)
//	CHAOS_SEC_AUTH_PASSWORD   – Login password          (default: secureP@ssw0rd!)
//	CHAOS_SEC_CLUSTER_ID     – UUID of a registered cluster  (default: 00000000-0000-0000-0000-000000000001)
//	CHAOS_SEC_EXPERIMENT_ID  – UUID of a completed experiment  (default: 00000000-0000-0000-0000-000000000002)

var (
	apiBaseURL      = getEnv("CHAOS_SEC_API_URL", "http://localhost:8080/api/v1")
	authEmail       = getEnv("CHAOS_SEC_AUTH_EMAIL", "admin@chaos-sec.io")
	authPassword    = getEnv("CHAOS_SEC_AUTH_PASSWORD", "secureP@ssw0rd!")
	clusterIDEnv    = getEnv("CHAOS_SEC_CLUSTER_ID", "00000000-0000-0000-0000-000000000001")
	experimentIDEnv = getEnv("CHAOS_SEC_EXPERIMENT_ID", "00000000-0000-0000-0000-000000000002")
)

// httpClient is shared across all load test requests with a generous timeout.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// getEnv returns the value of the environment variable named by key, or the
// provided fallback if the variable is not set.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------------
// LoadTestResult
// ---------------------------------------------------------------------------

// LoadTestResult holds aggregated metrics from a load test run.
type LoadTestResult struct {
	Endpoint        string
	Method          string
	TotalRequests   int
	SuccessRequests int
	FailedRequests  int
	ErrorRate       float64
	ResponseTimes   []time.Duration
	AvgResponseTime time.Duration
	P95ResponseTime time.Duration
	MinResponseTime time.Duration
	MaxResponseTime time.Duration
	TotalDuration   time.Duration
	RequestsPerSec  float64
}

// requestResult holds per-request timing and outcome.
type requestResult struct {
	Duration time.Duration
	Success  bool
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// authenticate logs in via the API and returns the access token.
func authenticate(t *testing.T) string {
	t.Helper()

	payload, _ := json.Marshal(map[string]string{
		"email":    authEmail,
		"password": authPassword,
	})

	resp, err := httpClient.Post(apiBaseURL+"/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Skipf("API server not reachable at %s: %v", apiBaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("authentication endpoint returned status %d – is the API server running and configured?", resp.StatusCode)
	}

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body), "failed to decode auth response")

	token, ok := body["access_token"].(string)
	require.True(t, ok, "auth response missing access_token field")

	return token
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// makeRequest performs a single HTTP request with auth, measures its latency,
// and returns whether the server responded with a 2xx status.
func makeRequest(method, url, token string, body []byte) requestResult {
	start := time.Now()

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return requestResult{Duration: time.Since(start), Success: false}
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return requestResult{Duration: elapsed, Success: false}
	}

	// Drain and close the body so the connection can be reused.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return requestResult{
		Duration: elapsed,
		Success:  resp.StatusCode >= 200 && resp.StatusCode < 300,
	}
}

// ---------------------------------------------------------------------------
// Concurrency & metrics
// ---------------------------------------------------------------------------

// runConcurrentLoad fans out n goroutines, each invoking fn, and collects the
// results together with the total wall-clock duration.
func runConcurrentLoad(n int, fn func(idx int) requestResult) ([]requestResult, time.Duration) {
	results := make([]requestResult, 0, n)
	var mu sync.Mutex
	var wg sync.WaitGroup

	start := time.Now()
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			rr := fn(idx)
			mu.Lock()
			results = append(results, rr)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	return results, time.Since(start)
}

// calculateResults aggregates raw request results into a LoadTestResult.
func calculateResults(endpoint, method string, results []requestResult, totalDuration time.Duration) LoadTestResult {
	r := LoadTestResult{
		Endpoint:      endpoint,
		Method:        method,
		TotalRequests: len(results),
		TotalDuration: totalDuration,
	}

	for _, rr := range results {
		r.ResponseTimes = append(r.ResponseTimes, rr.Duration)
		if rr.Success {
			r.SuccessRequests++
		} else {
			r.FailedRequests++
		}
	}

	if r.TotalRequests > 0 {
		r.ErrorRate = float64(r.FailedRequests) / float64(r.TotalRequests) * 100.0
	}

	if len(r.ResponseTimes) > 0 {
		sort.Slice(r.ResponseTimes, func(i, j int) bool {
			return r.ResponseTimes[i] < r.ResponseTimes[j]
		})

		r.MinResponseTime = r.ResponseTimes[0]
		r.MaxResponseTime = r.ResponseTimes[len(r.ResponseTimes)-1]

		var sum time.Duration
		for _, d := range r.ResponseTimes {
			sum += d
		}
		r.AvgResponseTime = sum / time.Duration(len(r.ResponseTimes))

		// P95: 95th percentile index (1-based ceiling, then convert to 0-based).
		p95Idx := int(math.Ceil(float64(len(r.ResponseTimes))*0.95)) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		r.P95ResponseTime = r.ResponseTimes[p95Idx]
	}

	if totalDuration.Seconds() > 0 {
		r.RequestsPerSec = float64(r.TotalRequests) / totalDuration.Seconds()
	}

	return r
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

// printLoadTestReport writes a formatted summary of a LoadTestResult to stdout.
func printLoadTestReport(r LoadTestResult) {
	divider := "==============================================================="

	fmt.Println()
	fmt.Println(divider)
	fmt.Printf("  LOAD TEST REPORT : %s\n", r.Endpoint)
	fmt.Println(divider)
	fmt.Printf("  Method             : %s\n", r.Method)
	fmt.Printf("  Total Requests     : %d\n", r.TotalRequests)
	fmt.Printf("  Successful         : %d\n", r.SuccessRequests)
	fmt.Printf("  Failed             : %d\n", r.FailedRequests)
	fmt.Printf("  Error Rate         : %.2f%%\n", r.ErrorRate)
	fmt.Printf("  Avg Response Time  : %v\n", r.AvgResponseTime.Round(time.Microsecond))
	fmt.Printf("  P95 Response Time  : %v\n", r.P95ResponseTime.Round(time.Microsecond))
	fmt.Printf("  Min Response Time  : %v\n", r.MinResponseTime.Round(time.Microsecond))
	fmt.Printf("  Max Response Time  : %v\n", r.MaxResponseTime.Round(time.Microsecond))
	fmt.Printf("  Total Duration     : %v\n", r.TotalDuration.Round(time.Millisecond))
	fmt.Printf("  Requests/sec       : %.2f\n", r.RequestsPerSec)
	fmt.Println(divider)
	fmt.Println()
}

// ---------------------------------------------------------------------------
// Load Tests
// ---------------------------------------------------------------------------

// TestLoad_ExperimentCreation simulates 100 concurrent experiment creation requests.
func TestLoad_ExperimentCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	buildPayload := func(idx int) []byte {
		payload, _ := json.Marshal(map[string]interface{}{
			"name":        fmt.Sprintf("load-test-exp-%d", idx),
			"description": "Load test: concurrent experiment creation",
			"type":        "pod_kill",
			"cluster_id":  clusterIDEnv,
			"namespace":   "chaos-sec-experiments",
			"target_selector": map[string]string{
				"app": "load-test-target",
			},
			"parameters": map[string]interface{}{
				"duration":     "30",
				"force":        false,
				"grace_period": "5",
			},
			"controls": []map[string]interface{}{
				{
					"name":            "SIEM Alert Generation",
					"type":            "alert",
					"expected":        "Alert generated within 60 seconds",
					"timeout_seconds": 120,
				},
			},
			"tags":            []string{"load-test"},
			"timeout_seconds": 300,
		})
		return payload
	}

	results, duration := runConcurrentLoad(100, func(idx int) requestResult {
		return makeRequest(http.MethodPost, apiBaseURL+"/experiments", token, buildPayload(idx))
	})

	report := calculateResults("POST /experiments", "POST", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// TestLoad_ExperimentListing simulates 500 concurrent list requests with pagination.
func TestLoad_ExperimentListing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	results, duration := runConcurrentLoad(500, func(idx int) requestResult {
		page := (idx % 25) + 1
		url := fmt.Sprintf("%s/experiments?page=%d&per_page=20&sort=created_at_desc", apiBaseURL, page)
		return makeRequest(http.MethodGet, url, token, nil)
	})

	report := calculateResults("GET /experiments (paginated)", "GET", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// TestLoad_ClusterHealthChecks simulates 50 concurrent cluster health check requests.
func TestLoad_ClusterHealthChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	results, duration := runConcurrentLoad(50, func(idx int) requestResult {
		url := fmt.Sprintf("%s/clusters/%s/health", apiBaseURL, clusterIDEnv)
		return makeRequest(http.MethodGet, url, token, nil)
	})

	report := calculateResults("GET /clusters/{id}/health", "GET", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// TestLoad_SIEMAlertIngestion simulates 200 concurrent SIEM alert ingestion requests.
func TestLoad_SIEMAlertIngestion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	buildPayload := func(idx int) []byte {
		payload, _ := json.Marshal(map[string]interface{}{
			"alert_id":   fmt.Sprintf("load-test-alert-%d", idx),
			"alert_type": "intrusion_detected",
			"severity":   "high",
			"source":     "load-test-siem",
			"message":    fmt.Sprintf("Load test SIEM alert #%d", idx),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"metadata": map[string]interface{}{
				"experiment_id": experimentIDEnv,
				"cluster_id":    clusterIDEnv,
			},
		})
		return payload
	}

	results, duration := runConcurrentLoad(200, func(idx int) requestResult {
		return makeRequest(http.MethodPost, apiBaseURL+"/siem/alerts", token, buildPayload(idx))
	})

	report := calculateResults("POST /siem/alerts", "POST", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// TestLoad_ReportGeneration simulates 50 concurrent report generation requests.
func TestLoad_ReportGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	results, duration := runConcurrentLoad(50, func(idx int) requestResult {
		url := fmt.Sprintf("%s/experiments/%s/results", apiBaseURL, experimentIDEnv)
		return makeRequest(http.MethodGet, url, token, nil)
	})

	report := calculateResults("GET /experiments/{id}/results", "GET", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// TestLoad_MixedWorkload simulates a realistic mixed workload with ~70% reads
// and ~30% writes across multiple API endpoints.
func TestLoad_MixedWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	token := authenticate(t)

	const totalRequests = 200

	results, duration := runConcurrentLoad(totalRequests, func(idx int) requestResult {
		// 70% reads, 30% writes
		if idx%10 < 7 {
			// Read operations – cycle through different read endpoints
			switch idx % 4 {
			case 0:
				// List experiments with varying pages
				page := (idx%20 + 1)
				url := fmt.Sprintf("%s/experiments?page=%d&per_page=20", apiBaseURL, page)
				return makeRequest(http.MethodGet, url, token, nil)
			case 1:
				// Cluster health
				url := fmt.Sprintf("%s/clusters/%s/health", apiBaseURL, clusterIDEnv)
				return makeRequest(http.MethodGet, url, token, nil)
			case 2:
				// Dashboard summary
				return makeRequest(http.MethodGet, apiBaseURL+"/dashboard/summary", token, nil)
			default:
				// Service health
				return makeRequest(http.MethodGet, apiBaseURL+"/health", token, nil)
			}
		}

		// Write operations
		switch idx % 3 {
		case 0:
			// Create experiment
			payload, _ := json.Marshal(map[string]interface{}{
				"name":       fmt.Sprintf("mixed-load-exp-%d", idx),
				"type":       "pod_kill",
				"cluster_id": clusterIDEnv,
				"namespace":  "chaos-sec-experiments",
				"target_selector": map[string]string{
					"app": "mixed-load-target",
				},
				"parameters": map[string]interface{}{
					"duration": "30",
				},
				"controls": []map[string]interface{}{
					{
						"name":            "SIEM Alert",
						"type":            "alert",
						"expected":        "Alert within 60s",
						"timeout_seconds": 120,
					},
				},
				"tags": []string{"mixed-load-test"},
			})
			return makeRequest(http.MethodPost, apiBaseURL+"/experiments", token, payload)
		case 1:
			// SIEM alert ingestion
			payload, _ := json.Marshal(map[string]interface{}{
				"alert_id":   fmt.Sprintf("mixed-alert-%d", idx),
				"alert_type": "intrusion_detected",
				"severity":   "medium",
				"source":     "mixed-load-siem",
				"message":    fmt.Sprintf("Mixed workload alert #%d", idx),
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
			})
			return makeRequest(http.MethodPost, apiBaseURL+"/siem/alerts", token, payload)
		default:
			// Start experiment
			url := fmt.Sprintf("%s/experiments/%s/start", apiBaseURL, experimentIDEnv)
			return makeRequest(http.MethodPost, url, token, nil)
		}
	})

	report := calculateResults("MIXED (reads 70%% / writes 30%%)", "MIXED", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}
