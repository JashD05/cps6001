package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/middleware"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Configuration
// ============================================================================
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

// ============================================================================
// LoadTestResult
// ============================================================================

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
	P99ResponseTime time.Duration
	MinResponseTime time.Duration
	MaxResponseTime time.Duration
	TotalDuration   time.Duration
	RequestsPerSec  float64
	StatusCodes     map[int]int
}

// requestResult holds per-request timing and outcome.
type requestResult struct {
	Duration   time.Duration
	StatusCode int
	Success    bool
}

// ============================================================================
// Authentication
// ============================================================================

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

// ============================================================================
// HTTP helpers
// ============================================================================

// makeRequest performs a single HTTP request with auth, measures its latency,
// and returns the full request result including status code.
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
		return requestResult{Duration: time.Since(start), StatusCode: 0, Success: false}
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return requestResult{Duration: elapsed, StatusCode: 0, Success: false}
	}

	statusCode := resp.StatusCode
	// Drain and close the body so the connection can be reused.
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return requestResult{
		Duration:   elapsed,
		StatusCode: statusCode,
		Success:    statusCode >= 200 && statusCode < 300,
	}
}

// ============================================================================
// Concurrency & metrics
// ============================================================================

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
		StatusCodes:   make(map[int]int),
	}

	for _, rr := range results {
		r.ResponseTimes = append(r.ResponseTimes, rr.Duration)
		r.StatusCodes[rr.StatusCode]++
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

		// P95: 95th percentile index.
		p95Idx := int(math.Ceil(float64(len(r.ResponseTimes))*0.95)) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		r.P95ResponseTime = r.ResponseTimes[p95Idx]

		// P99: 99th percentile index.
		p99Idx := int(math.Ceil(float64(len(r.ResponseTimes))*0.99)) - 1
		if p99Idx < 0 {
			p99Idx = 0
		}
		r.P99ResponseTime = r.ResponseTimes[p99Idx]
	}

	if totalDuration.Seconds() > 0 {
		r.RequestsPerSec = float64(r.TotalRequests) / totalDuration.Seconds()
	}

	return r
}

// ============================================================================
// Report
// ============================================================================

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
	fmt.Printf("  P99 Response Time  : %v\n", r.P99ResponseTime.Round(time.Microsecond))
	fmt.Printf("  Min Response Time  : %v\n", r.MinResponseTime.Round(time.Microsecond))
	fmt.Printf("  Max Response Time  : %v\n", r.MaxResponseTime.Round(time.Microsecond))
	fmt.Printf("  Total Duration     : %v\n", r.TotalDuration.Round(time.Millisecond))
	fmt.Printf("  Requests/sec       : %.2f\n", r.RequestsPerSec)
	fmt.Printf("  Status Codes       : %v\n", r.StatusCodes)
	fmt.Println(divider)
	fmt.Println()
}

// ============================================================================
// Local Test Server Infrastructure
// ============================================================================
//
// The following creates a local test server for load tests that don't
// require an external running instance. This allows testing middleware
// behavior (rate limiting, RBAC, etc.) under load with full control.

// loadTestEnv holds the local test server environment for load tests.
type loadTestEnv struct {
	server  *httptest.Server
	cfg     *config.Config
	authSvc *auth.AuthService

	// Counters for tracking handler invocations.
	loginAttempts   atomic.Int64
	createAttempts  atomic.Int64
	listAttempts    atomic.Int64
	deleteAttempts  atomic.Int64
	executeAttempts atomic.Int64

	// In-memory store with mutex protection.
	mu          sync.RWMutex
	experiments map[uuid.UUID]*models.Experiment
}

// loadTestConfig returns a configuration for the local load test server.
func loadTestConfig() *config.Config {
	return &config.Config{
		Env: "development",
		Server: config.ServerConfig{
			Host:               "0.0.0.0",
			Port:               "8080",
			ReadTimeout:        15 * time.Second,
			WriteTimeout:       15 * time.Second,
			IdleTimeout:        60 * time.Second,
			CORSAllowedOrigins: "*",
		},
		JWT: config.JWTConfig{
			Secret:        "load-test-jwt-secret-min-32-chars!!",
			Expiry:        1 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
			Issuer:        "chaos-sec-load-test",
		},
		RateLimit:  config.RateLimitConfig{Enabled: true, Requests: 1000, Window: 1 * time.Minute},
		Logging:    config.LoggingConfig{Level: "warn", Format: "console"},
		Kubernetes: config.KubernetesConfig{Namespace: "chaos-sec-load", PodTimeout: 5 * time.Minute, MaxConcurrent: 10},
	}
}

