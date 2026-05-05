package auth

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/chaos-sec/backend/internal/config"
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

func setupTestDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err, "failed to create sqlmock")
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func setupTestConfig() *config.Config {
	return &config.Config{
		Env: "development",
		JWT: config.JWTConfig{
			Secret:        "test-jwt-secret-for-unit-tests-32chars",
			Expiry:        1 * time.Hour,
			RefreshExpiry: 7 * 24 * time.Hour,
			Issuer:        "chaos-sec-test",
		},
	}
}

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// withAuthClaims injects TokenClaims into the Gin context, simulating
// what AuthMiddleware does for authenticated routes.
func withAuthClaims(claims *TokenClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("auth_claims", claims)
		c.Next()
	}
}

func makeRequestBody(t *testing.T, v interface{}) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

// Common test identifiers (deterministic UUIDs for reproducibility).
var (
	testUserID      = uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	testOrgID       = uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22")
	testRoleID      = uuid.MustParse("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a33")
	testOtherOrgID  = uuid.MustParse("d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a44")
	testEmail       = "test@example.com"
	testName        = "Test User"
	testPassword    = "securepassword123"
	testRoleName    = "admin"
	testPermissions = `["admin:all","experiments:read"]`
	testNow         = time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
)

// loginQueryRows builds a sqlmock row set matching the LoginHandler SELECT scan order.
func loginQueryRows(passwordHash string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "email", "password_hash", "name", "organization_id",
		"role_id", "is_active", "last_login_at", "created_at", "updated_at",
		"role_name", "permissions",
	}).AddRow(
		testUserID.String(), testEmail, passwordHash, testName,
		testOrgID.String(), testRoleID.String(), true,
		testNow, testNow, testNow,
		testRoleName, []byte(testPermissions),
	)
}

// meQueryRows builds a sqlmock row set matching the MeHandler SELECT scan order.
func meQueryRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "email", "password_hash", "name", "organization_id",
		"role_id", "is_active", "last_login_at", "created_at", "updated_at",
		"role_name", "role_description", "permissions",
	}).AddRow(
		testUserID.String(), testEmail, "$2a$10$hashhash", testName,
		testOrgID.String(), testRoleID.String(), true,
		testNow, testNow, testNow,
		testRoleName, "Administrator", []byte(testPermissions),
	)
}

// userByIDRows builds a sqlmock row set matching the RefreshHandler user-data SELECT.
func userByIDRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "email", "password_hash", "name", "organization_id",
		"role_id", "is_active", "last_login_at", "created_at", "updated_at",
		"role_name", "permissions",
	}).AddRow(
		testUserID.String(), testEmail, "$2a$10$hashhash", testName,
		testOrgID.String(), testRoleID.String(), true,
		testNow, testNow, testNow,
		testRoleName, []byte(testPermissions),
	)
}

// ---------------------------------------------------------------------------
// LoginHandler Tests
// ---------------------------------------------------------------------------

func TestLoginHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	passwordHash, err := HashPassword(testPassword)
	require.NoError(t, err)

	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.email").
		WithArgs(testEmail).
		WillReturnRows(loginQueryRows(passwordHash))

	mock.ExpectExec("UPDATE users SET last_login_at").
		WithArgs(testUserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := setupTestRouter()
	router.POST("/api/v1/auth/login", handler.LoginHandler)

	body := models.LoginRequest{Email: testEmail, Password: testPassword}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.TokenResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken, "access token should be present")
	assert.NotEmpty(t, resp.RefreshToken, "refresh token should be present")
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Greater(t, resp.ExpiresIn, int64(0))

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoginHandler_UserNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.email").
		WithArgs(testEmail).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.POST("/api/v1/auth/login", handler.LoginHandler)

	body := models.LoginRequest{Email: testEmail, Password: testPassword}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoginHandler_WrongPassword(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	passwordHash, err := HashPassword(testPassword)
	require.NoError(t, err)

	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.email").
		WithArgs(testEmail).
		WillReturnRows(loginQueryRows(passwordHash))

	router := setupTestRouter()
	router.POST("/api/v1/auth/login", handler.LoginHandler)

	body := models.LoginRequest{Email: testEmail, Password: "wrongpassword123"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoginHandler_InactiveAccount(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	passwordHash, err := HashPassword(testPassword)
	require.NoError(t, err)

	inactiveRows := sqlmock.NewRows([]string{
		"id", "email", "password_hash", "name", "organization_id",
		"role_id", "is_active", "last_login_at", "created_at", "updated_at",
		"role_name", "permissions",
	}).AddRow(
		testUserID.String(), testEmail, passwordHash, testName,
		testOrgID.String(), testRoleID.String(), false, // account inactive
		testNow, testNow, testNow,
		testRoleName, []byte(testPermissions),
	)

	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.email").
		WithArgs(testEmail).
		WillReturnRows(inactiveRows)

	router := setupTestRouter()
	router.POST("/api/v1/auth/login", handler.LoginHandler)

	body := models.LoginRequest{Email: testEmail, Password: testPassword}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "account_disabled", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoginHandler_MissingFields(t *testing.T) {
	tests := []struct {
		name        string
		body        map[string]interface{}
		wantStatus  int
		wantErrCode string
	}{
		{
			name:        "missing email",
			body:        map[string]interface{}{"password": "securepassword123"},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_request",
		},
		{
			name:        "missing password",
			body:        map[string]interface{}{"email": "test@example.com"},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_request",
		},
		{
			name:        "empty body",
			body:        map[string]interface{}{},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_request",
		},
		{
			name:        "password too short",
			body:        map[string]interface{}{"email": "test@example.com", "password": "short"},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_request",
		},
		{
			name:        "invalid email format",
			body:        map[string]interface{}{"email": "not-an-email", "password": "securepassword123"},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupTestDB(t)
			cfg := setupTestConfig()
			logger := zap.NewNop()

			handler, err := NewHandler(db, nil, cfg, logger)
			require.NoError(t, err)

			router := setupTestRouter()
			router.POST("/api/v1/auth/login", handler.LoginHandler)

			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp models.ErrorResponse
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.wantErrCode, resp.Error)
		})
	}
}

func TestLoginHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.email").
		WithArgs(testEmail).
		WillReturnError(fmt.Errorf("connection refused"))

	router := setupTestRouter()
	router.POST("/api/v1/auth/login", handler.LoginHandler)

	body := models.LoginRequest{Email: testEmail, Password: testPassword}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "internal_error", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// RegisterHandler Tests
// ---------------------------------------------------------------------------

func TestRegisterHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	adminClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	// 1. Check organization exists and is active
	mock.ExpectQuery("SELECT EXISTS.*FROM organizations WHERE id").
		WithArgs(testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 2. Check role exists
	mock.ExpectQuery("SELECT EXISTS.*FROM roles WHERE id").
		WithArgs(testRoleID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// 3. Check email is not taken
	mock.ExpectQuery("SELECT EXISTS.*FROM users WHERE email").
		WithArgs("newuser@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	// 4. Insert user and return created record
	newUserID := uuid.New()
	mock.ExpectQuery("(?s)INSERT INTO users.*RETURNING id").
		WithArgs("newuser@example.com", sqlmock.AnyArg(), "New User", testOrgID, testRoleID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password_hash", "name", "organization_id",
			"role_id", "is_active", "last_login_at", "created_at", "updated_at",
		}).AddRow(
			newUserID.String(), "newuser@example.com", "$2a$10$somehash", "New User",
			testOrgID.String(), testRoleID.String(), true,
			nil, testNow, testNow,
		))

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(adminClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp models.UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "newuser@example.com", resp.Email)
	assert.Equal(t, "New User", resp.Name)
	assert.True(t, resp.IsActive)
	assert.Equal(t, testOrgID, resp.OrganizationID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterHandler_DuplicateEmail(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	adminClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	mock.ExpectQuery("SELECT EXISTS.*FROM organizations WHERE id").
		WithArgs(testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	mock.ExpectQuery("SELECT EXISTS.*FROM roles WHERE id").
		WithArgs(testRoleID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Email is already taken
	mock.ExpectQuery("SELECT EXISTS.*FROM users WHERE email").
		WithArgs("existing@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(adminClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "existing@example.com",
		Password:       "securepassword123",
		Name:           "Existing User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "email_exists", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterHandler_MissingFields(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "missing email",
			body:       map[string]interface{}{"password": "securepassword123", "name": "User", "organization_id": testOrgID.String(), "role_id": testRoleID.String()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing password",
			body:       map[string]interface{}{"email": "user@example.com", "name": "User", "organization_id": testOrgID.String(), "role_id": testRoleID.String()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing name",
			body:       map[string]interface{}{"email": "user@example.com", "password": "securepassword123", "organization_id": testOrgID.String(), "role_id": testRoleID.String()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "password too short",
			body:       map[string]interface{}{"email": "user@example.com", "password": "short", "name": "User", "organization_id": testOrgID.String(), "role_id": testRoleID.String()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing organization_id",
			body:       map[string]interface{}{"email": "user@example.com", "password": "securepassword123", "name": "User", "role_id": testRoleID.String()},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing role_id",
			body:       map[string]interface{}{"email": "user@example.com", "password": "securepassword123", "name": "User", "organization_id": testOrgID.String()},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupTestDB(t)
			cfg := setupTestConfig()
			logger := zap.NewNop()

			handler, err := NewHandler(db, nil, cfg, logger)
			require.NoError(t, err)

			adminClaims := &TokenClaims{
				UserID:         testUserID,
				Email:          testEmail,
				Role:           "admin",
				OrganizationID: testOrgID,
				Permissions:    []string{"admin:all"},
				TokenType:      TokenTypeAccess,
			}

			router := setupTestRouter()
			router.POST("/api/v1/auth/register", withAuthClaims(adminClaims), handler.RegisterHandler)

			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRegisterHandler_NoPermission(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	viewerClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "viewer",
		OrganizationID: testOrgID,
		Permissions:    []string{"experiments:read"}, // no admin:all or users:manage
		TokenType:      TokenTypeAccess,
	}

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(viewerClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "forbidden", resp.Error)
}

func TestRegisterHandler_OrganizationNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	adminClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	// Organization does not exist or is inactive
	mock.ExpectQuery("SELECT EXISTS.*FROM organizations WHERE id").
		WithArgs(testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(adminClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_organization", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterHandler_RoleNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	adminClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	mock.ExpectQuery("SELECT EXISTS.*FROM organizations WHERE id").
		WithArgs(testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Role does not exist
	mock.ExpectQuery("SELECT EXISTS.*FROM roles WHERE id").
		WithArgs(testRoleID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(adminClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_role", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterHandler_CrossOrgCreation(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	// Non-admin with users:manage in their own org, trying to create in a different org
	operatorClaims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "operator",
		OrganizationID: testOrgID,
		Permissions:    []string{"users:manage"},
		TokenType:      TokenTypeAccess,
	}

	// Organization exists check passes
	mock.ExpectQuery("SELECT EXISTS.*FROM organizations WHERE id").
		WithArgs(testOtherOrgID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Role exists check passes
	mock.ExpectQuery("SELECT EXISTS.*FROM roles WHERE id").
		WithArgs(testRoleID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	router := setupTestRouter()
	router.POST("/api/v1/auth/register", withAuthClaims(operatorClaims), handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOtherOrgID.String(), // different from claims' org
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "forbidden", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterHandler_Unauthorized_NoClaims(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	// Route without auth claims middleware — simulates unauthenticated request
	router.POST("/api/v1/auth/register", handler.RegisterHandler)

	body := models.RegisterRequest{
		Email:          "newuser@example.com",
		Password:       "securepassword123",
		Name:           "New User",
		OrganizationID: testOrgID.String(),
		RoleID:         testRoleID.String(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)
}

// ---------------------------------------------------------------------------
// RefreshHandler Tests
// ---------------------------------------------------------------------------

func TestRefreshHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	// Generate a valid refresh token using the same JWT config as the handler
	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	refreshToken, _, err := authSvc.GenerateRefreshToken(testUserID, testEmail, testRoleName, testOrgID)
	require.NoError(t, err)

	// 1. Check user is still active
	mock.ExpectQuery("SELECT is_active FROM users WHERE id").
		WithArgs(testUserID).
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(true))

	// 2. Fetch fresh user data for new token generation
	mock.ExpectQuery("(?s)SELECT u\\.id.*FROM users u JOIN roles r.*WHERE u\\.id").
		WithArgs(testUserID).
		WillReturnRows(userByIDRows())

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: refreshToken}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.TokenResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken, "new access token should be present")
	assert.NotEmpty(t, resp.RefreshToken, "new refresh token should be present")
	assert.Equal(t, "Bearer", resp.TokenType)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefreshHandler_InvalidToken(t *testing.T) {
	db, _ := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: "this-is-not-a-valid-jwt"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_token", resp.Error)
}

func TestRefreshHandler_MissingToken(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_request", resp.Error)
}

func TestRefreshHandler_UserInactive(t *testing.T) {
	db, mock := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	refreshToken, _, err := authSvc.GenerateRefreshToken(testUserID, testEmail, testRoleName, testOrgID)
	require.NoError(t, err)

	// User account is disabled
	mock.ExpectQuery("SELECT is_active FROM users WHERE id").
		WithArgs(testUserID).
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(false))

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: refreshToken}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "account_disabled", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefreshHandler_UserNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	refreshToken, _, err := authSvc.GenerateRefreshToken(testUserID, testEmail, testRoleName, testOrgID)
	require.NoError(t, err)

	// User no longer exists
	mock.ExpectQuery("SELECT is_active FROM users WHERE id").
		WithArgs(testUserID).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: refreshToken}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "user_not_found", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefreshHandler_AccessTokenRejected(t *testing.T) {
	db, _ := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	// Generate an ACCESS token instead of a refresh token
	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	accessToken, _, err := authSvc.GenerateAccessToken(testUserID, testEmail, testRoleName, testOrgID, []string{"admin:all"})
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: accessToken}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Access tokens must be rejected by ValidateRefreshToken
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_token", resp.Error)
}

func TestRefreshHandler_BlacklistedToken(t *testing.T) {
	db, _ := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	refreshToken, refreshClaims, err := authSvc.GenerateRefreshToken(testUserID, testEmail, testRoleName, testOrgID)
	require.NoError(t, err)

	// Blacklist the token in Redis before the request
	blacklistKey := fmt.Sprintf("token:blacklist:%s", refreshClaims.ID)
	rdb.Set(nil, blacklistKey, "1", time.Hour)

	router := setupTestRouter()
	router.POST("/api/v1/auth/refresh", handler.RefreshHandler)

	body := models.RefreshRequest{RefreshToken: refreshToken}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "token_revoked", resp.Error)
}

// ---------------------------------------------------------------------------
// MeHandler Tests
// ---------------------------------------------------------------------------

func TestMeHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	claims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	mock.ExpectQuery("(?s)SELECT u\\.id.*r\\.description.*FROM users u JOIN roles r.*WHERE u\\.id").
		WithArgs(testUserID).
		WillReturnRows(meQueryRows())

	router := setupTestRouter()
	router.GET("/api/v1/auth/me", withAuthClaims(claims), handler.MeHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, testEmail, resp.Email)
	assert.Equal(t, testName, resp.Name)
	assert.Equal(t, testOrgID, resp.OrganizationID)
	assert.True(t, resp.IsActive)
	assert.NotNil(t, resp.Role)
	assert.Equal(t, testRoleName, resp.Role.Name)
	assert.Equal(t, "Administrator", resp.Role.Description)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMeHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	// No auth claims middleware — simulates unauthenticated request
	router.GET("/api/v1/auth/me", handler.MeHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)
}

func TestMeHandler_UserNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	claims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	mock.ExpectQuery("(?s)SELECT u\\.id.*r\\.description.*FROM users u JOIN roles r.*WHERE u\\.id").
		WithArgs(testUserID).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.GET("/api/v1/auth/me", withAuthClaims(claims), handler.MeHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "user_not_found", resp.Error)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMeHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	claims := &TokenClaims{
		UserID:         testUserID,
		Email:          testEmail,
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all"},
		TokenType:      TokenTypeAccess,
	}

	mock.ExpectQuery("(?s)SELECT u\\.id.*r\\.description.*FROM users u JOIN roles r.*WHERE u\\.id").
		WithArgs(testUserID).
		WillReturnError(fmt.Errorf("connection pool exhausted"))

	router := setupTestRouter()
	router.GET("/api/v1/auth/me", withAuthClaims(claims), handler.MeHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// LogoutHandler Tests
// ---------------------------------------------------------------------------

func TestLogoutHandler_Success(t *testing.T) {
	db, _ := setupTestDB(t)
	mr, rdb := setupMiniredis(t)
	_ = mr
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, rdb, cfg, logger)
	require.NoError(t, err)

	// Generate a valid access token
	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	accessToken, _, err := authSvc.GenerateAccessToken(testUserID, testEmail, testRoleName, testOrgID, []string{"admin:all"})
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/logout", handler.LogoutHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "logged out", resp["message"])
}

func TestLogoutHandler_InvalidToken(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/logout", handler.LogoutHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Already invalid/expired tokens still return 200 (nothing to blacklist)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "logged out", resp["message"])
}

func TestLogoutHandler_MissingAuthHeader(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/logout", handler.LogoutHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp models.ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "missing_token", resp.Error)
}

func TestLogoutHandler_MalformedAuthHeader(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		wantErrCode   string
	}{
		{
			name:        "basic auth instead of bearer",
			authHeader:  "Basic dXNlcjpwYXNz",
			wantErrCode: "invalid_token_format",
		},
		{
			name:        "bearer without space",
			authHeader:  "Bearertoken-no-space",
			wantErrCode: "invalid_token_format",
		},
		{
			name:        "empty bearer",
			authHeader:  "Bearer ",
			wantErrCode: "invalid_token_format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupTestDB(t)
			cfg := setupTestConfig()
			logger := zap.NewNop()

			handler, err := NewHandler(db, nil, cfg, logger)
			require.NoError(t, err)

			router := setupTestRouter()
			router.POST("/api/v1/auth/logout", handler.LogoutHandler)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp models.ErrorResponse
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.wantErrCode, resp.Error)
		})
	}
}

func TestLogoutHandler_NilRedis(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()

	// Handler with nil Redis client — should still succeed (no blacklist to set)
	handler, err := NewHandler(db, nil, cfg, logger)
	require.NoError(t, err)

	authSvc, err := New(&cfg.JWT)
	require.NoError(t, err)
	accessToken, _, err := authSvc.GenerateAccessToken(testUserID, testEmail, testRoleName, testOrgID, []string{"admin:all"})
	require.NoError(t, err)

	router := setupTestRouter()
	router.POST("/api/v1/auth/logout", handler.LogoutHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "logged out", resp["message"])
}
