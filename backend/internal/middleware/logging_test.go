package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// ---------------------------------------------------------------------------
// StructuredLogger Tests
// ---------------------------------------------------------------------------

func TestStructuredLogger_LogsAtInfoFor2xx(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected at least one log entry")

	entry := observed.All()[0]
	assert.Equal(t, "request", entry.Message)
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
}

func TestStructuredLogger_LogsAtWarnFor4xx(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/notfound", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected at least one log entry")

	entry := observed.All()[0]
	assert.Equal(t, "client error", entry.Message)
	assert.Equal(t, zapcore.WarnLevel, entry.Level)
}

func TestStructuredLogger_LogsAtErrorFor5xx(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/boom", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected at least one log entry")

	entry := observed.All()[0]
	assert.Equal(t, "server error", entry.Message)
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
}

func TestStructuredLogger_LogsAll2xxAsInfo(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	statusCodes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
	}

	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			observed.TakeAll() // reset

			router := gin.New()
			router.Use(StructuredLogger(logger))
			router.GET("/test", func(c *gin.Context) {
				c.Status(code)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.NotZero(t, observed.Len(), "expected at least one log entry")
			assert.Equal(t, zapcore.InfoLevel, observed.All()[0].Level)
		})
	}
}

func TestStructuredLogger_LogsStructuredFields(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping?foo=bar", nil)
	req.Header.Set("User-Agent", "TestBot/2.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected at least one log entry")

	fields := observed.All()[0].Context
	fieldMap := make(map[string]bool)
	for _, f := range fields {
		fieldMap[f.Key] = true
	}

	assert.True(t, fieldMap["method"], "expected 'method' field")
	assert.True(t, fieldMap["path"], "expected 'path' field")
	assert.True(t, fieldMap["query"], "expected 'query' field")
	assert.True(t, fieldMap["status"], "expected 'status' field")
	assert.True(t, fieldMap["request_duration_ms"], "expected 'request_duration_ms' field")
	assert.True(t, fieldMap["client_ip"], "expected 'client_ip' field")
	assert.True(t, fieldMap["user_agent"], "expected 'user_agent' field")
}

func TestStructuredLogger_LogsMethod(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len())
	field, ok := findZapField(observed.All()[0].Context, "method")
	require.True(t, ok)
	assert.Equal(t, "GET", field.String)
}

func TestStructuredLogger_LogsPath(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "path")
	require.True(t, ok)
	assert.Equal(t, "/ping", field.String)
}

func TestStructuredLogger_LogsQueryParams(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/search", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"results": nil})
	})

	req := httptest.NewRequest(http.MethodGet, "/search?q=hello&sort=asc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "query")
	require.True(t, ok)
	assert.Equal(t, "q=hello&sort=asc", field.String)
}

func TestStructuredLogger_LogsStatus(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "status")
	require.True(t, ok)
	assert.Equal(t, int64(http.StatusOK), field.Integer)
}

func TestStructuredLogger_LogsRequestDurationMs(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "request_duration_ms")
	require.True(t, ok, "expected 'request_duration_ms' field")
	// Duration should be a non-negative float64.
	assert.GreaterOrEqual(t, field.Integer, int64(0),
		"request_duration_ms should be non-negative")
}

func TestStructuredLogger_LogsClientIP(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "10.0.0.42:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "client_ip")
	require.True(t, ok)
	assert.NotEmpty(t, field.String)
}

func TestStructuredLogger_LogsUserAgent(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("User-Agent", "ChaosSecScanner/3.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "user_agent")
	require.True(t, ok)
	assert.Equal(t, "ChaosSecScanner/3.0", field.String)
}

func TestStructuredLogger_LogsRequestIDFromHeader(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "req-abc-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "request_id")
	require.True(t, ok)
	assert.Equal(t, "req-abc-123", field.String)
}

func TestStructuredLogger_LogsRequestIDFromContext(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	// RequestIDMiddleware sets the request ID in context and header.
	router.Use(func(c *gin.Context) {
		c.Set(string(RequestIDKey), "ctx-req-id-456")
		c.Header(RequestIDHeader, "ctx-req-id-456")
		c.Next()
	})
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "request_id")
	require.True(t, ok)
	assert.Equal(t, "ctx-req-id-456", field.String)
}

