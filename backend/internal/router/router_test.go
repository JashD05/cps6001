package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/database"
	"github.com/chaos-sec/backend/internal/middleware"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

func setupTestConfig() *config.Config {
	return &config.Config{
		Env: "development",
		JWT: config.JWTConfig{
			Secret:        "test-jwt-secret-for-unit-tests-32chars",
			Expiry:        1 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
			Issuer:        "chaos-sec-test",
		},
		Server: config.ServerConfig{
			CORSAllowedOrigins: "*",
		},
		RateLimit: config.RateLimitConfig{
			Enabled:  false,
			Requests: 1000,
			Window:   time.Minute,
		},
	}
}

func setupTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// setupTestDB creates a *database.DB backed by go-sqlmock.
// The logger and config fields of database.DB are left nil;
// this is safe for health check testing which only uses the embedded *sql.DB.
func setupTestDB(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err, "failed to create sqlmock")
	t.Cleanup(func() { sqlDB.Close() })
	db := &database.DB{DB: sqlDB}
	return db, mock
}

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// withAuthClaims injects TokenClaims into the Gin context, simulating
// what AuthMiddleware does for authenticated routes.
func withAuthClaims(claims *auth.TokenClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("auth_claims", claims)
		c.Next()
	}
}

// generateTestToken creates a valid JWT access token for testing.
func generateTestToken(t *testing.T, cfg *config.Config, perms []string) string {
	t.Helper()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)
	userID := uuid.New()
	orgID := uuid.New()
	token, _, err := authSvc.GenerateAccessToken(userID, "test@example.com", "admin", orgID, perms)
	require.NoError(t, err)
	return token
}

// ---------------------------------------------------------------------------
// Liveness Handler Tests
// ---------------------------------------------------------------------------