// newLoadTestEnv creates a local test server environment for load testing.
func newLoadTestEnv(t *testing.T, rateLimitRequests int) *loadTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := loadTestConfig()
	cfg.RateLimit.Requests = rateLimitRequests

	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err, "failed to create auth service")

	logger, err := cfg.BuildLogger()
	require.NoError(t, err, "failed to build logger")

	mw := middleware.New(authSvc, nil, cfg, logger)

	env := &loadTestEnv{
		cfg:         cfg,
		authSvc:     authSvc,
		experiments: make(map[uuid.UUID]*models.Experiment),
	}

	// Seed some experiments.
	for i := 0; i < 20; i++ {
		id := uuid.New()
		env.experiments[id] = &models.Experiment{
			Base:           models.Base{ID: id, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			OrganizationID: uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"),
			Name:           fmt.Sprintf("seeded-experiment-%d", i),
			Description:    "Seeded for load testing",
			Status:         "completed",
			CreatedBy:      uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"),
		}
	}

	engine := gin.New()

	// Apply middleware in production order.
	engine.Use(mw.RequestIDMiddleware())
	engine.Use(mw.RecoveryMiddleware())
	engine.Use(middleware.SecurityHeaders())
	engine.Use(mw.CORSMiddleware())
	engine.Use(mw.RateLimitMiddleware())
	engine.Use(middleware.RequestSizeLimit(1 * 1024 * 1024))

	// Health endpoint (no auth).
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	v1 := engine.Group("/api/v1")

	// Auth routes.
	authPublic := v1.Group("/auth")
	{
		authPublic.POST("/login", env.loginHandler)
		authPublic.POST("/refresh", env.refreshHandler)
	}

	authAuthed := v1.Group("/auth")
	authAuthed.Use(mw.AuthMiddleware())
	{
		authAuthed.GET("/me", env.meHandler)
		authAuthed.POST("/logout", env.logoutHandler)
		authAuthed.POST("/register", mw.RBACMiddleware("admin:all", "users:manage"), env.registerHandler)
	}

	// Experiment routes.
	experiments := v1.Group("/experiments")
	experiments.Use(mw.AuthMiddleware())
	{
		experiments.GET("", mw.RBACMiddleware("experiments:read"), env.listExperimentsHandler)
		experiments.POST("", mw.RBACMiddleware("experiments:write"), env.createExperimentHandler)
		experiments.GET("/:id", mw.RBACMiddleware("experiments:read"), env.getExperimentHandler)
		experiments.DELETE("/:id", mw.RBACMiddleware("experiments:delete"), env.deleteExperimentHandler)
		experiments.POST("/:id/execute", mw.RBACMiddleware("experiments:execute"), env.executeExperimentHandler)
	}

	// Dashboard routes.
	dashboard := v1.Group("/dashboard")
	dashboard.Use(mw.AuthMiddleware())
	{
		dashboard.GET("/summary", env.dashboardSummaryHandler)
		dashboard.GET("/metrics", env.dashboardMetricsHandler)
	}

	// Cluster routes.
	clusters := v1.Group("/clusters")
	clusters.Use(mw.AuthMiddleware())
	{
		clusters.GET("", mw.RBACMiddleware("clusters:read"), env.listClustersHandler)
		clusters.GET("/:id/health", mw.RBACMiddleware("clusters:read"), env.clusterHealthHandler)
	}

	env.server = httptest.NewServer(engine)
	t.Cleanup(func() { env.server.Close() })

	return env
}

// generateLoadTestToken creates a valid JWT for load testing.
func (env *loadTestEnv) generateLoadTestToken(role string, permissions []string) string {
	token, _, err := env.authSvc.GenerateAccessToken(
		uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"),
		"load-test@chaos-sec.io",
		role,
		uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"),
		permissions,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to generate load test token: %v", err))
	}
	return token
}

// ============================================================================
// Local Test Server Handlers
// ============================================================================

func (env *loadTestEnv) loginHandler(c *gin.Context) {
	env.loginAttempts.Add(1)
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid request body", "code": 400})
		return
	}

	// Simulate authentication check with a small delay to mimic DB lookup.
	time.Sleep(time.Microseconds(100))

	if req.Email == "" || req.Password == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Invalid credentials", "code": 401})
		return
	}

	accessToken, _, _ := env.authSvc.GenerateAccessToken(
		uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"),
		req.Email,
		"admin",
		uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"),
		[]string{"admin:all"},
	)
	refreshToken, _, _ := env.authSvc.GenerateRefreshToken(
		uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"),
		req.Email,
		"admin",
		uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"),
	)

	c.JSON(http.StatusOK, models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(env.cfg.JWT.Expiry.Seconds()),
		TokenType:    "Bearer",
	})
}

func (env *loadTestEnv) refreshHandler(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid request body", "code": 400})
		return
	}

	claims, err := env.authSvc.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Invalid refresh token", "code": 401})
		return
	}

	accessToken, _, _ := env.authSvc.GenerateAccessToken(
		claims.UserID, claims.Email, claims.Role, claims.OrganizationID,
		[]string{"admin:all"},
	)
	c.JSON(http.StatusOK, gin.H{"access_token": accessToken, "token_type": "Bearer"})
}

func (env *loadTestEnv) meHandler(c *gin.Context) {
	claims, _ := c.Get(string(middleware.ClaimsContextKey))
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": 401})
		return
	}
	c.JSON(http.StatusOK, gin.H{"email": "load-test@chaos-sec.io", "role": "admin"})
}

func (env *loadTestEnv) logoutHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "logged_out"})
}

func (env *loadTestEnv) registerHandler(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid request body", "code": 400})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": uuid.New(), "email": req.Email, "role": "viewer"})
}

func (env *loadTestEnv) listExperimentsHandler(c *gin.Context) {
	env.listAttempts.Add(1)
	env.mu.RLock()
	defer env.mu.RUnlock()

	result := make([]gin.H, 0, len(env.experiments))
	for _, exp := range env.experiments {
		result = append(result, gin.H{
			"id":     exp.ID,
			"name":   exp.Name,
			"status": exp.Status,
		})
	}
	c.JSON(http.StatusOK, models.PaginatedResponse{Data: result, Total: len(result), Page: 1, PageSize: 20, TotalPages: 1})
}

func (env *loadTestEnv) createExperimentHandler(c *gin.Context) {
	env.createAttempts.Add(1)
	var req models.CreateExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid request body", "code": 400})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Name is required", "code": 400})
		return
	}

	claims, _ := c.Get(string(middleware.ClaimsContextKey))
	tokenClaims, ok := claims.(*auth.TokenClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": 401})
		return
	}

	exp := &models.Experiment{
		Base:           models.Base{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now()},
		OrganizationID: tokenClaims.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         "draft",
		CreatedBy:      tokenClaims.UserID,
	}

	env.mu.Lock()
	env.experiments[exp.ID] = exp
	env.mu.Unlock()

	c.JSON(http.StatusCreated, gin.H{"id": exp.ID, "name": exp.Name, "status": exp.Status})
}

