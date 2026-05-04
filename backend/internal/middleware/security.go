package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// SecurityHeaders
// ---------------------------------------------------------------------------

// SecurityHeaders returns a Gin middleware that adds security-related HTTP
// headers to every response and generates a request ID if one is not already
// present. These headers help protect against clickjacking, MIME type sniffing,
// XSS, and other common web vulnerabilities.
//
// Headers set:
//   - X-Content-Type-Options: nosniff
//   - X-Frame-Options: DENY
//   - X-XSS-Protection: 0 (modern approach — disables legacy XSS filter)
//   - Content-Security-Policy: default-src 'self'
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
//   - X-Request-ID: <uuid> (generated if not already present)
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent MIME type sniffing — browsers must respect declared Content-Type.
		c.Header("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking — disallow iframe embedding entirely.
		c.Header("X-Frame-Options", "DENY")

		// Disable legacy XSS filter (modern approach). The old "1; mode=block"
		// could actually introduce vulnerabilities; setting to 0 is recommended.
		c.Header("X-XSS-Protection", "0")

		// Content Security Policy — restrict resource loading to same origin.
		c.Header("Content-Security-Policy", "default-src 'self'")

		// Control referrer information sent to other origins.
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features the page can request.
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Strict Transport Security — enforce HTTPS for 1 year with preload.
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Propagate or generate X-Request-ID header.
		if existingID := c.GetHeader("X-Request-ID"); existingID != "" {
			// Echo the existing request ID back in the response.
			c.Header("X-Request-ID", existingID)
		} else {
			requestID := uuid.New().String()
			c.Header("X-Request-ID", requestID)
			c.Request.Header.Set("X-Request-ID", requestID)
		}

		c.Next()
	}
}

// ---------------------------------------------------------------------------
// RateLimit (in-memory, sliding window)
// ---------------------------------------------------------------------------

// rateLimitEntry holds the sliding-window state for a single client IP.
type rateLimitEntry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// rateLimiterState is the shared state for the in-memory rate limiter.
type rateLimiterState struct {
	mu      sync.RWMutex
	entries sync.Map // map[string]*rateLimitEntry
}

// RateLimit returns a Gin middleware that enforces per-IP rate limiting using
// an in-memory sliding window algorithm. Each client IP is tracked
// independently and thread-safely via sync.Map and sync.RWMutex.
//
// Parameters:
//   - maxRequests: Maximum number of requests allowed within the window.
//     If <= 0, defaults to 100.
//   - window: Duration of the sliding window. If <= 0, defaults to 1 minute.
//
// When the limit is exceeded, the middleware aborts with 429 Too Many Requests
// and sets a Retry-After header indicating how long the client should wait.
func RateLimit(maxRequests int, window time.Duration) gin.HandlerFunc {
	if maxRequests <= 0 {
		maxRequests = 100
	}
	if window <= 0 {
		window = time.Minute
	}

	state := &rateLimiterState{}

	// Background cleanup of stale entries.
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			state.entries.Range(func(key, value interface{}) bool {
				entry, ok := value.(*rateLimitEntry)
				if !ok {
					state.entries.Delete(key)
					return true
				}
				entry.mu.Lock()
				i := 0
				for i < len(entry.timestamps) && now.Sub(entry.timestamps[i]) > window {
					i++
				}
				entry.timestamps = entry.timestamps[i:]
				entry.mu.Unlock()
				if len(entry.timestamps) == 0 {
					state.entries.Delete(key)
				}
				return true
			})
		}
	}()

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now()

		// Get or create the entry for this IP.
		newEntry := &rateLimitEntry{}
		actual, _ := state.entries.LoadOrStore(clientIP, newEntry)
		entry := actual.(*rateLimitEntry)

		entry.mu.Lock()
		// Sliding window: discard timestamps outside the window.
		i := 0
		for i < len(entry.timestamps) && now.Sub(entry.timestamps[i]) > window {
			i++
		}
		entry.timestamps = entry.timestamps[i:]

		currentCount := len(entry.timestamps)

		if currentCount >= maxRequests {
			// Calculate how long until the oldest request in the window expires.
			retryAfter := window - now.Sub(entry.timestamps[0])
			if retryAfter < time.Second {
				retryAfter = time.Second
			}
			entry.mu.Unlock()

			c.Header("Retry-After", fmt.Sprintf("%d", int64(retryAfter.Seconds())+1))
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", maxRequests))
			c.Header("X-RateLimit-Remaining", "0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please slow down.",
				"code":    http.StatusTooManyRequests,
			})
			return
		}

		// Record this request.
		entry.timestamps = append(entry.timestamps, now)
		remaining := maxRequests - currentCount - 1
		entry.mu.Unlock()

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", maxRequests))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Next()
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

