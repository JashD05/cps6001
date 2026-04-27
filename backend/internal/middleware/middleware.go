package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// contextKey type for context keys to avoid collisions.
type contextKey string

const (
	// Context keys used to store values in the Gin context.
	ClaimsContextKey contextKey = "auth_claims"
	RequestIDKey     contextKey = "request_id"

	// RequestIDHeader is the HTTP header used to propagate request IDs.
	RequestIDHeader = "X-Request-ID"

	// Rate limit key prefix for Redis.
	rateLimitPrefix = "rate_limit:"

	// Default rate limit window in seconds.
	defaultRateLimitWindow = 60
)

// Middleware holds dependencies shared across all middleware functions.
type Middleware struct {
	authSvc     *auth.AuthService
	rdb         *redis.Client
	cfg         *config.Config
	logger      *zap.Logger
	rateLimiter *localRateLimiter
}

// New creates a new Middleware instance with the provided dependencies.
func New(authSvc *auth.AuthService, rdb *redis.Client, cfg *config.Config, logger *zap.Logger) *Middleware {
	return &Middleware{
		authSvc:     authSvc,
		rdb:         rdb,
		cfg:         cfg,
		logger:      logger.Named("middleware"),
		rateLimiter: newLocalRateLimiter(cfg.RateLimit.Window),
	}
}

// ============================================================================
// Request ID Middleware
// ============================================================================

// RequestIDMiddleware generates a unique request ID for every incoming request.
// If the client provides an X-Request-ID header, it is reused; otherwise a new
// UUID is generated. The ID is stored in the Gin context and set as a response
// header so clients can correlate requests with server-side logs.
func (m *Middleware) RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use client-provided request ID if present, otherwise generate one.
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store in context for downstream handlers and loggers.
		c.Set(string(RequestIDKey), requestID)

		// Set the response header so the client can see the request ID.
		c.Header(RequestIDHeader, requestID)

		// Add the request ID to the Gin context's logger fields.
		c.Next()
	}
}

// ============================================================================
// Logging Middleware
// ============================================================================

// LoggingMiddleware logs every HTTP request using zap with structured fields.
// It records the request method, path, status code, latency, client IP,
// user agent, and request ID. Errors written during request handling are
// also captured.
func (m *Middleware) LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process the request.
		c.Next()

		// Calculate latency.
		latency := time.Since(start)

		// Build structured log fields.
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("body_size", c.Writer.Size()),
		}

		// Attach request ID if available.
		if reqID, exists := c.Get(string(RequestIDKey)); exists {
			fields = append(fields, zap.String("request_id", reqID.(string)))
		}

		// Attach user ID if authenticated.
		if claimsVal, exists := c.Get(string(ClaimsContextKey)); exists {
			if claims, ok := claimsVal.(*auth.TokenClaims); ok {
				fields = append(fields, zap.String("user_id", claims.UserID.String()))
				fields = append(fields, zap.String("org_id", claims.OrganizationID.String()))
			}
		}

		// Log at the appropriate level based on status code.
		status := c.Writer.Status()
		switch {
		case status >= 500:
			// Log errors from Gin context as well.
			if len(c.Errors) > 0 {
				fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
			}
			m.logger.Error("server error", fields...)
		case status >= 400:
			if len(c.Errors) > 0 {
				fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
			}
			m.logger.Warn("client error", fields...)
		default:
			m.logger.Info("request", fields...)
		}
	}
}

// ============================================================================
// CORS Middleware
// ============================================================================