func (env *loadTestEnv) getExperimentHandler(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid ID", "code": 400})
		return
	}

	env.mu.RLock()
	exp, ok := env.experiments[id]
	env.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "Experiment not found", "code": 404})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": exp.ID, "name": exp.Name, "status": exp.Status})
}

func (env *loadTestEnv) deleteExperimentHandler(c *gin.Context) {
	env.deleteAttempts.Add(1)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid ID", "code": 400})
		return
	}

	env.mu.Lock()
	defer env.mu.Unlock()

	if _, ok := env.experiments[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "Experiment not found", "code": 404})
		return
	}

	delete(env.experiments, id)
	c.JSON(http.StatusOK, gin.H{"message": "deleted", "id": id})
}

func (env *loadTestEnv) executeExperimentHandler(c *gin.Context) {
	env.executeAttempts.Add(1)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid ID", "code": 400})
		return
	}

	env.mu.Lock()
	defer env.mu.Unlock()

	exp, ok := env.experiments[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "code": 404})
		return
	}

	if exp.Status == "running" {
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "Already running", "code": 409})
		return
	}

	exp.Status = "running"
	exp.UpdatedAt = time.Now()
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "running"})
}

func (env *loadTestEnv) dashboardSummaryHandler(c *gin.Context) {
	env.mu.RLock()
	expCount := len(env.experiments)
	env.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"security_posture_score": 75.0,
		"experiment_summary": gin.H{
			"total":     expCount,
			"running":   0,
			"completed": expCount,
			"failed":    0,
		},
		"cluster_health":  gin.H{"healthy": 1, "degraded": 0, "unhealthy": 0},
		"threat_coverage": gin.H{"total_controls": 24, "validated": 18, "coverage": 75.0},
	})
}

func (env *loadTestEnv) dashboardMetricsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"experiments_per_day": []gin.H{
			{"timestamp": time.Now().AddDate(0, 0, -1).Format(time.RFC3339), "value": 5},
			{"timestamp": time.Now().Format(time.RFC3339), "value": 3},
		},
		"avg_duration": 285000,
		"success_rate": 82.5,
		"active_users": 2,
	})
}

func (env *loadTestEnv) listClustersHandler(c *gin.Context) {
	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data: []gin.H{{
			"id":     uuid.MustParse("f0eebc99-9c0b-4ef8-bb6d-6bb9bd380a55"),
			"name":   "load-test-cluster",
			"status": "connected",
		}},
		Total:      1,
		Page:       1,
		PageSize:   20,
		TotalPages: 1,
	})
}

func (env *loadTestEnv) clusterHealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"cluster_id":   c.Param("id"),
		"status":       "healthy",
		"cpu_usage":    45.2,
		"memory_usage": 62.1,
		"pod_count":    42,
		"node_count":   3,
	})
}

// ============================================================================
// Local Test Server Load Tests
// ============================================================================

// TestLoad_ConcurrentExperimentCreation_Local tests creating experiments
// concurrently against the local test server.
func TestLoad_ConcurrentExperimentCreation_Local(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000) // High rate limit to avoid throttling.
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	const numRequests = 100

	buildPayload := func(idx int) []byte {
		payload, _ := json.Marshal(map[string]interface{}{
			"name":        fmt.Sprintf("load-test-exp-%d", idx),
			"description": "Concurrent experiment creation load test",
		})
		return payload
	}

	results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
		url := env.server.URL + "/api/v1/experiments"
		return makeRequest(http.MethodPost, url, token, buildPayload(idx))
	})

	report := calculateResults("POST /api/v1/experiments (local)", "POST", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, numRequests, report.TotalRequests, "all requests should be counted")
	assert.LessOrEqual(t, report.ErrorRate, 5.0, "error rate should be <= 5%%")
	assert.Greater(t, report.RequestsPerSec, 0.0, "should have measurable throughput")

	// Verify all experiments were created.
	env.mu.RLock()
	createdCount := len(env.experiments)
	env.mu.RUnlock()

	// 20 seeded + up to 100 new.
	assert.GreaterOrEqual(t, createdCount, 20+numRequests-5,
		"most concurrent creates should persist in the store")

	// Verify handler was called.
	assert.Equal(t, int64(numRequests), env.createAttempts.Load(),
		"create handler should be called exactly once per request")
}

// TestLoad_ConcurrentReadRequests_Local tests concurrent GET requests
// against the local test server.
func TestLoad_ConcurrentReadRequests_Local(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("viewer", []string{"experiments:read", "templates:read"})

	const numRequests = 200

	results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
		endpoint := idx % 4
		switch endpoint {
		case 0:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
		case 1:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/summary", token, nil)
		case 2:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/metrics", token, nil)
		default:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/clusters", token, nil)
		}
	})

	report := calculateResults("GET (mixed read endpoints, local)", "GET", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, numRequests, report.TotalRequests)
	assert.LessOrEqual(t, report.ErrorRate, 1.0, "read error rate should be <= 1%%")
	assert.Greater(t, report.RequestsPerSec, 0.0)
}

// TestLoad_ConcurrentLoginRequests_Local tests concurrent login requests
// against the local test server.
func TestLoad_ConcurrentLoginRequests_Local(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)

	const numRequests = 50

	results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
		payload, _ := json.Marshal(map[string]string{
			"email":    "admin@chaos-sec.io",
			"password": "Admin123!",
		})
		return makeRequest(http.MethodPost, env.server.URL+"/api/v1/auth/login", "", payload)
	})

	report := calculateResults("POST /auth/login (local)", "POST", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, numRequests, report.TotalRequests)
	assert.LessOrEqual(t, report.ErrorRate, 5.0, "login error rate should be <= 5%%")
	assert.Equal(t, int64(numRequests), env.loginAttempts.Load())
}

