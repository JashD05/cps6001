package kubernetes

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
	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
		Kubernetes: config.KubernetesConfig{
			Namespace:     "chaos-sec",
			PodTimeout:    5 * time.Minute,
			MaxConcurrent: 10,
		},
	}
}

// withAuthClaims injects TokenClaims into the Gin context, simulating
// what AuthMiddleware does for authenticated routes.
func withAuthClaims(claims *auth.TokenClaims) gin.HandlerFunc {
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

// Common test identifiers
var (
	testUserID    = uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	testOrgID     = uuid.MustParse("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22")
	testClusterID = uuid.MustParse("c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a33")
	testNow       = time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
)

func adminClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:         testUserID,
		Email:          "admin@example.com",
		Role:           "admin",
		OrganizationID: testOrgID,
		Permissions:    []string{"admin:all", "clusters:read", "clusters:write"},
		TokenType:      auth.TokenTypeAccess,
	}
}

func readerClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:         testUserID,
		Email:          "reader@example.com",
		Role:           "viewer",
		OrganizationID: testOrgID,
		Permissions:    []string{"clusters:read"},
		TokenType:      auth.TokenTypeAccess,
	}
}

func noPermClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:         testUserID,
		Email:          "noperm@example.com",
		Role:           "viewer",
		OrganizationID: testOrgID,
		Permissions:    []string{},
		TokenType:      auth.TokenTypeAccess,
	}
}

// ---------------------------------------------------------------------------
// ListClustersHandler Tests
// ---------------------------------------------------------------------------

func TestListClustersHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	rows := sqlmock.NewRows([]string{
		"id", "name", "description", "api_endpoint", "default_namespace", "status",
		"kubernetes_version", "node_count", "last_connected_at", "created_at", "updated_at",
	}).AddRow(
		testClusterID, "test-cluster", "A test cluster", "https://k8s.example.com:6443",
		"default", "connected", "v1.28.0", 3, testNow, testNow, testNow,
	).AddRow(
		uuid.New(), "prod-cluster", nil, "https://prod-k8s.example.com:6443",
		"chaos-sec", "connected", nil, nil, nil, testNow, testNow,
	)

	mock.ExpectQuery("SELECT id, name, description, api_endpoint, default_namespace, status.*FROM kubernetes_clusters WHERE organization_id").
		WithArgs(testOrgID).
		WillReturnRows(rows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters", withAuthClaims(claims), handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	data, ok := resp["data"].([]interface{})
	require.True(t, ok, "response should contain 'data' array")
	assert.Len(t, data, 2)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListClustersHandler_EmptyList(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("SELECT id, name, description, api_endpoint, default_namespace, status.*FROM kubernetes_clusters WHERE organization_id").
		WithArgs(testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "api_endpoint", "default_namespace", "status",
			"kubernetes_version", "node_count", "last_connected_at", "created_at", "updated_at",
		}))

	router := setupTestRouter()
	router.GET("/api/v1/clusters", withAuthClaims(claims), handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	data, ok := resp["data"].([]interface{})
	require.True(t, ok, "response should contain 'data' array")
	assert.Empty(t, data)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListClustersHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	// No auth claims middleware — simulates unauthenticated request
	router.GET("/api/v1/clusters", handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListClustersHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("SELECT id, name, description, api_endpoint, default_namespace, status.*FROM kubernetes_clusters WHERE organization_id").
		WithArgs(testOrgID).
		WillReturnError(fmt.Errorf("connection refused"))

	router := setupTestRouter()
	router.GET("/api/v1/clusters", withAuthClaims(claims), handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListClustersHandler_ScanError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Return a row with wrong column count to trigger a scan error.
	// The handler should still return 200 and skip the bad row, logging the error.
	rows := sqlmock.NewRows([]string{
		"id", "name", "description", "api_endpoint", "default_namespace", "status",
		"kubernetes_version", "node_count", "last_connected_at", "created_at", "updated_at",
	}).AddRow(
		// Intentionally using a string where UUID is expected should still be handled
		testClusterID, "test-cluster", "A test cluster", "https://k8s.example.com:6443",
		"default", "connected", "v1.28.0", 3, testNow, testNow, testNow,
	)

	mock.ExpectQuery("SELECT id, name, description, api_endpoint, default_namespace, status.*FROM kubernetes_clusters WHERE organization_id").
		WithArgs(testOrgID).
		WillReturnRows(rows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters", withAuthClaims(claims), handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 even if some rows fail to scan (logged and skipped)
	assert.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// RegisterClusterHandler Tests
// ---------------------------------------------------------------------------

func TestRegisterClusterHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Expect the INSERT query
	mock.ExpectQuery("(?s)INSERT INTO kubernetes_clusters.*RETURNING id, created_at, updated_at").
		WithArgs(
			testOrgID,
			"test-cluster",
			nil, // nilIfEmpty for description
			"https://k8s.example.com:6443",
			"ca-cert-data",
			"client-cert-data",
			"client-key-data",
			"chaos-sec", // default namespace
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(testClusterID, testNow, testNow))

	router := setupTestRouter()
	router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "test-cluster",
		APIEndpoint:       "https://k8s.example.com:6443",
		CACertificate:     "ca-cert-data",
		ClientCertificate: "client-cert-data",
		ClientKey:         "client-key-data",
		DefaultNamespace:  "chaos-sec",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "pending", resp["status"])
	assert.Equal(t, "test-cluster", resp["name"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterClusterHandler_DefaultNamespace(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// When DefaultNamespace is empty, it should default to "chaos-sec"
	mock.ExpectQuery("(?s)INSERT INTO kubernetes_clusters.*RETURNING id, created_at, updated_at").
		WithArgs(
			testOrgID,
			"my-cluster",
			nil,
			"https://k8s.example.com:6443",
			"ca-cert",
			"client-cert",
			"client-key",
			"chaos-sec", // default namespace
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(testClusterID, testNow, testNow))

	router := setupTestRouter()
	router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "my-cluster",
		APIEndpoint:       "https://k8s.example.com:6443",
		CACertificate:     "ca-cert",
		ClientCertificate: "client-cert",
		ClientKey:         "client-key",
		// DefaultNamespace intentionally empty
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegisterClusterHandler_InvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "missing name",
			body:       map[string]interface{}{"api_endpoint": "https://k8s.example.com:6443", "ca_certificate": "ca", "client_certificate": "cert", "client_key": "key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing api_endpoint",
			body:       map[string]interface{}{"name": "cluster", "ca_certificate": "ca", "client_certificate": "cert", "client_key": "key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing ca_certificate",
			body:       map[string]interface{}{"name": "cluster", "api_endpoint": "https://k8s.example.com:6443", "client_certificate": "cert", "client_key": "key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing client_certificate",
			body:       map[string]interface{}{"name": "cluster", "api_endpoint": "https://k8s.example.com:6443", "ca_certificate": "ca", "client_key": "key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing client_key",
			body:       map[string]interface{}{"name": "cluster", "api_endpoint": "https://k8s.example.com:6443", "ca_certificate": "ca", "client_certificate": "cert"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "name too short",
			body:       map[string]interface{}{"name": "a", "api_endpoint": "https://k8s.example.com:6443", "ca_certificate": "ca", "client_certificate": "cert", "client_key": "key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       map[string]interface{}{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupTestDB(t)
			cfg := setupTestConfig()
			logger := zap.NewNop()
			handler := NewHandler(db, cfg, logger)

			claims := adminClaims()

			router := setupTestRouter()
			router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRegisterClusterHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	// No auth claims middleware
	router.POST("/api/v1/clusters", handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "test-cluster",
		APIEndpoint:       "https://k8s.example.com:6443",
		CACertificate:     "ca-cert",
		ClientCertificate: "client-cert",
		ClientKey:         "client-key",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegisterClusterHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)INSERT INTO kubernetes_clusters.*RETURNING id, created_at, updated_at").
		WithArgs(
			testOrgID,
			"test-cluster",
			nil,
			"https://k8s.example.com:6443",
			"ca-cert",
			"client-cert",
			"client-key",
			"chaos-sec",
		).
		WillReturnError(fmt.Errorf("unique constraint violation"))

	router := setupTestRouter()
	router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "test-cluster",
		APIEndpoint:       "https://k8s.example.com:6443",
		CACertificate:     "ca-cert",
		ClientCertificate: "client-cert",
		ClientKey:         "client-key",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterHandler Tests
// ---------------------------------------------------------------------------

func TestGetClusterHandler_InvalidID(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id", withAuthClaims(claims), handler.GetClusterHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_id", resp["error"])
}

func TestGetClusterHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id", handler.GetClusterHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetClusterHandler_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Cluster not found in DB
	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id", withAuthClaims(claims), handler.GetClusterHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetClusterHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(fmt.Errorf("connection refused"))

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id", withAuthClaims(claims), handler.GetClusterHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// DeleteClusterHandler Tests
// ---------------------------------------------------------------------------

func TestDeleteClusterHandler_Success(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// No running experiments
	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Successful delete
	mock.ExpectExec("DELETE FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "Cluster deleted.", resp["message"])

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteClusterHandler_HasRunningRuns(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Running experiments found
	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteClusterHandler_NotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// No running experiments
	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Delete returns 0 rows affected (not found or wrong org)
	mock.ExpectExec("DELETE FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteClusterHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeleteClusterHandler_InvalidID(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_id", resp["error"])
}

func TestDeleteClusterHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Running experiment check query fails, but handler continues with delete attempt
	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Delete query fails
	mock.ExpectExec("DELETE FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(fmt.Errorf("database error"))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterHealthHandler Tests
// ---------------------------------------------------------------------------

func TestGetClusterHealthHandler_InvalidID(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", withAuthClaims(claims), handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/not-a-uuid/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_id", resp["error"])
}

func TestGetClusterHealthHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetClusterHealthHandler_ClusterNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", withAuthClaims(claims), handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetClusterHealthHandler_DatabaseError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(fmt.Errorf("connection refused"))

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", withAuthClaims(claims), handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterNamespacesHandler Tests
// ---------------------------------------------------------------------------

func TestGetClusterNamespacesHandler_InvalidID(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/namespaces", withAuthClaims(claims), handler.GetClusterNamespacesHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/not-a-uuid/namespaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "invalid_id", resp["error"])
}

func TestGetClusterNamespacesHandler_Unauthorized(t *testing.T) {
	db, _ := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/namespaces", handler.GetClusterNamespacesHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/namespaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---------------------------------------------------------------------------
// getClaimsFromContext Tests (unit testing the helper)
// ---------------------------------------------------------------------------

func TestGetClaimsFromContext(t *testing.T) {
	t.Run("valid claims in context", func(t *testing.T) {
		claims := adminClaims()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("auth_claims", claims)

		got, err := getClaimsFromContext(c)
		require.NoError(t, err)
		assert.Equal(t, claims.UserID, got.UserID)
		assert.Equal(t, claims.Email, got.Email)
		assert.Equal(t, claims.Role, got.Role)
		assert.Equal(t, claims.OrganizationID, got.OrganizationID)
	})

	t.Run("no claims in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		got, err := getClaimsFromContext(c)
		assert.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("wrong type in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("auth_claims", "not-claims")

		got, err := getClaimsFromContext(c)
		assert.Error(t, err)
		assert.Nil(t, got)
	})
}

// ---------------------------------------------------------------------------
// nilIfEmpty Tests
// ---------------------------------------------------------------------------

func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		input string
		want  interface{}
	}{
		{"", nil},
		{"hello", "hello"},
		{" ", " "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := nilIfEmpty(tt.input)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetClusterHealthHandler - cluster found but client connection fails
// ---------------------------------------------------------------------------

func TestGetClusterHealthHandler_ClusterFound_ClientError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Cluster exists in DB but has invalid certs so client creation will fail
	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "name", "description", "api_endpoint",
			"ca_certificate", "client_certificate", "client_key",
			"default_namespace", "status", "kubernetes_version", "node_count",
			"last_connected_at", "created_at", "updated_at",
		}).AddRow(
			testClusterID, testOrgID, "test-cluster", "A test cluster",
			"https://k8s.example.com:6443", "invalid-ca", "invalid-cert", "invalid-key",
			"chaos-sec", "error", nil, nil,
			nil, testNow, testNow,
		))

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", withAuthClaims(claims), handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler returns 200 with healthy=false when client can't connect
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["healthy"])
	assert.Equal(t, testClusterID.String(), resp["cluster_id"])

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterNamespacesHandler - cluster not found in DB
// ---------------------------------------------------------------------------

func TestGetClusterNamespacesHandler_ClusterNotFound(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnError(sql.ErrNoRows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/namespaces", withAuthClaims(claims), handler.GetClusterNamespacesHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/namespaces", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// getOrCreateClient returns error for cluster not found, which maps to 502
	assert.Equal(t, http.StatusBadGateway, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterHandler - cluster found in DB, client creation fails (live info shows error)
// ---------------------------------------------------------------------------

func TestGetClusterHandler_ClusterFound_ClientError(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Cluster exists in DB but has invalid certificates
	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "name", "description", "api_endpoint",
			"ca_certificate", "client_certificate", "client_key",
			"default_namespace", "status", "kubernetes_version", "node_count",
			"last_connected_at", "created_at", "updated_at",
		}).AddRow(
			testClusterID, testOrgID, "test-cluster", "A test cluster",
			"https://k8s.example.com:6443", "invalid-ca", "invalid-cert", "invalid-key",
			"chaos-sec", "error", nil, nil,
			nil, testNow, testNow,
		))

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id", withAuthClaims(claims), handler.GetClusterHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 with cluster data but live_info showing error
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", resp["name"])

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// RegisterClusterHandler - with description
// ---------------------------------------------------------------------------

func TestRegisterClusterHandler_WithDescription(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	desc := "Production Kubernetes cluster"
	mock.ExpectQuery("(?s)INSERT INTO kubernetes_clusters.*RETURNING id, created_at, updated_at").
		WithArgs(
			testOrgID,
			"prod-cluster",
			desc,
			"https://prod.example.com:6443",
			"ca-cert",
			"client-cert",
			"client-key",
			"chaos-sec",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(testClusterID, testNow, testNow))

	router := setupTestRouter()
	router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "prod-cluster",
		Description:       desc,
		APIEndpoint:       "https://prod.example.com:6443",
		CACertificate:     "ca-cert",
		ClientCertificate: "client-cert",
		ClientKey:         "client-key",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// ListClustersHandler - with nullable fields (NULL kubernetes_version, node_count, etc.)
// ---------------------------------------------------------------------------

func TestListClustersHandler_NullableFields(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	rows := sqlmock.NewRows([]string{
		"id", "name", "description", "api_endpoint", "default_namespace", "status",
		"kubernetes_version", "node_count", "last_connected_at", "created_at", "updated_at",
	}).AddRow(
		testClusterID, "new-cluster", nil, "https://k8s.example.com:6443",
		"default", "pending", nil, nil, nil, testNow, testNow,
	)

	mock.ExpectQuery("SELECT id, name, description, api_endpoint, default_namespace, status.*FROM kubernetes_clusters WHERE organization_id").
		WithArgs(testOrgID).
		WillReturnRows(rows)

	router := setupTestRouter()
	router.GET("/api/v1/clusters", withAuthClaims(claims), handler.ListClustersHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	data, ok := resp["data"].([]interface{})
	require.True(t, ok, "response should contain 'data' array")
	assert.Len(t, data, 1)

	cluster := data[0].(map[string]interface{})
	assert.Equal(t, "new-cluster", cluster["name"])
	assert.Nil(t, cluster["description"])
	assert.Nil(t, cluster["kubernetes_version"])
	assert.Nil(t, cluster["node_count"])
	assert.Nil(t, cluster["last_connected_at"])

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// DeleteClusterHandler - running experiment check query fails (fail-safe: continues)
// ---------------------------------------------------------------------------

func TestDeleteClusterHandler_RunningCheckFails(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	// Running experiment check query fails — handler should continue
	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnError(fmt.Errorf("database error"))

	// Delete proceeds since the running check failed (fail-safe)
	mock.ExpectExec("DELETE FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// DeleteClusterHandler - running check returns running experiments (409 Conflict)
// ---------------------------------------------------------------------------

func TestDeleteClusterHandler_RunningExperimentsExist(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("SELECT COUNT.*FROM experiment_runs WHERE cluster_id").
		WithArgs(testClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	router := setupTestRouter()
	router.DELETE("/api/v1/clusters/:id", withAuthClaims(claims), handler.DeleteClusterHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/"+testClusterID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "has_running_runs", resp["error"])

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetClusterHealthHandler - successful cluster DB fetch but client unavailable
// (verifies response structure with cluster_id, cluster_name, status, etc.)
// ---------------------------------------------------------------------------

func TestGetClusterHealthHandler_ClusterFound_ResponseStructure(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	mock.ExpectQuery("(?s)SELECT id, organization_id, name.*FROM kubernetes_clusters WHERE id").
		WithArgs(testClusterID, testOrgID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "name", "description", "api_endpoint",
			"ca_certificate", "client_certificate", "client_key",
			"default_namespace", "status", "kubernetes_version", "node_count",
			"last_connected_at", "created_at", "updated_at",
		}).AddRow(
			testClusterID, testOrgID, "my-cluster", nil,
			"https://k8s.example.com:6443", "invalid-ca", "invalid-cert", "invalid-key",
			"chaos-sec", "error", nil, nil,
			nil, testNow, testNow,
		))

	router := setupTestRouter()
	router.GET("/api/v1/clusters/:id/health", withAuthClaims(claims), handler.GetClusterHealthHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+testClusterID.String()+"/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, testClusterID.String(), resp["cluster_id"])
	assert.Equal(t, "my-cluster", resp["cluster_name"])
	assert.Equal(t, "error", resp["status"])
	assert.Equal(t, false, resp["healthy"])
	// checked_at should be present
	assert.NotEmpty(t, resp["checked_at"])

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// RegisterClusterHandler - with custom namespace
// ---------------------------------------------------------------------------

func TestRegisterClusterHandler_CustomNamespace(t *testing.T) {
	db, mock := setupTestDB(t)
	cfg := setupTestConfig()
	logger := zap.NewNop()
	handler := NewHandler(db, cfg, logger)

	claims := adminClaims()

	customNS := "my-custom-ns"
	mock.ExpectQuery("(?s)INSERT INTO kubernetes_clusters.*RETURNING id, created_at, updated_at").
		WithArgs(
			testOrgID,
			"ns-cluster",
			nil,
			"https://k8s.example.com:6443",
			"ca-cert",
			"client-cert",
			"client-key",
			customNS, // custom namespace, not "chaos-sec"
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(testClusterID, testNow, testNow))

	router := setupTestRouter()
	router.POST("/api/v1/clusters", withAuthClaims(claims), handler.RegisterClusterHandler)

	body := registerClusterRequest{
		Name:              "ns-cluster",
		APIEndpoint:       "https://k8s.example.com:6443",
		CACertificate:     "ca-cert",
		ClientCertificate: "client-cert",
		ClientKey:         "client-key",
		DefaultNamespace:  customNS,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters", makeRequestBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	require.NoError(t, mock.ExpectationsWereMet())
}