// CORS returns a Gin middleware that adds Cross-Origin Resource Sharing headers
// to every response. Only origins in the allowedOrigins list are permitted.
// If allowedOrigins is empty, all origins are denied (most secure default).
//
// Wildcard support:
//   - "*" allows all origins (for development).
//   - Patterns like "https://*.example.com" match any subdomain.
//
// Allowed methods: GET, POST, PUT, DELETE, PATCH, OPTIONS.
// Allowed headers: Content-Type, Authorization, X-Request-ID.
// Credentials: allowed.
// Preflight requests: handled with 204 No Content.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	// Build lookup structures.
	originSet := make(map[string]bool)
	allowAll := false
	var wildcardPatterns []string

	for _, o := range allowedOrigins {
		trimmed := strings.TrimSpace(o)
		if trimmed == "*" {
			allowAll = true
			break
		}
		if trimmed != "" {
			// If the origin contains a wildcard pattern like "https://*.example.com",
			// store it separately for matching.
			if strings.Contains(trimmed, "*") {
				wildcardPatterns = append(wildcardPatterns, trimmed)
			} else {
				originSet[trimmed] = true
			}
		}
	}

	// isOriginAllowed checks whether a request origin is in the allowed list,
	// including wildcard pattern matching.
	isOriginAllowed := func(origin string) bool {
		if allowAll {
			return true
		}
		if originSet[origin] {
			return true
		}
		for _, pattern := range wildcardPatterns {
			if matchWildcardOrigin(pattern, origin) {
				return true
			}
		}
		return false
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		allowed := isOriginAllowed(origin)

		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			// Only set Allow-Credentials when a specific origin is echoed.
			// Browsers reject the combination of wildcard origin (*) and credentials.
			c.Header("Access-Control-Allow-Credentials", "true")
		} else if allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		// If the origin is not allowed, we simply don't set the header —
		// the browser will block the request.

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		c.Header("Access-Control-Max-Age", "86400") // 24-hour preflight cache

		// Handle preflight (OPTIONS) requests.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// matchWildcardOrigin checks whether a request origin matches a wildcard pattern.
// The pattern format is "https://*.example.com" or "http://*.example.com".
// It supports a single wildcard '*' at the subdomain level.
func matchWildcardOrigin(pattern, origin string) bool {
	// No wildcard means exact match (handled elsewhere, but just in case).
	if !strings.Contains(pattern, "*") {
		return pattern == origin
	}

	// Split pattern at the wildcard.
	parts := strings.SplitN(pattern, "*", 2)
	prefix := parts[0]
	suffix := parts[1]

	if !strings.HasPrefix(origin, prefix) {
		return false
	}
	if !strings.HasSuffix(origin, suffix) {
		return false
	}

	// The portion between prefix and suffix must be a valid subdomain
	// (non-empty, no slashes).
	inner := origin[len(prefix) : len(origin)-len(suffix)]
	if inner == "" {
		return false
	}
	if strings.Contains(inner, "/") {
		return false
	}

	return true
}

// ---------------------------------------------------------------------------
// RequestSizeLimit
// ---------------------------------------------------------------------------