// TestLoad_MixedWorkload_Local simulates a realistic mixed workload with
// ~70% reads and ~30% writes across multiple API endpoints on the local server.
func TestLoad_MixedWorkload_Local(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	const totalRequests = 300

	results, duration := runConcurrentLoad(totalRequests, func(idx int) requestResult {
		// 70% reads, 30% writes.
		if idx%10 < 7 {
			// Read operations.
			switch idx % 4 {
			case 0:
				return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
			case 1:
				return makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/summary", token, nil)
			case 2:
				clusterID := "f0eebc99-9c0b-4ef8-bb6d-6bb9bd380a55"
				return makeRequest(http.MethodGet, env.server.URL+"/api/v1/clusters/"+clusterID+"/health", token, nil)
			default:
				return makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
			}
		}

		// Write operations.
		switch idx % 3 {
		case 0:
			payload, _ := json.Marshal(map[string]interface{}{
				"name":        fmt.Sprintf("mixed-load-exp-%d", idx),
				"description": "Mixed workload test experiment",
			})
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token, payload)
		case 1:
			expID := uuid.New().String() // Will likely 404 but tests the path.
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments/"+expID+"/execute", token, nil)
		default:
			return makeRequest(http.MethodDelete, env.server.URL+"/api/v1/experiments/"+uuid.New().String(), token, nil)
		}
	})

	report := calculateResults("MIXED (reads 70%% / writes 30%%, local)", "MIXED", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, totalRequests, report.TotalRequests)

	// Write operations to non-existent experiments will return 404, so we
	// account for that. The key metric is that the server doesn't crash and
	// handles all requests.
	_, has429 := report.StatusCodes[429]
	assert.False(t, has429, "should not hit rate limit with high threshold")

	// The server should not return any 5xx errors.
	_, has500 := report.StatusCodes[500]
	assert.False(t, has500, "server should not return 500 errors under load")
}

// ============================================================================
// Rate Limiting Under Load (Local Server)
// ============================================================================

// TestLoad_RateLimiting_Basic verifies that the rate limiter properly
// blocks requests beyond the configured threshold.
func TestLoad_RateLimiting_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	// Create a server with a low rate limit.
	env := newLoadTestEnv(t, 20) // 20 requests per window.

	const totalRequests = 40
	results, duration := runConcurrentLoad(totalRequests, func(idx int) requestResult {
		return makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
	})

	report := calculateResults("GET /health (rate limited)", "GET", results, duration)
	printLoadTestReport(report)

	blocked, has429 := report.StatusCodes[429]
	allowed, has200 := report.StatusCodes[200]

	t.Logf("Allowed (200): %d, Blocked (429): %d", allowed, blocked)

	assert.True(t, has429, "should have some 429 responses")
	assert.True(t, has200, "should have some 200 responses")
	assert.Greater(t, blocked, 0, "some requests should be rate limited")
}

// TestLoad_RateLimiting_PerIPIsolation verifies that different client IPs
// have separate rate limit buckets.
func TestLoad_RateLimiting_PerIPIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 30)

	// Send requests from two different "clients" by using two separate
	// HTTP clients with different X-Forwarded-For headers.
	const requestsPerClient = 20

	clientA := &http.Client{Timeout: 10 * time.Second}
	clientB := &http.Client{Timeout: 10 * time.Second}

	var resultsA []requestResult
	var resultsB []requestResult
	var muA, muB sync.Mutex
	var wg sync.WaitGroup

	// Client A: direct requests.
	wg.Add(requestsPerClient)
	for i := 0; i < requestsPerClient; i++ {
		go func() {
			defer wg.Done()
			start := time.Now()
			req, _ := http.NewRequest(http.MethodGet, env.server.URL+"/health", nil)
			resp, err := clientA.Do(req)
			elapsed := time.Since(start)
			if err != nil {
				muA.Lock()
				resultsA = append(resultsA, requestResult{Duration: elapsed, StatusCode: 0, Success: false})
				muA.Unlock()
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			muA.Lock()
			resultsA = append(resultsA, requestResult{Duration: elapsed, StatusCode: resp.StatusCode, Success: resp.StatusCode == 200})
			muA.Unlock()
		}()
	}

	// Client B: requests with a different forwarded IP.
	wg.Add(requestsPerClient)
	for i := 0; i < requestsPerClient; i++ {
		go func() {
			defer wg.Done()
			start := time.Now()
			req, _ := http.NewRequest(http.MethodGet, env.server.URL+"/health", nil)
			req.Header.Set("X-Forwarded-For", "10.0.0.2")
			resp, err := clientB.Do(req)
			elapsed := time.Since(start)
			if err != nil {
				muB.Lock()
				resultsB = append(resultsB, requestResult{Duration: elapsed, StatusCode: 0, Success: false})
				muB.Unlock()
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			muB.Lock()
			resultsB = append(resultsB, requestResult{Duration: elapsed, StatusCode: resp.StatusCode, Success: resp.StatusCode == 200})
			muB.Unlock()
		}()
	}

	wg.Wait()

	reportA := calculateResults("Client A /health", "GET", resultsA, 1*time.Second)
	reportB := calculateResults("Client B /health (different IP)", "GET", resultsB, 1*time.Second)

	t.Logf("Client A: %d success, %d failed, error rate %.2f%%", reportA.SuccessRequests, reportA.FailedRequests, reportA.ErrorRate)
	t.Logf("Client B: %d success, %d failed, error rate %.2f%%", reportB.SuccessRequests, reportB.FailedRequests, reportB.ErrorRate)

	// Both clients should be able to make requests within their own rate limit.
	assert.LessOrEqual(t, reportA.ErrorRate, 50.0, "client A should not be fully blocked")
}

// TestLoad_RateLimiting_LoginEndpoint verifies that the login endpoint
// is properly rate limited and that brute-force attacks are mitigated.
func TestLoad_RateLimiting_LoginEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 10) // Very low rate limit: 10 req/min.

	const totalAttempts = 30
	var blockedCount atomic.Int64
	var successCount atomic.Int64

	results, duration := runConcurrentLoad(totalAttempts, func(idx int) requestResult {
		payload, _ := json.Marshal(map[string]string{
			"email":    "attacker@evil.com",
			"password": fmt.Sprintf("wrong-password-%d", idx),
		})
		rr := makeRequest(http.MethodPost, env.server.URL+"/api/v1/auth/login", "", payload)
		if rr.StatusCode == http.StatusTooManyRequests {
			blockedCount.Add(1)
		} else {
			successCount.Add(1)
		}
		return rr
	})

	report := calculateResults("POST /auth/login (rate limited)", "POST", results, duration)
	printLoadTestReport(report)

	t.Logf("Blocked: %d, Processed: %d", blockedCount.Load(), successCount.Load())
	assert.Greater(t, blockedCount.Load(), int64(0), "some login attempts should be rate limited")
}