func TestStructuredLogger_LogsTraceIDFromHeader(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(TraceIDHeader, "trace-xyz-789")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "trace_id")
	require.True(t, ok)
	assert.Equal(t, "trace-xyz-789", field.String)
}

func TestStructuredLogger_LogsBodySizeForPost(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.POST("/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/data", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	field, ok := findZapField(observed.All()[0].Context, "body_size")
	require.True(t, ok, "expected 'body_size' field for POST request")
	// body_size represents the response writer size, which should be non-negative.
	assert.GreaterOrEqual(t, field.Integer, int64(0))
}

func TestStructuredLogger_LogsBodySizeForPut(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.PUT("/data/1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
	})

	body := strings.NewReader(`{"name":"updated"}`)
	req := httptest.NewRequest(http.MethodPut, "/data/1", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	_, ok := findZapField(observed.All()[0].Context, "body_size")
	require.True(t, ok, "expected 'body_size' field for PUT request")
}

func TestStructuredLogger_LogsBodySizeForPatch(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.PATCH("/data/1", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "patched"})
	})

	body := strings.NewReader(`{"name":"patched"}`)
	req := httptest.NewRequest(http.MethodPatch, "/data/1", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	_, ok := findZapField(observed.All()[0].Context, "body_size")
	require.True(t, ok, "expected 'body_size' field for PATCH request")
}

func TestStructuredLogger_NoBodySizeForGet(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	_, ok := findZapField(observed.All()[0].Context, "body_size")
	assert.False(t, ok, "did not expect 'body_size' field for GET request")
}

func TestStructuredLogger_SkipsHealthCheckHealthz(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "request should still be processed")
	assert.Zero(t, observed.Len(), "health check requests should not be logged")
}

func TestStructuredLogger_SkipsHealthCheckHealth(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "request should still be processed")
	assert.Zero(t, observed.Len(), "health check requests should not be logged")
}

func TestStructuredLogger_SkipsHealthCheckLivez(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "request should still be processed")
	assert.Zero(t, observed.Len(), "liveness probe should not be logged")
}

func TestStructuredLogger_SkipsHealthCheckReadyz(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "request should still be processed")
	assert.Zero(t, observed.Len(), "readiness probe should not be logged")
}

func TestStructuredLogger_LogsNonHealthCheckEndpoint(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/api/v1/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotZero(t, observed.Len(), "non-health-check requests should be logged")
}