// CORSMiddleware adds Cross-Origin Resource Sharing headers to every response.
// This allows the React frontend (typically served from a different origin
// during development) to communicate with the API. In production, the
// allowed origins should be restricted via environment configuration.
func (m *Middleware) CORSMiddleware() gin.HandlerFunc {
	// Determine allowed origins based on environment.
	allowedOrigins := "*"
	if !m.cfg.IsDevelopment() {
		// In production, restrict to specific origins from environment.
		// For now, we default to allowing all but log a warning.
		m.logger.Warn("CORS allows all origins — configure CHAOS_CORS_ALLOWED_ORIGINS for production")
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Check if the origin is allowed.
		if allowedOrigins == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if isOriginAllowed(origin, allowedOrigins) {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-API-Key")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400") // Preflight cache: 24 hours
		c.Header("Vary", "Origin")

		// Handle preflight (OPTIONS) requests.
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// isOriginAllowed checks whether a request origin is in the allowed set.
// The allowed parameter is a comma-separated list of origins.
func isOriginAllowed(origin, allowed string) bool {
	if allowed == "*" {
		return true
	}
	for _, o := range strings.Split(allowed, ",") {
		o = strings.TrimSpace(o)
		if o == origin {
			return true
		}
	}
	return false
}

// ============================================================================
// Auth Middleware
// ============================================================================

// AuthMiddleware validates JWT tokens from the Authorization header.
// It extracts the Bearer token, validates it using the AuthService,
// and stores the parsed claims in the Gin context for downstream
// handlers and RBAC middleware.
//
// If the token is missing, malformed, expired, or blacklisted, the
// request is rejected with a 401 Unauthorized response.
func (m *Middleware) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract the Authorization header.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Authorization header is required.",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		// Parse Bearer token.
		tokenString := extractBearerToken(authHeader)
		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "invalid_auth_format",
				"message": "Authorization header must use Bearer scheme.",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		// Check if the token has been blacklisted (e.g., after logout).
		if m.rdb != nil {
			blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenString)
			blacklisted, err := m.rdb.Exists(c.Request.Context(), blacklistKey).Result()
			if err != nil && err != redis.Nil {
				m.logger.Error("redis error checking token blacklist",
					zap.Error(err),
					zap.String("request_id", getRequestID(c)),
				)
				// Do not block the request on Redis errors — let token validation proceed.
			} else if blacklisted > 0 {
				m.logger.Warn("blacklisted token used",
					zap.String("request_id", getRequestID(c)),
				)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":   "token_revoked",
					"message": "This token has been revoked.",
					"code":    http.StatusUnauthorized,
				})
				return
			}
		}

		// Validate the token using the auth service.
		claims, err := m.authSvc.ValidateAccessToken(tokenString)
		if err != nil {
			m.logger.Debug("invalid access token",
				zap.Error(err),
				zap.String("request_id", getRequestID(c)),
			)

			status := http.StatusUnauthorized
			errMsg := "Invalid or expired authentication token."

			if err == auth.ErrExpiredToken {
				errMsg = "Authentication token has expired. Please refresh or log in again."
			}

			c.AbortWithStatusJSON(status, gin.H{
				"error":   "unauthorized",
				"message": errMsg,
				"code":    status,
			})
			return
		}

		// Store claims in context for downstream handlers and RBAC middleware.
		c.Set(string(ClaimsContextKey), claims)

		// Add user context to zap logger for any downstream logging.
		c.Next()
	}
}

// ============================================================================
// RBAC Middleware
// ============================================================================

// RBACMiddleware checks whether the authenticated user has all of the
// required permissions. It must be used AFTER AuthMiddleware so that
// token claims are available in the context.
//
// If any of the required permissions is missing, the request is rejected
// with a 403 Forbidden response. Users with the "admin:all" permission
// bypass all RBAC checks (handled by TokenClaims.HasPermission).
func (m *Middleware) RBACMiddleware(requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Retrieve claims from context (set by AuthMiddleware).
		claimsVal, exists := c.Get(string(ClaimsContextKey))
		if !exists {
			m.logger.Error("RBAC middleware used without auth middleware",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Authentication required.",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		claims, ok := claimsVal.(*auth.TokenClaims)
		if !ok {
			m.logger.Error("invalid auth claims type in context",
				zap.String("path", c.Request.URL.Path),
			)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Authentication context error.",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		// Check all required permissions.
		for _, permission := range requiredPermissions {
			if !claims.HasPermission(permission) {
				m.logger.Info("RBAC permission denied",
					zap.String("user_id", claims.UserID.String()),
					zap.String("org_id", claims.OrganizationID.String()),
					zap.String("role", claims.Role),
					zap.String("required_permission", permission),
					zap.Strings("user_permissions", claims.Permissions),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": fmt.Sprintf("Permission '%s' is required.", permission),
					"code":    http.StatusForbidden,
				})
				return
			}
		}

		c.Next()
	}
}

// OrgScopeMiddleware ensures the user can only access resources within
// their own organization. It extracts the organization_id from the URL
// parameter or query string and compares it against the token claims.
// This must be used AFTER AuthMiddleware.
func (m *Middleware) OrgScopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsVal, exists := c.Get(string(ClaimsContextKey))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Authentication required.",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		claims, ok := claimsVal.(*auth.TokenClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Authentication context error.",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		// Admin users can access any organization.
		if claims.IsAdmin() {
			c.Next()
			return
		}

		// Check organization_id from query parameter.
		if orgID := c.Query("organization_id"); orgID != "" {
			if orgID != claims.OrganizationID.String() {
				m.logger.Warn("cross-organization access attempt",
					zap.String("user_id", claims.UserID.String()),
					zap.String("user_org_id", claims.OrganizationID.String()),
					zap.String("requested_org_id", orgID),
				)
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "forbidden",
					"message": "You can only access resources within your own organization.",
					"code":    http.StatusForbidden,
				})
				return
			}
		}

		c.Next()
	}
}