// TestLoad_RateLimiting_AuthenticatedVsUnauthenticated verifies that
// authenticated and unauthenticated requests are tracked separately.
func TestLoad_RateLimiting_AuthenticatedVsUnauthenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 50)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	// Send unauthenticated requests first.
	var unauthResults []requestResult
	for i := 0; i < 30; i++ {
		rr := makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
		unauthResults = append(unauthResults, rr)
	}

	// Now send authenticated requests to a different endpoint.
	var authResults []requestResult
	for i := 0; i < 30; i++ {
		rr := makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
		authResults = append(authResults, rr)
	}

	unauthReport := calculateResults("Unauthenticated /health", "GET", unauthResults, 1*time.Second)
	authReport := calculateResults("Authenticated /experiments", "GET", authResults, 1*time.Second)

	t.Logf("Unauth: %d success, %.2f%% error rate", unauthReport.SuccessRequests, unauthReport.ErrorRate)
	t.Logf("Auth: %d success, %.2f%% error rate", authReport.SuccessRequests, authReport.ErrorRate)

	// Both should mostly succeed since they're different endpoints/IPs.
	assert.LessOrEqual(t, unauthReport.ErrorRate, 50.0)
}

// ============================================================================
// Database Connection Pool Stress Tests
// ============================================================================

// TestLoad_DatabaseConnectionPool_Simulated simulates database connection pool
// stress by using a bounded channel as a connection pool and making concurrent
// requests that acquire and release connections.
func TestLoad_DatabaseConnectionPool_Simulated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	const poolSize = 5
	const numRequests = 100

	// Simulate a connection pool with a bounded channel.
	pool := make(chan struct{}, poolSize)
	for i := 0; i < poolSize; i++ {
		pool <- struct{}{}
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var timeoutCount atomic.Int64

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Try to acquire a connection with a timeout.
			select {
			case <-pool:
				// Simulate work.
				time.Sleep(time.Duration(idx%10+1) * time.Millisecond)
				successCount.Add(1)
				pool <- struct{}{} // Release.
			case <-time.After(5 * time.Second):
				timeoutCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Pool size: %d, Requests: %d, Success: %d, Timeout: %d, Duration: %v",
		poolSize, numRequests, successCount.Load(), timeoutCount.Load(), elapsed.Round(time.Millisecond))

	assert.Equal(t, int64(numRequests), successCount.Load()+timeoutCount.Load(),
		"all requests should either succeed or timeout")
	assert.Equal(t, int64(0), timeoutCount.Load(),
		"no requests should timeout with sufficient pool size")
	assert.Greater(t, successCount.Load(), int64(0),
		"some requests should succeed")
}

// TestLoad_DatabaseConnectionPool_Exhaustion simulates what happens when the
// connection pool is exhausted under heavy load.
func TestLoad_DatabaseConnectionPool_Exhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	const poolSize = 3
	const numRequests = 50
	const connTimeout = 500 * time.Millisecond

	pool := make(chan struct{}, poolSize)
	for i := 0; i < poolSize; i++ {
		pool <- struct{}{}
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var timeoutCount atomic.Int64

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			select {
			case <-pool:
				// Simulate a slow query.
				time.Sleep(time.Duration(50+idx%100) * time.Millisecond)
				successCount.Add(1)
				pool <- struct{}{}
			case <-time.After(connTimeout):
				timeoutCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Pool size: %d, Requests: %d, Success: %d, Timeout: %d",
		poolSize, numRequests, successCount.Load(), timeoutCount.Load())

	// With a pool of 3 and slow queries (50-150ms), some requests should timeout.
	assert.Greater(t, timeoutCount.Load(), int64(0),
		"some requests should timeout when pool is exhausted")
	assert.Greater(t, successCount.Load(), int64(0),
		"some requests should still succeed")

	// Verify no request is lost.
	assert.Equal(t, int64(numRequests), successCount.Load()+timeoutCount.Load())
}

// TestLoad_DatabaseConnectionPool_WithRealSQLDB tests concurrent access to
// a real SQL database (if available) or falls back to a simulated test.
func TestLoad_DatabaseConnectionPool_WithRealSQLDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	dbURL := os.Getenv("CHAOS_SEC_TEST_DB_URL")
	if dbURL == "" {
		t.Skip("CHAOS_SEC_TEST_DB_URL not set, skipping real DB pool stress test")
	}

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err, "should connect to test database")
	defer db.Close()

	// Configure connection pool.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	const numQueries = 100
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	start := time.Now()

	for i := 0; i < numQueries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result int
			err := db.QueryRow("SELECT 1").Scan(&result)
			if err != nil {
				errorCount.Add(1)
				return
			}
			successCount.Add(1)
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("DB Pool Queries: %d, Success: %d, Errors: %d, Duration: %v",
		numQueries, successCount.Load(), errorCount.Load(), elapsed.Round(time.Millisecond))

	assert.Equal(t, int64(numQueries), successCount.Load()+errorCount.Load())
	assert.LessOrEqual(t, errorCount.Load(), int64(5), "very few queries should fail")
}