func TestLivenessHandler_Success(t *testing.T) {
	r := &Router{
		engine: setupTestEngine(),
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/live", r.livenessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "alive", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.False(t, resp.Timestamp.IsZero(), "timestamp should be set")
}

// Verify that the liveness endpoint does NOT require authentication.
func TestLivenessEndpoint_NoAuthRequired(t *testing.T) {
	r := &Router{
		engine: setupTestEngine(),
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/live", r.livenessHandler)

	// Request without any Authorization header
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ---------------------------------------------------------------------------
// Readiness Handler Tests
// ---------------------------------------------------------------------------

func TestReadinessHandler_AllHealthy(t *testing.T) {
	db, mock := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr

	mock.ExpectPing()

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    rdb,
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ready", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.Equal(t, "healthy", resp.Checks["database"])
	assert.Equal(t, "healthy", resp.Checks["redis"])
	assert.False(t, resp.Timestamp.IsZero())

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadinessHandler_UnhealthyDB(t *testing.T) {
	db, mock := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr

	mock.ExpectPing().WillReturnError(fmt.Errorf("connection refused"))

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    rdb,
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", resp.Status)
	assert.Contains(t, resp.Checks["database"], "unhealthy")
	assert.Equal(t, "healthy", resp.Checks["redis"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadinessHandler_NilRedis(t *testing.T) {
	db, mock := setupTestDB(t)

	mock.ExpectPing()

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    nil, // Redis not configured
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ready", resp.Status)
	assert.Equal(t, "healthy", resp.Checks["database"])
	assert.Equal(t, "not_configured", resp.Checks["redis"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadinessHandler_UnhealthyRedis(t *testing.T) {
	db, mock := setupTestDB(t)

	mock.ExpectPing()

	// Create a miniredis, get a Redis client, then close miniredis
	// to simulate an unavailable Redis server.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	mr.Close()

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    rdb,
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", resp.Status)
	assert.Equal(t, "healthy", resp.Checks["database"])
	assert.Contains(t, resp.Checks["redis"], "unhealthy")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadinessHandler_BothUnhealthy(t *testing.T) {
	db, mock := setupTestDB(t)

	mock.ExpectPing().WillReturnError(fmt.Errorf("connection refused"))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	mr.Close()

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    rdb,
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp models.HealthCheckResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "not_ready", resp.Status)
	assert.Contains(t, resp.Checks["database"], "unhealthy")
	assert.Contains(t, resp.Checks["redis"], "unhealthy")

	require.NoError(t, mock.ExpectationsWereMet())
}

// Verify that the readiness endpoint does NOT require authentication.
func TestReadinessEndpoint_NoAuthRequired(t *testing.T) {
	db, mock := setupTestDB(t)

	mock.ExpectPing()

	r := &Router{
		engine: setupTestEngine(),
		db:     db,
		rdb:    nil,
		logger: zap.NewNop(),
	}
	r.engine.GET("/health/ready", r.readinessHandler)

	// Request without any Authorization header
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// Auth Middleware Tests
// ---------------------------------------------------------------------------

func TestAuthMiddleware_NoHeader(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
	assert.Contains(t, resp["message"], "Authorization header is required")
}

func TestAuthMiddleware_InvalidFormat(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []struct {
		name        string
		authHeader  string
		wantErrCode string
	}{
		{"basic auth instead of bearer", "Basic dXNlcjpwYXNz", "invalid_auth_format"},
		{"bearer without space", "Bearertoken-no-space", "invalid_auth_format"},
		{"empty bearer", "Bearer ", "invalid_auth_format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.wantErrCode, resp["error"])
		})
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer this-is-not-a-valid-jwt")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	cfg := setupTestConfig()
	cfg.JWT.Expiry = 1 * time.Nanosecond

	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	userID := uuid.New()
	orgID := uuid.New()
	token, _, err := authSvc.GenerateAccessToken(userID, "test@example.com", "admin", orgID, []string{"admin:all"})
	require.NoError(t, err)

	// Wait for the token to expire
	time.Sleep(10 * time.Millisecond)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
	assert.Contains(t, resp["message"], "expired")
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		claimsVal, _ := c.Get("auth_claims")
		claims, ok := claimsVal.(*auth.TokenClaims)
		if ok {
			c.JSON(http.StatusOK, gin.H{
				"message": "success",
				"user_id": claims.UserID.String(),
				"role":    claims.Role,
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no claims"})
		}
	})

	token := generateTestToken(t, cfg, []string{"admin:all"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "success", resp["message"])
	assert.Equal(t, "admin", resp["role"])
}

func TestAuthMiddleware_RefreshTokenRejected(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	// Generate a refresh token instead of an access token
	userID := uuid.New()
	orgID := uuid.New()
	refreshToken, _, err := authSvc.GenerateRefreshToken(userID, "test@example.com", "admin", orgID)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Refresh tokens must be rejected by AuthMiddleware (ValidateAccessToken)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_BlacklistedToken(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mr, rdb := setupMiniredis(t)
	_ = mr

	mw := middleware.New(authSvc, rdb, cfg, zap.NewNop())

	userID := uuid.New()
	orgID := uuid.New()
	token, claims, err := authSvc.GenerateAccessToken(userID, "test@example.com", "admin", orgID, []string{"admin:all"})
	require.NoError(t, err)

	// Blacklist the token in Redis
	blacklistKey := fmt.Sprintf("token:blacklist:%s", claims.ID)
	rdb.Set(context.Background(), blacklistKey, "1", time.Hour)

	engine := setupTestEngine()
	engine.Use(mw.AuthMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "token_revoked", resp["error"])
}

// ---------------------------------------------------------------------------
// RBAC Middleware Tests
// ---------------------------------------------------------------------------

func TestRBACMiddleware_HasPermission(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	claims := &auth.TokenClaims{
		UserID:         uuid.New(),
		Email:          "admin@example.com",
		Role:           "admin",
		OrganizationID: uuid.New(),
		Permissions:    []string{"experiments:read", "experiments:write"},
		TokenType:      auth.TokenTypeAccess,
	}

	engine := setupTestEngine()
	engine.Use(withAuthClaims(claims))
	engine.Use(mw.RBACMiddleware("experiments:read"))
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRBACMiddleware_NoPermission(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	claims := &auth.TokenClaims{
		UserID:         uuid.New(),
		Email:          "viewer@example.com",
		Role:           "viewer",
		OrganizationID: uuid.New(),
		Permissions:    []string{"experiments:read"}, // Missing experiments:write
		TokenType:      auth.TokenTypeAccess,
	}

	engine := setupTestEngine()
	engine.Use(withAuthClaims(claims))
	engine.Use(mw.RBACMiddleware("experiments:write"))
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "forbidden", resp["error"])
	assert.Contains(t, resp["message"], "experiments:write")
}

func TestRBACMiddleware_NoClaimsInContext(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	// No withAuthClaims middleware — claims not in context
	engine.Use(mw.RBACMiddleware("experiments:read"))
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "internal_error", resp["error"])
}

func TestRBACMiddleware_AdminAllPermission(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	claims := &auth.TokenClaims{
		UserID:         uuid.New(),
		Email:          "admin@example.com",
		Role:           "admin",
		OrganizationID: uuid.New(),
		Permissions:    []string{"admin:all"}, // admin:all grants all permissions
		TokenType:      auth.TokenTypeAccess,
	}

	engine := setupTestEngine()
	engine.Use(withAuthClaims(claims))
	engine.Use(mw.RBACMiddleware("experiments:write", "clusters:read", "users:manage"))
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// admin:all should pass all permission checks
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRBACMiddleware_MultipleRequiredPermissions(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	t.Run("has one but not all", func(t *testing.T) {
		claims := &auth.TokenClaims{
			UserID:         uuid.New(),
			Email:          "partial@example.com",
			Role:           "operator",
			OrganizationID: uuid.New(),
			Permissions:    []string{"experiments:read"}, // Missing experiments:write
			TokenType:      auth.TokenTypeAccess,
		}

		engine := setupTestEngine()
		engine.Use(withAuthClaims(claims))
		engine.Use(mw.RBACMiddleware("experiments:read", "experiments:write"))
		engine.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("has all required", func(t *testing.T) {
		claims := &auth.TokenClaims{
			UserID:         uuid.New(),
			Email:          "full@example.com",
			Role:           "admin",
			OrganizationID: uuid.New(),
			Permissions:    []string{"experiments:read", "experiments:write"},
			TokenType:      auth.TokenTypeAccess,
		}

		engine := setupTestEngine()
		engine.Use(withAuthClaims(claims))
		engine.Use(mw.RBACMiddleware("experiments:read", "experiments:write"))
		engine.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "success"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// Protected Route Tests — verify route groups require auth
// ---------------------------------------------------------------------------

func TestProtectedRoutes_RequireAuth(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()

	// Set up protected routes mimicking the real router structure
	v1 := engine.Group("/api/v1")
	experiments := v1.Group("/experiments")
	experiments.Use(mw.AuthMiddleware())
	{
		experiments.GET("", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "list"}) })
		experiments.POST("", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "create"}) })
		experiments.GET("/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "get"}) })
	}

	clusters := v1.Group("/clusters")
	clusters.Use(mw.AuthMiddleware())
	{
		clusters.GET("", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "list clusters"}) })
		clusters.POST("", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "register cluster"}) })
	}

	dashboard := v1.Group("/dashboard")
	dashboard.Use(mw.AuthMiddleware())
	{
		dashboard.GET("/summary", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "dashboard"}) })
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list experiments", http.MethodGet, "/api/v1/experiments"},
		{"create experiment", http.MethodPost, "/api/v1/experiments"},
		{"get experiment", http.MethodGet, "/api/v1/experiments/some-id"},
		{"list clusters", http.MethodGet, "/api/v1/clusters"},
		{"register cluster", http.MethodPost, "/api/v1/clusters"},
		{"dashboard summary", http.MethodGet, "/api/v1/dashboard/summary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code,
				"Route %s %s should require authentication", tt.method, tt.path)
		})
	}
}

func TestProtectedRoutes_Authenticated(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()

	v1 := engine.Group("/api/v1")
	experiments := v1.Group("/experiments")
	experiments.Use(mw.AuthMiddleware())
	{
		experiments.GET("", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"message": "list"}) })
	}

	token := generateTestToken(t, cfg, []string{"admin:all"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "list", resp["message"])
}