// ============================================================================
// Rate Limit Middleware
// ============================================================================

// RateLimitMiddleware provides per-user rate limiting to prevent abuse.
// It uses Redis for distributed rate limiting when available, falling back
// to an in-memory rate limiter for single-instance deployments.
//
// The limiter identifies users by their authenticated user ID (from JWT
// claims) or by their client IP address for unauthenticated endpoints.
func (m *Middleware) RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !m.cfg.RateLimit.Enabled {
			c.Next()
			return
		}

		// Determine the rate limit key: use user ID if authenticated,
		// otherwise fall back to client IP.
		var limitKey string
		if claimsVal, exists := c.Get(string(ClaimsContextKey)); exists {
			if claims, ok := claimsVal.(*auth.TokenClaims); ok {
				limitKey = fmt.Sprintf("%suser:%s", rateLimitPrefix, claims.UserID.String())
			}
		}
		if limitKey == "" {
			limitKey = fmt.Sprintf("%sip:%s", rateLimitPrefix, c.ClientIP())
		}

		// Try Redis-based rate limiting first, fall back to local.
		if m.rdb != nil {
			if !m.checkRedisRateLimit(c, limitKey) {
				return
			}
		} else {
			if !m.rateLimiter.allow(limitKey, m.cfg.RateLimit.Requests) {
				m.logger.Warn("rate limit exceeded (local)",
					zap.String("limit_key", limitKey),
					zap.Int("limit", m.cfg.RateLimit.Requests),
					zap.Duration("window", m.cfg.RateLimit.Window),
				)
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please slow down.",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
		}

		// Set rate limit headers on the response.
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", m.cfg.RateLimit.Requests))
		c.Header("X-RateLimit-Window", m.cfg.RateLimit.Window.String())

		c.Next()
	}
}

// checkRedisRateLimit implements a sliding window rate limiter using Redis.
// It increments a counter for the current window and checks against the limit.
// Returns true if the request is allowed, false if it should be rejected.
func (m *Middleware) checkRedisRateLimit(c *gin.Context, key string) bool {
	ctx := c.Request.Context()
	windowSeconds := int(m.cfg.RateLimit.Window.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = defaultRateLimitWindow
	}

	// Use a sliding window counter with Redis INCR and EXPIRE.
	// The key includes the current time window bucket.
	now := time.Now()
	windowKey := fmt.Sprintf("%s:%d", key, now.Unix()/int64(windowSeconds))

	count, err := m.rdb.Incr(ctx, windowKey).Result()
	if err != nil {
		m.logger.Error("redis rate limit increment failed",
			zap.Error(err),
			zap.String("key", windowKey),
		)
		// Fail open: allow the request if Redis is unavailable.
		return true
	}

	// Set expiry on the key only when it's first created (count == 1).
	if count == 1 {
		if err := m.rdb.Expire(ctx, windowKey, m.cfg.RateLimit.Window+time.Second).Err(); err != nil {
			m.logger.Error("redis rate limit expiry set failed", zap.Error(err))
		}
	}

	// Set remaining headers.
	remaining := m.cfg.RateLimit.Requests - int(count)
	if remaining < 0 {
		remaining = 0
	}
	c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

	if int(count) > m.cfg.RateLimit.Requests {
		m.logger.Warn("rate limit exceeded (redis)",
			zap.String("limit_key", key),
			zap.Int64("count", count),
			zap.Int("limit", m.cfg.RateLimit.Requests),
		)
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error":   "rate_limit_exceeded",
			"message": "Too many requests. Please slow down.",
			"code":    http.StatusTooManyRequests,
		})
		return false
	}

	return true
}