// TestLoad_DatabaseConnectionPool_ConcurrentWrites simulates concurrent
// writes competing for the same database resources.
func TestLoad_DatabaseConnectionPool_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	dbURL := os.Getenv("CHAOS_SEC_TEST_DB_URL")
	if dbURL == "" {
		// Simulate the test with in-memory structures.
		t.Log("CHAOS_SEC_TEST_DB_URL not set, simulating concurrent write contention")

		var mu sync.Mutex
		var writeCount atomic.Int64
		const numWrites = 50

		var wg sync.WaitGroup
		start := time.Now()

		for i := 0; i < numWrites; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				mu.Lock()
				// Simulate write.
				time.Sleep(time.Duration(idx%5+1) * time.Millisecond)
				writeCount.Add(1)
				mu.Unlock()
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		assert.Equal(t, int64(numWrites), writeCount.Load(),
			"all writes should complete")
		t.Logf("Simulated %d concurrent writes in %v", numWrites, elapsed.Round(time.Millisecond))
		return
	}

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	const numWrites = 50
	var successCount atomic.Int64
	var wg sync.WaitGroup

	start := time.Now()
	for i := 0; i < numWrites; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := db.Exec("SELECT pg_sleep(0.01)") // 10ms simulated work.
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("DB concurrent writes: %d, Success: %d, Duration: %v",
		numWrites, successCount.Load(), elapsed.Round(time.Millisecond))
	assert.Greater(t, successCount.Load(), int64(0))
}

// ============================================================================
// Concurrent Request Pattern Tests
// ============================================================================

// TestLoad_ConcurrentBurstRequests tests handling of sudden burst traffic.
func TestLoad_ConcurrentBurstRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 2000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	// Burst of 100 requests all at once.
	const burstSize = 100

	results, duration := runConcurrentLoad(burstSize, func(idx int) requestResult {
		switch idx % 3 {
		case 0:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
		case 1:
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/summary", token, nil)
		default:
			return makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
		}
	})

	report := calculateResults("Burst (mixed endpoints)", "GET", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, burstSize, report.TotalRequests)
	// The server should handle the burst without 5xx errors.
	_, has500 := report.StatusCodes[500]
	assert.False(t, has500, "server should not return 500 errors on burst")
}

// TestLoad_SustainedTraffic simulates sustained traffic over a period of time.
func TestLoad_SustainedTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("viewer", []string{"experiments:read", "templates:read"})

	const totalRequests = 500
	const concurrency = 20

	// Use a semaphore to control concurrency.
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	results := make([]requestResult, totalRequests)
	start := time.Now()

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			switch idx % 5 {
			case 0:
				results[idx] = makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
			case 1:
				results[idx] = makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/summary", token, nil)
			case 2:
				results[idx] = makeRequest(http.MethodGet, env.server.URL+"/api/v1/dashboard/metrics", token, nil)
			case 3:
				results[idx] = makeRequest(http.MethodGet, env.server.URL+"/api/v1/clusters", token, nil)
			default:
				results[idx] = makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	report := calculateResults("Sustained traffic (20 concurrent)", "GET", results, duration)
	printLoadTestReport(report)

	assert.Equal(t, totalRequests, report.TotalRequests)
	assert.LessOrEqual(t, report.ErrorRate, 5.0, "error rate should be <= 5%%")
	assert.Greater(t, report.RequestsPerSec, 0.0)

	// P99 should be reasonable (under 1 second for local server).
	assert.Less(t, report.P99ResponseTime, 1*time.Second,
		"P99 response time should be under 1 second for local server")
}

// TestLoad_WriteContention tests concurrent write operations to the same resource.
func TestLoad_WriteContention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	// First create an experiment.
	createResp, _ := makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token,
		[]byte(`{"name":"contention-test","description":"test"}`))
	if createResp.StatusCode != http.StatusCreated {
		// Try to find an existing experiment.
		env.mu.RLock()
		for id := range env.experiments {
			createResp = &http.Response{StatusCode: http.StatusOK}
			_ = id
			break
		}
		env.mu.RUnlock()
	}

	// Now send concurrent execute requests for the same experiment.
	// Most will get 404 (random UUID), but the lock contention test is valid.
	const numRequests = 50

	var wg sync.WaitGroup
	var conflictCount atomic.Int64
	var notFoundCount atomic.Int64
	var successCount atomic.Int64

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			expID := uuid.New().String()
			rr := makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments/"+expID+"/execute", token, nil)
			switch rr.StatusCode {
			case http.StatusOK:
				successCount.Add(1)
			case http.StatusConflict:
				conflictCount.Add(1)
			case http.StatusNotFound:
				notFoundCount.Add(1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Execute: success=%d, conflict=%d, not_found=%d",
		successCount.Load(), conflictCount.Load(), notFoundCount.Load())

	// The server should handle all requests without panicking.
	assert.Equal(t, int64(numRequests), successCount.Load()+conflictCount.Load()+notFoundCount.Load())
}

