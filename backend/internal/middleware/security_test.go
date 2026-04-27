package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// findZapField searches for a zapcore.Field with the given key in a slice.
func findZapField(fields []zapcore.Field, key string) (zapcore.Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}
	return zapcore.Field{}, false
}

// newObservedLogger creates a zap logger backed by the observer core for
// capturing structured log output in tests.
func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	return logger, recorded
}

// setupRouter creates a Gin router with the given middleware and standard
// GET /ping and POST /upload handlers.
func setupRouter(mw ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mw...)
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	r.POST("/upload", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "uploaded"})
	})
	r.POST("/echo", func(c *gin.Context) {
		// Reads the body to verify it is still available after sanitization.
		body := make([]byte, 4096)
		n, _ := c.Request.Body.Read(body)
		c.Data(http.StatusOK, "text/plain", body[:n])
	})
	return r
}

// ---------------------------------------------------------------------------
// SecurityHeaders Tests
// ---------------------------------------------------------------------------

func TestSecurityHeaders_SetsAllHeaders(t *testing.T) {
	router := setupRouter(SecurityHeaders())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "0",
		"Content-Security-Policy":   "default-src 'self'",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains; preload",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		assert.Equal(t, expected, got, "header %q mismatch", header)
	}
}

func TestSecurityHeaders_AllHeaderKeysPresent(t *testing.T) {
	router := setupRouter(SecurityHeaders())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requiredKeys := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Content-Security-Policy",
		"Referrer-Policy",
		"Permissions-Policy",
		"Strict-Transport-Security",
	}

	for _, key := range requiredKeys {
		assert.NotEmpty(t, w.Header().Get(key), "expected security header %q to be set", key)
	}
}

func TestSecurityHeaders_GeneratesRequestIDIfMissing(t *testing.T) {
	router := setupRouter(SecurityHeaders())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID, "expected X-Request-ID to be generated")
	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	assert.Contains(t, requestID, "-", "expected UUID format for X-Request-ID")
}

func TestSecurityHeaders_PreservesExistingRequestID(t *testing.T) {
	router := setupRouter(SecurityHeaders())

	existingID := "custom-request-id-12345"
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	got := w.Header().Get("X-Request-ID")
	assert.Equal(t, existingID, got, "expected existing X-Request-ID to be preserved")
}

func TestSecurityHeaders_DoesNotBlockRequest(t *testing.T) {
	router := setupRouter(SecurityHeaders())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSecurityHeaders_AppliesToAllHTTPMethods(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders())
	router.Any("/api/:action", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"), "%s request missing header", method)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestSecurityHeaders_HeadersSetOnErrorResponse(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/fail", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.NotEmpty(t, w.Header().Get("Strict-Transport-Security"))
}

// ---------------------------------------------------------------------------
// RateLimit Tests (in-memory, sliding window)
// ---------------------------------------------------------------------------

func TestRateLimit_AllowsRequestsUnderLimit(t *testing.T) {
	router := setupRouter(RateLimit(5, time.Minute))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "request %d should be allowed", i+1)
	}
}

func TestRateLimit_BlocksRequestsOverLimit(t *testing.T) {
	router := setupRouter(RateLimit(3, time.Minute))

	// Exhaust the limit.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.1.2:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Next request should be blocked.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.2:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimit_SetsRetryAfterHeader(t *testing.T) {
	router := setupRouter(RateLimit(2, 10*time.Second))

	// Exhaust the limit.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "192.168.1.3:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// Next request should be blocked with Retry-After.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.3:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	retryAfter := w.Header().Get("Retry-After")
	assert.NotEmpty(t, retryAfter, "expected Retry-After header on 429 response")
}