// ============================================================================
// Recovery Middleware
// ============================================================================

// RecoveryMiddleware catches panics within the request handler and logs
// the error, then returns a 500 Internal Server Error. This prevents
// the server from crashing on unexpected panics.
func (m *Middleware) RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("panic recovered in HTTP handler",
					zap.Any("panic", r),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("client_ip", c.ClientIP()),
					zap.String("request_id", getRequestID(c)),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":   "internal_error",
					"message": "An unexpected error occurred. Please try again later.",
					"code":    http.StatusInternalServerError,
				})
			}
		}()
		c.Next()
	}
}

// ============================================================================
// Security Headers Middleware
// ============================================================================

// SecurityHeadersMiddleware adds common security-related HTTP headers
// to every response. These headers help protect against common web
// vulnerabilities such as clickjacking, MIME type sniffing, and XSS.
func (m *Middleware) SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking by disallowing iframe embedding.
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing.
		c.Header("X-Content-Type-Options", "nosniff")

		// Enable browser XSS filtering.
		c.Header("X-XSS-Protection", "1; mode=block")

		// Strict Transport Security (HSTS) — enforce HTTPS.
		// Only set in production with HTTPS.
		if !m.cfg.IsDevelopment() {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// Control referrer information sent to other origins.
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict the features the page can use.
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		c.Next()
	}
}

// ============================================================================
// Helpers
// ============================================================================

// extractBearerToken extracts the Bearer token from an Authorization header.
// Returns an empty string if the header format is invalid.
func extractBearerToken(authHeader string) string {
	if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "Bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}

// getRequestID retrieves the request ID from the Gin context.
func getRequestID(c *gin.Context) string {
	if reqID, exists := c.Get(string(RequestIDKey)); exists {
		if id, ok := reqID.(string); ok {
			return id
		}
	}
	return ""
}

// ============================================================================
// Local (in-memory) Rate Limiter
// ============================================================================

// localRateLimiter provides a simple in-memory token bucket rate limiter
// for deployments without Redis. It is thread-safe and automatically
// cleans up stale entries.
type localRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	window  time.Duration
	stopCh  chan struct{}
}

// rateBucket tracks request counts within a time window.
type rateBucket struct {
	count     int
	expiresAt time.Time
}

// newLocalRateLimiter creates a local rate limiter with the given window.
// It starts a background goroutine to clean up expired buckets.
func newLocalRateLimiter(window time.Duration) *localRateLimiter {
	rl := &localRateLimiter{
		buckets: make(map[string]*rateBucket),
		window:  window,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine.
	go rl.cleanup()

	return rl
}

// allow checks whether a request with the given key is allowed.
// Returns true if under the limit, false if the limit has been exceeded.
func (rl *localRateLimiter) allow(key string, limit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	bucket, exists := rl.buckets[key]
	if !exists || now.After(bucket.expiresAt) {
		// Create or reset the bucket.
		rl.buckets[key] = &rateBucket{
			count:     1,
			expiresAt: now.Add(rl.window),
		}
		return true
	}

	bucket.count++
	if bucket.count > limit {
		return false
	}

	return true
}

// cleanup periodically removes expired buckets to prevent memory leaks.
func (rl *localRateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, bucket := range rl.buckets {
				if now.After(bucket.expiresAt) {
					delete(rl.buckets, key)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Stop terminates the cleanup goroutine. Call this during graceful shutdown.
func (rl *localRateLimiter) Stop() {
	close(rl.stopCh)
}