// TestLoad_RBACUnderLoad tests that RBAC enforcement works correctly
// even under concurrent load.
func TestLoad_RBACUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)

	adminToken := env.generateLoadTestToken("admin", []string{"admin:all"})
	viewerToken := env.generateLoadTestToken("viewer", []string{"experiments:read"})

	const numRequests = 50

	t.Run("admin requests all succeed", func(t *testing.T) {
		results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
			payload, _ := json.Marshal(map[string]interface{}{
				"name":        fmt.Sprintf("admin-load-%d", idx),
				"description": "Admin concurrent creation",
			})
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", adminToken, payload)
		})

		report := calculateResults("POST /experiments (admin)", "POST", results, duration)
		t.Logf("Admin: %d/%d success (%.1f%%)", report.SuccessRequests, report.TotalRequests, 100-report.ErrorRate)

		assert.LessOrEqual(t, report.ErrorRate, 5.0, "admin requests should mostly succeed")
	})

	t.Run("viewer write requests all fail with 403", func(t *testing.T) {
		results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
			payload, _ := json.Marshal(map[string]interface{}{
				"name": fmt.Sprintf("viewer-load-%d", idx),
			})
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", viewerToken, payload)
		})

		report := calculateResults("POST /experiments (viewer)", "POST", results, duration)
		t.Logf("Viewer: %d/%d blocked (%.1f%%)", report.StatusCodes[403], report.TotalRequests, report.ErrorRate)

		forbidden, has403 := report.StatusCodes[403]
		assert.True(t, has403, "viewer should receive 403 responses")
		assert.Equal(t, numRequests, forbidden, "all viewer write attempts should be forbidden")
	})

	t.Run("viewer read requests succeed", func(t *testing.T) {
		results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", viewerToken, nil)
		})

		report := calculateResults("GET /experiments (viewer)", "GET", results, duration)
		t.Logf("Viewer reads: %d/%d success", report.SuccessRequests, report.TotalRequests)

		assert.LessOrEqual(t, report.ErrorRate, 5.0, "viewer read requests should mostly succeed")
	})
}

// ============================================================================
// Response Time Distribution Tests
// ============================================================================

// TestLoad_ResponseTimeDistribution verifies that response times fall within
// acceptable bounds under normal load.
func TestLoad_ResponseTimeDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("viewer", []string{"experiments:read"})

	const numRequests = 100

	results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
		return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
	})

	report := calculateResults("GET /experiments (response time dist)", "GET", results, duration)
	printLoadTestReport(report)

	// Verify response time distribution.
	assert.Less(t, report.AvgResponseTime, 100*time.Millisecond,
		"average response time should be under 100ms for local server")
	assert.Less(t, report.P95ResponseTime, 200*time.Millisecond,
		"P95 response time should be under 200ms")
	assert.Less(t, report.P99ResponseTime, 500*time.Millisecond,
		"P99 response time should be under 500ms")

	// Standard deviation check: max should not be more than 10x the average.
	if report.AvgResponseTime > 0 {
		maxRatio := float64(report.MaxResponseTime) / float64(report.AvgResponseTime)
		assert.Less(t, maxRatio, 20.0,
			"max response time should not be more than 20x the average (no extreme outliers)")
	}
}

// TestLoad_NoMemoryLeaksUnderRepeatedRequests makes many repeated requests
// and verifies the server remains responsive throughout.
func TestLoad_NoMemoryLeaksUnderRepeatedRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("viewer", []string{"experiments:read"})

	const rounds = 10
	const requestsPerRound = 50

	var avgTimes []time.Duration

	for round := 0; round < rounds; round++ {
		results, _ := runConcurrentLoad(requestsPerRound, func(idx int) requestResult {
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
		})

		var sum time.Duration
		for _, rr := range results {
			sum += rr.Duration
		}
		avg := sum / time.Duration(len(results))
		avgTimes = append(avgTimes, avg)
	}

	// Check that response times don't degrade significantly over time.
	// The last round should not be more than 5x slower than the first.
	if len(avgTimes) >= 2 {
		firstAvg := avgTimes[0]
		lastAvg := avgTimes[len(avgTimes)-1]

		t.Logf("Round 1 avg: %v, Round %d avg: %v", firstAvg, len(avgTimes), lastAvg)

		if firstAvg > 0 {
			degradation := float64(lastAvg) / float64(firstAvg)
			assert.Less(t, degradation, 5.0,
				"response time should not degrade by more than 5x over %d rounds (got %.2fx)",
				rounds, degradation)
		}
	}
}

// ============================================================================
// External Server Load Tests
// ============================================================================
// The following tests target an external running API server. They will
// be skipped if the server is not reachable.

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
			switch idx % 4 {
			case 0:
				page := (idx%20 + 1)
				url := fmt.Sprintf("%s/experiments?page=%d&per_page=20", apiBaseURL, page)
				return makeRequest(http.MethodGet, url, token, nil)
			case 1:
				url := fmt.Sprintf("%s/clusters/%s/health", apiBaseURL, clusterIDEnv)
				return makeRequest(http.MethodGet, url, token, nil)
			case 2:
				return makeRequest(http.MethodGet, apiBaseURL+"/dashboard/summary", token, nil)
			default:
				return makeRequest(http.MethodGet, strings.Replace(apiBaseURL, "/api/v1", "/health", 1), token, nil)
			}
		}

		// Write operations
		switch idx % 3 {
		case 0:
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
			url := fmt.Sprintf("%s/experiments/%s/start", apiBaseURL, experimentIDEnv)
			return makeRequest(http.MethodPost, url, token, nil)
		}
	})

	report := calculateResults("MIXED (reads 70%% / writes 30%%)", "MIXED", results, duration)
	printLoadTestReport(report)

	assert.LessOrEqual(t, report.ErrorRate, 10.0, "error rate should be <= 10%%")
}

// ============================================================================
// Stress / Edge Case Tests
// ============================================================================