func TestStructuredLogger_LogsGinErrors(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(StructuredLogger(logger))
	router.GET("/fail", func(c *gin.Context) {
		_ = c.Error(http.ErrAbortHandler)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.NotZero(t, observed.Len(), "expected log entry")
	field, ok := findZapField(observed.All()[0].Context, "errors")
	require.True(t, ok, "expected 'errors' field when Gin errors are present")
	assert.NotEmpty(t, field.String)
}

// ---------------------------------------------------------------------------
// RequestTracer Tests
// ---------------------------------------------------------------------------

func TestRequestTracer_GeneratesTraceIDWhenMissing(t *testing.T) {
	logger, _ := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// A new trace ID should have been generated and set in the response header.
	traceID := w.Header().Get(TraceIDHeader)
	assert.NotEmpty(t, traceID, "RequestTracer should generate a trace ID when none is provided")
}

func TestRequestTracer_GeneratesValidUUID(t *testing.T) {
	logger, _ := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	traceID := w.Header().Get(TraceIDHeader)
	assert.NotEmpty(t, traceID, "expected trace ID in response")

	// UUID v4 format: 8-4-4-4-12 hex characters.
	assert.Len(t, traceID, 36, "trace ID should be a UUID with 36 characters")
	assert.Equal(t, 4, strings.Count(traceID, "-"), "UUID should have 4 hyphens")
}

func TestRequestTracer_PropagatesExistingTraceID(t *testing.T) {
	logger, _ := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(TraceIDHeader, "existing-trace-id-12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The existing trace ID should be preserved in the response.
	traceID := w.Header().Get(TraceIDHeader)
	assert.Equal(t, "existing-trace-id-12345", traceID,
		"RequestTracer should propagate existing trace ID")
}

func TestRequestTracer_SetsTraceIDInContext(t *testing.T) {
	logger, _ := newObservedLogger()
	defer logger.Sync()

	var contextTraceID string
	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		if tid, exists := c.Get(string(TraceIDKey)); exists {
			contextTraceID = tid.(string)
		}
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEmpty(t, contextTraceID, "trace ID should be set in Gin context")

	// Context trace ID should match the response header.
	responseTraceID := w.Header().Get(TraceIDHeader)
	assert.Equal(t, contextTraceID, responseTraceID,
		"context trace ID should match response header trace ID")
}

func TestRequestTracer_SetsTraceIDInRequestHeader(t *testing.T) {
	logger, _ := newObservedLogger()
	defer logger.Sync()

	var headerTraceID string
	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		headerTraceID = c.Request.Header.Get(TraceIDHeader)
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.NotEmpty(t, headerTraceID,
		"trace ID should be set on the request headers for downstream use")
}

func TestRequestTracer_LogsRequestStart(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Find the "request started" log entry.
	var foundStart bool
	for _, entry := range observed.All() {
		if entry.Message == "request started" {
			foundStart = true
			assert.Equal(t, zapcore.DebugLevel, entry.Level,
				"request start should be logged at Debug level")

			field, ok := findZapField(entry.Context, "trace_id")
			assert.True(t, ok, "request start log should include trace_id")
			assert.NotEmpty(t, field.String)
			break
		}
	}
	assert.True(t, foundStart, "expected 'request started' log entry")
}

func TestRequestTracer_LogsRequestEnd(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Find the "request completed" log entry.
	var foundEnd bool
	for _, entry := range observed.All() {
		if entry.Message == "request completed" {
			foundEnd = true
			assert.Equal(t, zapcore.DebugLevel, entry.Level,
				"request end should be logged at Debug level")

			_, ok := findZapField(entry.Context, "trace_id")
			assert.True(t, ok, "request end log should include trace_id")

			_, ok = findZapField(entry.Context, "status")
			assert.True(t, ok, "request end log should include status")

			break
		}
	}
	assert.True(t, foundEnd, "expected 'request completed' log entry")
}

func TestRequestTracer_ExistingTraceIDInStartLog(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(TraceIDHeader, "upstream-trace-999")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify the propagated trace ID appears in the start log.
	for _, entry := range observed.All() {
		if entry.Message == "request started" {
			field, ok := findZapField(entry.Context, "trace_id")
			require.True(t, ok)
			assert.Equal(t, "upstream-trace-999", field.String)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Combined StructuredLogger + RequestTracer Tests
// ---------------------------------------------------------------------------

func TestStructuredLogger_WithRequestTracer_TraceIDInLogFields(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.Use(StructuredLogger(logger))
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Find the StructuredLogger entry (message "request").
	for _, entry := range observed.All() {
		if entry.Message == "request" {
			field, ok := findZapField(entry.Context, "trace_id")
			assert.True(t, ok, "StructuredLogger should include trace_id when RequestTracer is used")
			assert.NotEmpty(t, field.String)
			return
		}
	}
	t.Fatal("expected StructuredLogger 'request' entry not found")
}

func TestStructuredLogger_WithRequestTracer_SkipsHealthCheckButTracerStillRuns(t *testing.T) {
	logger, observed := newObservedLogger()
	defer logger.Sync()

	router := gin.New()
	router.Use(RequestTracer(logger))
	router.Use(StructuredLogger(logger))
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Health check should not produce a StructuredLogger entry.
	for _, entry := range observed.All() {
		if entry.Message == "request" {
			t.Error("StructuredLogger should skip health check endpoints")
		}
	}

	// But RequestTracer should still have added the trace ID header.
	traceID := w.Header().Get(TraceIDHeader)
	assert.NotEmpty(t, traceID, "RequestTracer should still set trace ID for health checks")

	// And RequestTracer debug logs should still be present.
	var foundStart, foundEnd bool
	for _, entry := range observed.All() {
		if entry.Message == "request started" {
			foundStart = true
		}
		if entry.Message == "request completed" {
			foundEnd = true
		}
	}
	assert.True(t, foundStart, "RequestTracer should log start for health checks")
	assert.True(t, foundEnd, "RequestTracer should log end for health checks")
}