func TestRateLimit_SetsRateLimitHeaders(t *testing.T) {
	router := setupRouter(RateLimit(10, time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.4:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	remaining := w.Header().Get("X-RateLimit-Remaining")
	assert.NotEmpty(t, remaining, "expected X-RateLimit-Remaining header")
}

func TestRateLimit_429ResponseBody(t *testing.T) {
	router := setupRouter(RateLimit(1, time.Minute))

	// First request passes.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.5:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Second request is blocked.
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.5:1234"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "failed to parse 429 response body")
	assert.Equal(t, "rate_limit_exceeded", resp["error"])
}

func TestRateLimit_DifferentIPsTrackedSeparately(t *testing.T) {
	router := setupRouter(RateLimit(1, time.Minute))

	ips := []string{"10.0.0.1:1111", "10.0.0.2:2222", "10.0.0.3:3333"}
	for _, ip := range ips {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "IP %s should be allowed (separate tracking)", ip)
	}
}

func TestRateLimit_DefaultMaxRequestsWhenZero(t *testing.T) {
	router := setupRouter(RateLimit(0, time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.6:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Default of 100 should allow this request.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "100", w.Header().Get("X-RateLimit-Limit"))
}

func TestRateLimit_DefaultMaxRequestsWhenNegative(t *testing.T) {
	router := setupRouter(RateLimit(-5, time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.7:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "100", w.Header().Get("X-RateLimit-Limit"))
}

func TestRateLimit_DefaultWindowWhenZero(t *testing.T) {
	router := setupRouter(RateLimit(50, 0))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.8:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_DefaultWindowWhenNegative(t *testing.T) {
	router := setupRouter(RateLimit(50, -time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.9:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRateLimit_SlidingWindowExpiration(t *testing.T) {
	// Use a very short window to test that expired entries are pruned.
	router := setupRouter(RateLimit(2, 200*time.Millisecond))

	ip := "192.168.1.10:1234"

	// Send 2 requests to exhaust the limit.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Immediate request should be blocked.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Wait for the window to expire.
	time.Sleep(300 * time.Millisecond)

	// Request should now be allowed again.
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = ip
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "request should be allowed after window expires")
}

// ---------------------------------------------------------------------------
// CORS Tests
// ---------------------------------------------------------------------------

func TestCORS_AllowedOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com", "https://admin.example.com"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_BlocksDisallowedOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), "disallowed origin should not get Allow-Origin header")
}

func TestCORS_PreflightOptions(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := gin.New()
	router.Use(CORS(allowed))
	router.OPTIONS("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, "preflight should return 204")
	assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_WildcardAllowsAll(t *testing.T) {
	allowed := []string{"*"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "https://anything.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_WildcardNoOriginHeader(t *testing.T) {
	allowed := []string{"*"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_WildcardPatternSubdomain(t *testing.T) {
	allowed := []string{"https://*.example.com"}
	router := setupRouter(CORS(allowed))

	tests := []struct {
		origin      string
		shouldAllow bool
	}{
		{"https://app.example.com", true},
		{"https://admin.example.com", true},
		{"https://api.example.com", true},
		{"https://evil.other.com", false},
		{"http://app.example.com", false}, // wrong scheme
		{"https://example.com", false},    // no subdomain
	}

	for _, tc := range tests {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			got := w.Header().Get("Access-Control-Allow-Origin")
			if tc.shouldAllow {
				assert.Equal(t, tc.origin, got)
			} else {
				assert.Empty(t, got, "expected no Allow-Origin for %q", tc.origin)
			}
		})
	}
}

func TestCORS_EmptyAllowedOrigins(t *testing.T) {
	router := setupRouter(CORS([]string{}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_SetsAllowCredentials(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_SetsMaxAge(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
}

func TestCORS_SetsVaryHeader(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "Origin", w.Header().Get("Vary"))
}

func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	allowed := []string{"https://app.example.com"}
	router := gin.New()
	router.Use(CORS(allowed))
	router.OPTIONS("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_WhitespaceInOrigins(t *testing.T) {
	allowed := []string{" https://app.example.com ", "https://admin.example.com "}
	router := setupRouter(CORS(allowed))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// RequestSizeLimit Tests
// ---------------------------------------------------------------------------

func TestRequestSizeLimit_AllowsNormalRequests(t *testing.T) {
	maxBytes := int64(1024)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader("hello world")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = int64(len("hello world"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestSizeLimit_RejectsOversizedRequests(t *testing.T) {
	maxBytes := int64(10)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader("this is a much longer body that exceeds the limit")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = int64(len("this is a much longer body that exceeds the limit"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestRequestSizeLimit_ErrorMessageContainsMaxSize(t *testing.T) {
	maxBytes := int64(2048)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader(strings.Repeat("x", 3000))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = 3000
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err, "failed to parse response")
	msg, ok := resp["message"].(string)
	require.True(t, ok, "expected message to be string")
	assert.Contains(t, msg, "2048")
}

func TestRequestSizeLimit_ExactlyAtLimit(t *testing.T) {
	maxBytes := int64(10)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader("0123456789") // exactly 10 bytes
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = 10
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestSizeLimit_OneByteOverLimit(t *testing.T) {
	maxBytes := int64(10)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader("0123456789A") // 11 bytes
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = 11
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestRequestSizeLimit_EmptyBody(t *testing.T) {
	maxBytes := int64(1024)
	router := setupRouter(RequestSizeLimit(maxBytes))

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.ContentLength = 0
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestSizeLimit_GetRequest(t *testing.T) {
	maxBytes := int64(10)
	router := setupRouter(RequestSizeLimit(maxBytes))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestSizeLimit_Default10MB(t *testing.T) {
	// Verify that the default 10MB limit allows reasonable payloads.
	defaultLimit := int64(10 * 1024 * 1024)
	router := setupRouter(RequestSizeLimit(defaultLimit))

	body := strings.NewReader(strings.Repeat("x", 1024)) // 1KB
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = 1024
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequestSizeLimit_ResponseBodyOnError(t *testing.T) {
	maxBytes := int64(5)
	router := setupRouter(RequestSizeLimit(maxBytes))

	body := strings.NewReader("too long body")
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.ContentLength = int64(len("too long body"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "request_too_large", resp["error"])
}

// ---------------------------------------------------------------------------
// AuditLog Tests
// ---------------------------------------------------------------------------

func TestAuditLog_LogsRequestDetails(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected at least one log entry")

	entry := observed.All()[0]
	assert.Equal(t, "http_request", entry.Message)
	assert.Equal(t, zapcore.InfoLevel, entry.Level)

	// Verify key structured fields are present.
	fields := entry.Context
	fieldKeys := make(map[string]bool)
	for _, f := range fields {
		fieldKeys[f.Key] = true
	}

	assert.True(t, fieldKeys["method"], "expected 'method' field")
	assert.True(t, fieldKeys["path"], "expected 'path' field")
	assert.True(t, fieldKeys["status"], "expected 'status' field")
	assert.True(t, fieldKeys["duration"], "expected 'duration' field")
	assert.True(t, fieldKeys["client_ip"], "expected 'client_ip' field")
	assert.True(t, fieldKeys["user_agent"], "expected 'user_agent' field")
}

func TestAuditLog_RecordsMethod(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len())
	field, ok := findZapField(observed.All()[0].Context, "method")
	require.True(t, ok)
	assert.Equal(t, "GET", field.String)
}

func TestAuditLog_RecordsPath(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "path")
	require.True(t, ok)
	assert.Equal(t, "/ping", field.String)
}

func TestAuditLog_RecordsStatusCode(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "status")
	require.True(t, ok)
	assert.Equal(t, int64(http.StatusOK), field.Integer)
}

func TestAuditLog_RecordsDuration(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	_, ok := findZapField(observed.All()[0].Context, "duration")
	assert.True(t, ok, "expected 'duration' field")
}

func TestAuditLog_RecordsClientIP(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "client_ip")
	require.True(t, ok)
	assert.NotEmpty(t, field.String)
}

func TestAuditLog_RecordsUserAgent(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("User-Agent", "TestBot/1.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "user_agent")
	require.True(t, ok)
	assert.Equal(t, "TestBot/1.0", field.String)
}

func TestAuditLog_GeneratesRequestID(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check response header.
	requestID := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID, "AuditLog should generate X-Request-ID if missing")

	// Check log field.
	field, ok := findZapField(observed.All()[0].Context, "request_id")
	require.True(t, ok)
	assert.NotEmpty(t, field.String)
}

func TestAuditLog_ErrorsFor5xxResponses(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(AuditLog(logger))
	router.GET("/boom", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	foundError := false
	for _, entry := range observed.All() {
		if entry.Level == zapcore.ErrorLevel && entry.Message == "server_error" {
			foundError = true
		}
	}
	assert.True(t, foundError, "expected 'server_error' error log for 5xx response")
}

func TestAuditLog_NoErrorFor4xxResponses(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(AuditLog(logger))
	router.GET("/notfound", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	for _, entry := range observed.All() {
		if entry.Level == zapcore.ErrorLevel && entry.Message == "server_error" {
			t.Error("did not expect 'server_error' error log for 4xx response")
		}
	}
}

func TestAuditLog_MultipleRequests(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(AuditLog(logger))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	assert.GreaterOrEqual(t, observed.Len(), 5, "expected at least 5 log entries for 5 requests")
}

// ---------------------------------------------------------------------------
// SanitizeInput Tests
// ---------------------------------------------------------------------------

func TestSanitizeInput_TrimsWhitespaceFromQueryParams(t *testing.T) {
	router := gin.New()
	router.Use(SanitizeInput())
	router.GET("/search", func(c *gin.Context) {
		q := c.Query("q")
		c.String(http.StatusOK, "q=%s", q)
	})

	// Build URL with proper encoding: spaces are encoded as %20, which
	// decode to "  hello world  " — the middleware should trim these.
	u := &url.URL{Path: "/search", RawQuery: "q=%20%20hello%20world%20%20"}
	req := httptest.NewRequest(http.MethodGet, u.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "hello world")
	assert.NotContains(t, body, "  hello world  ")
}

func TestSanitizeInput_TrimsMultipleQueryParams(t *testing.T) {
	router := gin.New()
	router.Use(SanitizeInput())
	router.GET("/search", func(c *gin.Context) {
		name := c.Query("name")
		tag := c.Query("tag")
		c.String(http.StatusOK, "name=%s,tag=%s", name, tag)
	})

	// Build URL with properly encoded leading/trailing spaces.
	u := &url.URL{Path: "/search", RawQuery: "name=%20foo%20&tag=%20bar%20"}
	req := httptest.NewRequest(http.MethodGet, u.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "name=foo")
	assert.Contains(t, body, "tag=bar")
}

func TestSanitizeInput_RejectsNullBytesInQueryParams(t *testing.T) {
	router := setupRouter(SanitizeInput())

	// URL with null byte in query param (URL-encoded as %00).
	req := httptest.NewRequest(http.MethodGet, "/ping?q=hello%00world", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_input", resp["error"])
}

func TestSanitizeInput_RejectsNullBytesInBody(t *testing.T) {
	router := setupRouter(SanitizeInput())

	body := bytes.NewReader([]byte("hello\x00world"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_input", resp["error"])
}

func TestSanitizeInput_AllowsCleanBody(t *testing.T) {
	router := setupRouter(SanitizeInput())

	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(`{"name":"test"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSanitizeInput_AllowsCleanQueryParams(t *testing.T) {
	router := setupRouter(SanitizeInput())

	req := httptest.NewRequest(http.MethodGet, "/ping?q=hello&sort=asc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSanitizeInput_PreservesBodyForDownstreamHandlers(t *testing.T) {
	router := gin.New()
	router.Use(SanitizeInput())
	router.POST("/echo", func(c *gin.Context) {
		body := make([]byte, 4096)
		n, _ := c.Request.Body.Read(body)
		c.Data(http.StatusOK, "text/plain", body[:n])
	})

	payload := "clean body data"
	body := strings.NewReader(payload)
	req := httptest.NewRequest(http.MethodPost, "/echo", body)
	req.ContentLength = int64(len(payload))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, payload, w.Body.String())
}

func TestSanitizeInput_AllowsGetRequestWithNoBody(t *testing.T) {
	router := setupRouter(SanitizeInput())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSanitizeInput_HandlesEmptyQueryParams(t *testing.T) {
	router := setupRouter(SanitizeInput())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// Integration / Combined middleware tests
// ---------------------------------------------------------------------------

func TestSecurityHeadersAndAuditLog_Combined(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := setupRouter(SecurityHeaders(), AuditLog(logger))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	assert.NotZero(t, observed.Len(), "expected audit log entry")
}

func TestSecurityHeadersAndSanitizeInput_Combined(t *testing.T) {
	router := setupRouter(SecurityHeaders(), SanitizeInput())

	u := &url.URL{Path: "/ping", RawQuery: "q=%20%20hello%20%20"}
	req := httptest.NewRequest(http.MethodGet, u.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "0", w.Header().Get("X-XSS-Protection"))
}

func TestRateLimitAndSecurityHeaders_Combined(t *testing.T) {
	router := setupRouter(SecurityHeaders(), RateLimit(5, time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"))
}
