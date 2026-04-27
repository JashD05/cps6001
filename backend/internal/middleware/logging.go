package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Constants for the logging & tracing middleware
// ---------------------------------------------------------------------------

const (
	// TraceIDHeader is the HTTP header used to propagate distributed trace IDs.
	TraceIDHeader = "X-Trace-ID"

	// TraceIDKey is the context key used to store the trace ID in the Gin context.
	TraceIDKey contextKey = "trace_id"

	// Health check endpoints that should be skipped by the structured logger.
	healthCheckPath  = "/healthz"
	healthCheckPath2 = "/health"
	livenessPath     = "/livez"
	readinessPath    = "/readyz"
)

// isHealthCheck returns true if the request path corresponds to a health check
// endpoint that should be excluded from structured request logging.
func isHealthCheck(path string) bool {
	switch path {
	case healthCheckPath, healthCheckPath2, livenessPath, readinessPath:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// StructuredLogger
// ---------------------------------------------------------------------------

// StructuredLogger returns a Gin middleware that replaces basic request logging
// with structured JSON logging via zap. Every request (except health checks)
// is logged with rich structured fields suitable for ingestion into log
// aggregation systems such as ELK, Datadog, or Loki.
//
// Logged fields:
//   - method          – HTTP method (GET, POST, …)
//   - path            – URL path
//   - query           – Raw query string (may be empty)
//   - status          – HTTP response status code
//   - request_duration_ms – Request duration in milliseconds (float64)
//   - client_ip       – Client IP address
//   - user_agent       – User-Agent header value
//   - request_id      – X-Request-ID header value (if present)
//   - trace_id        – X-Trace-ID header value (if present)
//   - body_size       – Response body size in bytes (for POST/PUT/PATCH only)
//
// Log levels are determined by the response status code:
//   - 2xx → Info
//   - 4xx → Warn
//   - 5xx → Error
//
// Health check endpoints (/healthz, /health, /livez, /readyz) are skipped
// entirely to reduce log noise from load-balancer probes.
func StructuredLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for health check endpoints.
		if isHealthCheck(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process the request.
		c.Next()

		// Calculate request duration in milliseconds.
		duration := time.Since(start)
		durationMs := float64(duration) / float64(time.Millisecond)

		// Build structured log fields.
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Float64("request_duration_ms", durationMs),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		// Attach request ID if present.
		if requestID := c.GetHeader(RequestIDHeader); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		} else if reqID, exists := c.Get(string(RequestIDKey)); exists {
			if id, ok := reqID.(string); ok {
				fields = append(fields, zap.String("request_id", id))
			}
		}

		// Attach trace ID if present.
		if traceID := c.GetHeader(TraceIDHeader); traceID != "" {
			fields = append(fields, zap.String("trace_id", traceID))
		} else if tid, exists := c.Get(string(TraceIDKey)); exists {
			if id, ok := tid.(string); ok {
				fields = append(fields, zap.String("trace_id", id))
			}
		}

		// Include request body size for methods that typically carry a body.
		method := c.Request.Method
		if method == "POST" || method == "PUT" || method == "PATCH" {
			fields = append(fields, zap.Int("body_size", c.Writer.Size()))
		}

		// Attach Gin errors if present.
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		// Log at the appropriate level based on status code.
		status := c.Writer.Status()
		switch {
		case status >= 500:
			logger.Error("server error", fields...)
		case status >= 400:
			logger.Warn("client error", fields...)
		default:
			logger.Info("request", fields...)
		}
	}
}

// ---------------------------------------------------------------------------
// RequestTracer
// ---------------------------------------------------------------------------

// RequestTracer returns a Gin middleware that enables distributed tracing
// across services. It ensures every request carries a trace ID that can be
// used to correlate logs and spans across multiple backend services.
//
// Behaviour:
//  1. If the incoming request already has an X-Trace-ID header, that value
//     is reused so that upstream services can propagate an existing trace.
//  2. If no X-Trace-ID header is present, a new UUID-based trace ID is
//     generated.
//  3. The trace ID is stored in the Gin context under the TraceIDKey so that
//     downstream handlers and middleware (e.g., StructuredLogger) can access
//     it without parsing headers.
//  4. The trace ID is added to the response headers (X-Trace-ID) so that
//     clients and downstream services can reference it.
//  5. Request start and end are logged at Debug level with the trace ID,
//     providing an easy way to trace the full lifecycle of a request.
func RequestTracer(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use existing trace ID if provided, otherwise generate one.
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		// Store trace ID in the Gin context for downstream handlers.
		c.Set(string(TraceIDKey), traceID)

		// Ensure the trace ID is available on the request headers for
		// any downstream HTTP calls made by the handler.
		c.Request.Header.Set(TraceIDHeader, traceID)

		// Add the trace ID to the response headers.
		c.Header(TraceIDHeader, traceID)

		// Log the start of the request with trace ID.
		logger.Debug("request started",
			zap.String("trace_id", traceID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("client_ip", c.ClientIP()),
		)

		// Process the request.
		c.Next()

		// Log the end of the request with trace ID and outcome.
		logger.Debug("request completed",
			zap.String("trace_id", traceID),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
		)
	}
}