// RequestSizeLimit returns a Gin middleware that rejects request bodies larger
// than maxBytes. This prevents clients from sending excessively large payloads
// that could exhaust server memory or cause denial-of-service.
//
// The check is performed in two stages:
//  1. If the Content-Length header is present and exceeds maxBytes, the request
//     is rejected immediately with 413 Payload Too Large before the body is read.
//  2. If Content-Length is absent or unknown, the body is wrapped with
//     http.MaxBytesReader so that any subsequent read by the handler that
//     exceeds maxBytes will automatically trigger a 413 response.
func RequestSizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fast path: check Content-Length header first.
		if c.Request.ContentLength > maxBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "request_too_large",
				"message": fmt.Sprintf("Request body exceeds the maximum allowed size of %d bytes.", maxBytes),
				"code":    http.StatusRequestEntityTooLarge,
			})
			return
		}

		// If Content-Length is unknown (0 or -1), wrap the body with
		// MaxBytesReader for runtime enforcement. The handler will
		// receive a "request body too large" error if it tries to read
		// past the limit.
		if c.Request.ContentLength <= 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}

		c.Next()
	}
}

// ---------------------------------------------------------------------------
// AuditLog
// ---------------------------------------------------------------------------

// AuditLog returns a Gin middleware that logs every HTTP request with details
// including method, path, status, duration, IP, and user-agent. It uses zap
// structured logging for easy querying in log aggregation systems.
//
// If no X-Request-ID header is present on the incoming request, one is generated
// automatically so that every request can be traced through the system.
func AuditLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate a request ID if not already present.
		if c.GetHeader("X-Request-ID") == "" {
			requestID := uuid.New().String()
			c.Header("X-Request-ID", requestID)
			c.Request.Header.Set("X-Request-ID", requestID)
		}

		start := time.Now()

		// Process the request.
		c.Next()

		// Calculate request duration.
		duration := time.Since(start)

		// Log with structured fields for easy querying in log aggregation systems.
		logger.Info("http_request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("request_id", c.GetHeader("X-Request-ID")),
			zap.Int("body_size", c.Writer.Size()),
		)

		// Log warnings for slow requests (>5s).
		if duration > 5*time.Second {
			logger.Warn("slow_request",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Duration("duration", duration),
				zap.String("client_ip", c.ClientIP()),
			)
		}

		// Log errors for server-side errors (5xx).
		if c.Writer.Status() >= http.StatusInternalServerError {
			logger.Error("server_error",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("status", c.Writer.Status()),
				zap.Duration("duration", duration),
				zap.String("client_ip", c.ClientIP()),
			)
		}
	}
}

// ---------------------------------------------------------------------------
// SanitizeInput
// ---------------------------------------------------------------------------

// SanitizeInput returns a Gin middleware that performs basic input sanitization
// on incoming requests:
//   - Trims leading and trailing whitespace from all query parameters.
//   - Rejects requests that contain null bytes (\x00) in query parameters or
//     the request body, responding with 400 Bad Request.
func SanitizeInput() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check query parameters for null bytes and trim whitespace.
		q := c.Request.URL.Query()
		sanitized := url.Values{}
		nullFound := false

		for key, values := range q {
			for i, v := range values {
				if strings.Contains(v, "\x00") {
					nullFound = true
				}
				values[i] = strings.TrimSpace(v)
			}
			sanitized[key] = values
		}

		if nullFound {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_input",
				"message": "Request contains null bytes in query parameters.",
				"code":    http.StatusBadRequest,
			})
			return
		}

		// Replace the query string with sanitized values.
		c.Request.URL.RawQuery = sanitized.Encode()

		// Check request body for null bytes (up to a reasonable limit).
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)) // 1MB limit for scanning
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error":   "invalid_input",
					"message": "Failed to read request body.",
					"code":    http.StatusBadRequest,
				})
				return
			}

			if bytes.Contains(bodyBytes, []byte{0x00}) {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error":   "invalid_input",
					"message": "Request contains null bytes in body.",
					"code":    http.StatusBadRequest,
				})
				return
			}

			// Restore the body so downstream handlers can still read it.
			c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		c.Next()
	}
}