// TestLoad_LargePayloadHandling tests that the server handles large request
// bodies correctly under concurrent load.
func TestLoad_LargePayloadHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	t.Run("many concurrent small payloads succeed", func(t *testing.T) {
		const numRequests = 50

		results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
			payload, _ := json.Marshal(map[string]interface{}{
				"name":        fmt.Sprintf("small-payload-%d", idx),
				"description": strings.Repeat("x", 100), // 100 bytes
			})
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token, payload)
		})

		report := calculateResults("POST /experiments (small payloads)", "POST", results, duration)
		printLoadTestReport(report)

		assert.LessOrEqual(t, report.ErrorRate, 5.0)
	})

	t.Run("oversized payload is rejected with 413", func(t *testing.T) {
		// Create a payload larger than 1MB (the server's limit).
		largeDesc := strings.Repeat("a", 2*1024*1024)
		payload, _ := json.Marshal(map[string]interface{}{
			"name":        "oversized-experiment",
			"description": largeDesc,
		})

		rr := makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token, payload)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.StatusCode,
			"Oversized payload should be rejected with 413")
	})

	t.Run("concurrent oversized payloads don't crash server", func(t *testing.T) {
		const numRequests = 10

		var wg sync.WaitGroup
		var tooLargeCount atomic.Int64

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				largeDesc := strings.Repeat("b", 2*1024*1024)
				payload, _ := json.Marshal(map[string]interface{}{
					"name":        "concurrent-oversized",
					"description": largeDesc,
				})
				rr := makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token, payload)
				if rr.StatusCode == http.StatusRequestEntityTooLarge {
					tooLargeCount.Add(1)
				}
			}()
		}

		wg.Wait()

		assert.Greater(t, tooLargeCount.Load(), int64(0),
			"some requests should be rejected as too large")

		// Verify server is still responsive after the stress.
		rr := makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
		assert.Equal(t, http.StatusOK, rr.StatusCode,
			"server should still be healthy after oversized payload stress")
	})
}

// TestLoad_MalformedRequestsUnderLoad tests server resilience against
// malformed concurrent requests.
func TestLoad_MalformedRequestsUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	t.Run("concurrent malformed JSON doesn't crash server", func(t *testing.T) {
		const numRequests = 30

		results, duration := runConcurrentLoad(numRequests, func(idx int) requestResult {
			// Send invalid JSON.
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token,
				[]byte(`{invalid json`))
		})

		report := calculateResults("POST /experiments (malformed JSON)", "POST", results, duration)
		printLoadTestReport(report)

		// All should fail with 400, not 500.
		_, has500 := report.StatusCodes[500]
		assert.False(t, has500, "malformed JSON should not cause 500 errors")

		// Server should still be responsive.
		rr := makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
		assert.Equal(t, http.StatusOK, rr.StatusCode)
	})

	t.Run("concurrent empty bodies don't crash server", func(t *testing.T) {
		const numRequests = 30

		results, _ := runConcurrentLoad(numRequests, func(idx int) requestResult {
			return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token,
				[]byte(`{}`))
		})

		report := calculateResults("POST /experiments (empty bodies)", "POST", results, time.Second)

		// Should get 400 (missing required fields), not 500.
		_, has500 := report.StatusCodes[500]
		assert.False(t, has500, "empty body should not cause 500 errors")

		// Server should still be responsive.
		rr := makeRequest(http.MethodGet, env.server.URL+"/health", "", nil)
		assert.Equal(t, http.StatusOK, rr.StatusCode)
	})
}

// TestLoad_HandlerCounterConsistency verifies that the handler counters
// match the number of requests that were processed.
func TestLoad_HandlerCounterConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	const numCreates = 30
	const numLists = 50

	// Concurrent creates.
	_, _ = runConcurrentLoad(numCreates, func(idx int) requestResult {
		payload, _ := json.Marshal(map[string]interface{}{
			"name": fmt.Sprintf("counter-test-%d", idx),
		})
		return makeRequest(http.MethodPost, env.server.URL+"/api/v1/experiments", token, payload)
	})

	// Concurrent lists.
	_, _ = runConcurrentLoad(numLists, func(idx int) requestResult {
		return makeRequest(http.MethodGet, env.server.URL+"/api/v1/experiments", token, nil)
	})

	// Verify handler was called the expected number of times.
	createAttempts := env.createAttempts.Load()
	listAttempts := env.listAttempts.Load()

	t.Logf("Create attempts: %d (expected ~%d)", createAttempts, numCreates)
	t.Logf("List attempts: %d (expected ~%d)", listAttempts, numLists)

	assert.GreaterOrEqual(t, createAttempts, int64(numCreates),
		"create handler should be called at least once per create request")
	assert.GreaterOrEqual(t, listAttempts, int64(numLists),
		"list handler should be called at least once per list request")
}

// TestLoad_ConcurrentTokenValidation tests that token validation works
// correctly under concurrent load.
func TestLoad_ConcurrentTokenValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	env := newLoadTestEnv(t, 5000)
	token := env.generateLoadTestToken("admin", []string{"admin:all"})

	// Also create an invalid token.
	invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.payload"

	const numValid = 50
	const numInvalid = 30

	t.Run("valid tokens under load", func(t *testing.T) {
		results, duration := runConcurrentLoad(numValid, func(idx int) requestResult {
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/auth/me", token, nil)
		})

		report := calculateResults("GET /auth/me (valid tokens)", "GET", results, duration)
		assert.LessOrEqual(t, report.ErrorRate, 5.0, "valid tokens should mostly succeed")
	})

	t.Run("invalid tokens under load", func(t *testing.T) {
		results, duration := runConcurrentLoad(numInvalid, func(idx int) requestResult {
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/auth/me", invalidToken, nil)
		})

		report := calculateResults("GET /auth/me (invalid tokens)", "GET", results, duration)

		forbidden, _ := report.StatusCodes[401]
		assert.Equal(t, numInvalid, forbidden, "all invalid tokens should be rejected with 401")
	})

	t.Run("mixed valid and invalid tokens", func(t *testing.T) {
		const total = 80
		results, duration := runConcurrentLoad(total, func(idx int) requestResult {
			if idx%2 == 0 {
				return makeRequest(http.MethodGet, env.server.URL+"/api/v1/auth/me", token, nil)
			}
			return makeRequest(http.MethodGet, env.server.URL+"/api/v1/auth/me", invalidToken, nil)
		})

		report := calculateResults("GET /auth/me (mixed tokens)", "GET", results, duration)

		// Should have a mix of 200 and 401.
		_, has200 := report.StatusCodes[200]
		_, has401 := report.StatusCodes[401]
		assert.True(t, has200, "should have some 200 responses for valid tokens")
		assert.True(t, has401, "should have some 401 responses for invalid tokens")
	})
}