// ---------------------------------------------------------------------------
// Middleware Chain Integration Tests
// ---------------------------------------------------------------------------

func TestMiddlewareChain_AuthThenRBAC(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	apiGroup := engine.Group("/api/v1")
	protected := apiGroup.Group("/experiments")
	protected.Use(mw.AuthMiddleware())
	protected.Use(mw.RBACMiddleware("experiments:read"))
	{
		protected.GET("", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "experiments list"})
		})
	}

	t.Run("no auth header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("valid token with correct permission", func(t *testing.T) {
		token := generateTestToken(t, cfg, []string{"experiments:read"})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("valid token without permission", func(t *testing.T) {
		token := generateTestToken(t, cfg, []string{"clusters:read"})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("admin:all passes all checks", func(t *testing.T) {
		token := generateTestToken(t, cfg, []string{"admin:all"})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/experiments", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// ---------------------------------------------------------------------------
// CORS Middleware Tests
// ---------------------------------------------------------------------------

func TestCORSMiddleware_PreflightRequest(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.CORSMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	// Preflight OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// CORS preflight should return 204 No Content
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Check CORS headers
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORSMiddleware_ActualRequest(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.CORSMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// ---------------------------------------------------------------------------
// Recovery Middleware Tests
// ---------------------------------------------------------------------------

func TestRecoveryMiddleware_PanicRecovery(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.RecoveryMiddleware())
	engine.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Recovery middleware should catch the panic and return 500
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------------------------------------------------------------------------
// Security Headers Middleware Tests
// ---------------------------------------------------------------------------

func TestSecurityHeadersMiddleware(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.SecurityHeadersMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check that security headers are set
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, w.Header().Get("X-XSS-Protection"))
}

// ---------------------------------------------------------------------------
// Request ID Middleware Tests
// ---------------------------------------------------------------------------

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.RequestIDMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		requestID := c.GetHeader(middleware.RequestIDHeader)
		c.JSON(http.StatusOK, gin.H{"request_id": requestID})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Response header should have a request ID
	requestID := w.Header().Get(middleware.RequestIDHeader)
	assert.NotEmpty(t, requestID, "request ID should be set in response header")

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["request_id"])
}

func TestRequestIDMiddleware_UsesExistingID(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	engine := setupTestEngine()
	engine.Use(mw.RequestIDMiddleware())
	engine.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	existingID := "existing-request-id-123"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(middleware.RequestIDHeader, existingID)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, existingID, w.Header().Get(middleware.RequestIDHeader))
}

// ---------------------------------------------------------------------------
// Full Middleware Stack Test
// ---------------------------------------------------------------------------

func TestFullMiddlewareStack_HealthEndpointsAccessible(t *testing.T) {
	cfg := setupTestConfig()
	authSvc, err := auth.New(&cfg.JWT)
	require.NoError(t, err)

	db, mock := setupTestDB(t)
	mock.ExpectPing()

	mw := middleware.New(authSvc, nil, cfg, zap.NewNop())

	r := &Router{
		engine:     setupTestEngine(),
		db:         db,
		rdb:        nil,
		logger:     zap.NewNop(),
		middleware: mw,
	}

	// Register global middleware like the real router does
	r.engine.Use(mw.RecoveryMiddleware())
	r.engine.Use(mw.SecurityHeadersMiddleware())
	r.engine.Use(mw.RequestIDMiddleware())
	r.engine.Use(mw.CORSMiddleware())

	// Register health routes
	r.engine.GET("/health/live", r.livenessHandler)
	r.engine.GET("/health/ready", r.readinessHandler)

	t.Run("liveness with full middleware stack", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		w := httptest.NewRecorder()
		r.engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Security headers should be present
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.NotEmpty(t, w.Header().Get(middleware.RequestIDHeader))
	})

	t.Run("readiness with full middleware stack", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
		w := httptest.NewRecorder()
		r.engine.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp models.HealthCheckResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "ready", resp.Status)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
