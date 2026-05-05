package siem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestSplunkSIEM_ImplementsSIEMConnector(t *testing.T) {
	var connector SIEMConnector
	connector = &SplunkSIEM{}
	assert.NotNil(t, connector)
}

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNewSplunkSIEM_Defaults(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  10 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	assert.NotNil(t, s)
	assert.Equal(t, cfg, s.cfg)
	assert.NotNil(t, s.client)
	assert.Equal(t, 10*time.Second, s.client.Timeout)
	assert.Empty(t, s.sessionToken)
}

func TestNewSplunkSIEM_WithLogger(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	logger := zap.NewNop()
	s := NewSplunkSIEM(cfg, WithSplunkLogger(logger))

	assert.NotNil(t, s)
}

// ---------------------------------------------------------------------------
// Connect
// ---------------------------------------------------------------------------

func TestSplunkSIEM_Connect_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/services/auth/login", r.URL.Path)

		// Parse form data.
		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "admin", r.FormValue("username"))
		assert.Equal(t, "changeme", r.FormValue("password"))
		assert.Equal(t, "json", r.FormValue("output_mode"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"sessionKey":"splunk-session-token-12345"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "splunk-session-token-12345", s.sessionToken)
}

func TestSplunkSIEM_Connect_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"messages":[{"type":"ERROR","text":"Invalid credentials"}]}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "wrong",
		Password:   "wrong",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestSplunkSIEM_Connect_EmptySessionKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"sessionKey":""}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no session key")
}

func TestSplunkSIEM_Connect_Unreachable(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint:   "http://localhost:1",
		Timeout:    1 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	assert.Error(t, err)
}

func TestSplunkSIEM_Connect_ExistingValidSession(t *testing.T) {
	healthCheckCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"initial-token"}`)

		case "/services/server/info":
			healthCheckCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg)

	// First connect.
	err := s.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "initial-token", s.sessionToken)

	// Reset flag and connect again — should just validate session.
	healthCheckCalled = false
	err = s.Connect(context.Background())
	require.NoError(t, err)
	assert.True(t, healthCheckCalled, "second connect should validate existing session via health check")
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestSplunkSIEM_HealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/services/server/info", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("output_mode"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestSplunkSIEM_HealthCheck_FlatResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"isActive":true,"version":"9.0.0"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestSplunkSIEM_HealthCheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `Service Unavailable`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestSplunkSIEM_HealthCheck_Unreachable(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint:   "http://localhost:1",
		Timeout:    1 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// QueryAlerts
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_Success(t *testing.T) {
	expID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	runID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	sid := "search-job-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			// Create search job.
			assert.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			// Search job status — return DONE.
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			// Search results.
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "json", r.URL.Query().Get("output_mode"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			results := splunkSearchResponse{
				Results: []map[string]interface{}{
					{
						"_id":           "alert-001",
						"timestamp":     "2024-01-15T10:30:00Z",
						"severity":      "high",
						"type":          "network_intrusion",
						"source":        "firewall",
						"description":   "Suspicious traffic detected",
						"experiment_id": expID.String(),
						"run_id":        runID.String(),
						"risk_score":    85.0,
					},
				},
			}
			respBytes, _ := json.Marshal(results)
			w.Write(respBytes)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	// Connect first to get a session token.
	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC),
		},
		Severity:     "high",
		Source:       "firewall",
		AlertType:    "network_intrusion",
		ExperimentID: expID,
		RunID:        runID,
		Pagination: Pagination{
			Offset: 0,
			Limit:  50,
		},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, "alert-001", alerts[0].ID)
	assert.Equal(t, "high", alerts[0].Severity)
	assert.Equal(t, "network_intrusion", alerts[0].Type)
	assert.Equal(t, "firewall", alerts[0].Source)
	assert.Equal(t, "Suspicious traffic detected", alerts[0].Description)
	assert.Equal(t, expID, alerts[0].ExperimentID)
	assert.Equal(t, runID, alerts[0].RunID)
	assert.Equal(t, 85.0, alerts[0].Metadata["risk_score"])
}

func TestSplunkSIEM_QueryAlerts_NotConnected(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err := s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSplunkSIEM_QueryAlerts_EmptyResults(t *testing.T) {
	sid := "search-job-empty"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-24 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 100},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	assert.Empty(t, alerts)
}

func TestSplunkSIEM_QueryAlerts_SearchJobCreationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"messages":[{"type":"ERROR","text":"Invalid search"}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

// ---------------------------------------------------------------------------
// SendAlert
// ---------------------------------------------------------------------------

func TestSplunkSIEM_SendAlert_Success(t *testing.T) {
	alert := SIEMAlert{
		ID:          "alert-123",
		Timestamp:   time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
		Severity:    "critical",
		Type:        "privilege_escalation",
		Source:      "host-ids",
		Description: "Privilege escalation attempt detected",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/receivers/simple":
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "chaos-sec:alert", r.URL.Query().Get("sourcetype"))
			assert.Equal(t, "my-index", r.URL.Query().Get("index"))
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

			var receivedAlert SIEMAlert
			err := json.NewDecoder(r.Body).Decode(&receivedAlert)
			assert.NoError(t, err)
			assert.Equal(t, alert.ID, receivedAlert.ID)
			assert.Equal(t, alert.Severity, receivedAlert.Severity)
			assert.Equal(t, alert.Type, receivedAlert.Type)

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"text":"Event received"}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "my-index",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	err = s.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

func TestSplunkSIEM_SendAlert_NotConnected(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	alert := SIEMAlert{
		ID:        "alert-456",
		Timestamp: time.Now(),
		Severity:  "low",
		Type:      "test",
	}

	err := s.SendAlert(context.Background(), alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSplunkSIEM_SendAlert_ServerError(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-789",
		Timestamp: time.Now(),
		Severity:  "medium",
		Type:      "scan",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/receivers/simple":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"messages":[{"type":"ERROR","text":"Internal server error"}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	err = s.SendAlert(context.Background(), alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestSplunkSIEM_SendAlert_DefaultIndex(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-default-index",
		Timestamp: time.Now(),
		Severity:  "low",
		Type:      "test",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/receivers/simple":
			// When no index is configured, the index query param should not be set.
			assert.Empty(t, r.URL.Query().Get("index"))
			assert.Equal(t, "chaos-sec:alert", r.URL.Query().Get("sourcetype"))

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"text":"Event received"}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		// Index is intentionally not set.
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	err = s.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestSplunkSIEM_Close_WithSession(t *testing.T) {
	logoutCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/authentication/current-context/logout":
			logoutCalled = true
			assert.Equal(t, http.MethodPost, r.Method)
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Username: "admin",
		Password: "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, s.sessionToken)

	err = s.Close()
	assert.NoError(t, err)
	assert.Empty(t, s.sessionToken)
	assert.True(t, logoutCalled)
}

func TestSplunkSIEM_Close_WithoutSession(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	err := s.Close()
	assert.NoError(t, err)
	assert.Empty(t, s.sessionToken)
}

func TestSplunkSIEM_Close_LogoutFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/authentication/current-context/logout":
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Username: "admin",
		Password: "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	// Close should still succeed even if logout fails (it logs a warning).
	err = s.Close()
	assert.NoError(t, err)
	assert.Empty(t, s.sessionToken, "session token should be cleared even if logout fails")
}

// ---------------------------------------------------------------------------
// Auth headers
// ---------------------------------------------------------------------------

func TestSplunkSIEM_AuthHeaders_SessionToken(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")

		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"my-session-key"}`)

		case "/services/server/info":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	// Reset and call HealthCheck which should use the session token.
	capturedAuth = ""
	err = s.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-session-key", capturedAuth)
}

func TestSplunkSIEM_AuthHeaders_APIKeyFallback(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		APIKey:     "my-api-key",
	}
	s := NewSplunkSIEM(cfg)

	// Without a session token, should use API key.
	err := s.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-api-key", capturedAuth)
}

func TestSplunkSIEM_AuthHeaders_BasicAuthFallback(t *testing.T) {
	var capturedUser, capturedPass string
	var hasBasicAuth bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser, capturedPass, hasBasicAuth = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "splunkadmin",
		Password:   "splunkpass",
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, hasBasicAuth)
	assert.Equal(t, "splunkadmin", capturedUser)
	assert.Equal(t, "splunkpass", capturedPass)
}

// ---------------------------------------------------------------------------
// BuildSearchQuery
// ---------------------------------------------------------------------------

func TestSplunkSIEM_BuildSearchQuery_DefaultIndex(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
		},
	}

	result := s.buildSearchQuery(query)
	assert.Contains(t, result, "index=main")
}

func TestSplunkSIEM_BuildSearchQuery_CustomIndex(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
		Index:    "custom-alerts",
	}
	s := NewSplunkSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
		},
	}

	result := s.buildSearchQuery(query)
	assert.Contains(t, result, "index=custom-alerts")
}

func TestSplunkSIEM_BuildSearchQuery_WithFilters(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
		Index:    "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	expID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	runID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	query := AlertQuery{
		Severity:     "critical",
		Source:       "ids-sensor",
		AlertType:    "malware_detection",
		ExperimentID: expID,
		RunID:        runID,
	}

	result := s.buildSearchQuery(query)
	assert.Contains(t, result, "index=siem-alerts")
	assert.Contains(t, result, "severity=critical")
	assert.Contains(t, result, `source="ids-sensor"`)
	assert.Contains(t, result, `type="malware_detection"`)
	assert.Contains(t, result, fmt.Sprintf(`experiment_id="%s"`, expID.String()))
	assert.Contains(t, result, fmt.Sprintf(`run_id="%s"`, runID.String()))
}

func TestSplunkSIEM_BuildSearchQuery_TimeRange(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
		Index:    "main",
	}
	s := NewSplunkSIEM(cfg)

	from := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

	query := AlertQuery{
		TimeRange: TimeRange{From: from, To: to},
	}

	result := s.buildSearchQuery(query)
	assert.Contains(t, result, "earliest=")
	assert.Contains(t, result, "latest=")
}

func TestSplunkSIEM_BuildSearchQuery_NoFilters(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
		Index:    "main",
	}
	s := NewSplunkSIEM(cfg)

	query := AlertQuery{
		Pagination: Pagination{Limit: 100},
	}

	result := s.buildSearchQuery(query)
	// Should only contain index=main, no additional filters.
	assert.Contains(t, result, "index=main")
	// Should not contain any filter terms beyond the base.
	parts := strings.Split(result, " ")
	assert.Equal(t, 1, len(parts), "with no filters, query should just be index=main")
}

// ---------------------------------------------------------------------------
// ParseSplunkResults
// ---------------------------------------------------------------------------

func TestSplunkSIEM_ParseSplunkResults_FullResult(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	expID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	runID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	results := []map[string]interface{}{
		{
			"_id":           "alert-001",
			"timestamp":     "2024-01-15T10:30:00Z",
			"severity":      "high",
			"type":          "network_intrusion",
			"source":        "firewall",
			"description":   "Suspicious traffic detected",
			"experiment_id": expID.String(),
			"run_id":        runID.String(),
			"risk_score":    85.0,
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)

	assert.Equal(t, "alert-001", alerts[0].ID)
	assert.Equal(t, "high", alerts[0].Severity)
	assert.Equal(t, "network_intrusion", alerts[0].Type)
	assert.Equal(t, "firewall", alerts[0].Source)
	assert.Equal(t, "Suspicious traffic detected", alerts[0].Description)
	assert.Equal(t, expID, alerts[0].ExperimentID)
	assert.Equal(t, runID, alerts[0].RunID)
}

func TestSplunkSIEM_ParseSplunkResults_ArrayValues(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	// Splunk can return values as arrays for multi-value fields.
	results := []map[string]interface{}{
		{
			"_id":       []interface{}{"alert-arr"},
			"timestamp": []interface{}{"2024-02-20T15:00:00Z"},
			"severity":  []interface{}{"medium"},
			"type":      []interface{}{"scan"},
			"source":    []interface{}{"scanner"},
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)

	assert.Equal(t, "alert-arr", alerts[0].ID)
	assert.Equal(t, "medium", alerts[0].Severity)
	assert.Equal(t, "scan", alerts[0].Type)
	assert.Equal(t, "scanner", alerts[0].Source)
}

func TestSplunkSIEM_ParseSplunkResults_MinimalResult(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	results := []map[string]interface{}{
		{
			"_id": "alert-minimal",
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)

	assert.Equal(t, "alert-minimal", alerts[0].ID)
	assert.Empty(t, alerts[0].Severity)
	assert.Empty(t, alerts[0].Type)
}

func TestSplunkSIEM_ParseSplunkResults_EmptyResults(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	alerts := s.parseSplunkResults([]map[string]interface{}{})
	assert.Empty(t, alerts)
}

func TestSplunkSIEM_ParseSplunkResults_FallbackFields(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	results := []map[string]interface{}{
		{
			"id":         "fallback-id",
			"alert_type": "brute_force",
			"_source":    "internal-network",
			"_raw":       "Raw event log data for fallback description",
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)

	assert.Equal(t, "fallback-id", alerts[0].ID)
	assert.Equal(t, "brute_force", alerts[0].Type)
	assert.Equal(t, "internal-network", alerts[0].Source)
	assert.Equal(t, "Raw event log data for fallback description", alerts[0].Description)
}

func TestSplunkSIEM_ParseSplunkResults_MetadataExtraction(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	results := []map[string]interface{}{
		{
			"_id":          "alert-meta",
			"severity":     "low",
			"custom_field": "custom_value",
			"priority":     3,
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)

	assert.Equal(t, "alert-meta", alerts[0].ID)
	assert.Equal(t, "low", alerts[0].Severity)
	assert.Equal(t, "custom_value", alerts[0].Metadata["custom_field"])
	assert.Equal(t, 3, alerts[0].Metadata["priority"])
}

// ---------------------------------------------------------------------------
// Retry logic
// ---------------------------------------------------------------------------

func TestSplunkSIEM_Connect_RetrySuccess(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"sessionKey":"retried-token"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "retried-token", s.sessionToken)
}

func TestSplunkSIEM_Retry_Exhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	// Connect() uses withRetry, so retries will be attempted before giving up.
	err := s.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 2 retries")
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"sid":"search-123"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	// Set up session token so we pass the "not connected" check.
	s.sessionToken = "test-token"

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err := s.QueryAlerts(ctx, query)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewSIEMConnector factory integration
// ---------------------------------------------------------------------------

func TestNewSIEMConnector_Splunk(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}

	connector, err := NewSIEMConnector("splunk", cfg)
	require.NoError(t, err)
	assert.NotNil(t, connector)

	// Verify it's a SplunkSIEM.
	s, ok := connector.(*SplunkSIEM)
	assert.True(t, ok)
	assert.NotNil(t, s)
}

func TestNewSIEMConnector_Splunk_InvalidConfig(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "",
		Timeout:  5 * time.Second,
	}

	_, err := NewSIEMConnector("splunk", cfg)
	assert.Error(t, err)
}

func TestNewSIEMConnector_Mock(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9999",
		Timeout:  5 * time.Second,
	}

	connector, err := NewSIEMConnector("mock", cfg)
	require.NoError(t, err)
	assert.NotNil(t, connector)

	_, ok := connector.(*MockSIEM)
	assert.True(t, ok)
}

func TestNewSIEMConnector_EmptyProvider(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}

	_, err := NewSIEMConnector("", cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be specified")
}

// ---------------------------------------------------------------------------
// Splunk search job flow (create → wait → results) end-to-end
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_SearchJobFailed(t *testing.T) {
	sid := "failed-job-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"FAILED"}}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func TestSplunkSIEM_QueryAlerts_SearchJobFlatStatus(t *testing.T) {
	sid := "flat-job-456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			// Return flat (non-wrapped) format for status.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"dispatchState":"DONE"}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	assert.Empty(t, alerts)
}

func TestSplunkSIEM_QueryAlerts_SearchJobCreationReturnsEmptySID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":""}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	// After the createSearchJob refactor, an empty flat SID triggers the
	// wrapper path which returns "no entries" since {"sid":""} has no entry array.
	assert.Contains(t, err.Error(), "no entries")
}

// ---------------------------------------------------------------------------
// SplunkSIEM send alert with API key (no session token)
// ---------------------------------------------------------------------------

func TestSplunkSIEM_SendAlert_WithAPIKey(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-api-key",
		Timestamp: time.Now(),
		Severity:  "medium",
		Type:      "scan",
	}

	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"text":"Event received"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		APIKey:     "my-api-key-456",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)
	// Set session token directly to avoid needing the auth endpoint.
	s.sessionToken = "session-token-override"

	err := s.SendAlert(context.Background(), alert)
	require.NoError(t, err)
	// When session token is set, it takes priority over API key.
	assert.Equal(t, "Bearer session-token-override", capturedAuth)
}

// ---------------------------------------------------------------------------
// Verify search job request parameters
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_SearchJobParameters(t *testing.T) {
	sid := "param-test-job"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			// Verify request method and parameters.
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

			err := r.ParseForm()
			require.NoError(t, err)
			assert.Contains(t, r.FormValue("search"), "index=siem-alerts")
			assert.Equal(t, "json", r.FormValue("output_mode"))
			assert.Equal(t, "25", r.FormValue("max_count"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC),
		},
		Pagination: Pagination{
			Offset: 0,
			Limit:  25,
		},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Verify search job creation uses Splunk Atom/JSON wrapper format
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_WrappedSearchJobResponse(t *testing.T) {
	sid := "wrapped-job-789"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			// Splunk Atom/JSON wrapper format.
			fmt.Fprintf(w, `{"entry":[{"content":{"sid":"%s"}}]}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[{"_id":"wrapped-alert","severity":"low","type":"audit"}]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "wrapped-alert", alerts[0].ID)
	assert.Equal(t, "low", alerts[0].Severity)
	assert.Equal(t, "audit", alerts[0].Type)
}

// ---------------------------------------------------------------------------
// Verify search job creation returns no entries
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_SearchJobNoEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			// Return wrapped format with empty entries and invalid flat format.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"entry":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no entries")
}

// ---------------------------------------------------------------------------
// Verify SendAlert passes correct content type and index
// ---------------------------------------------------------------------------

func TestSplunkSIEM_SendAlert_VerifyRequestFormat(t *testing.T) {
	alert := SIEMAlert{
		ID:          "alert-format-test",
		Timestamp:   time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		Severity:    "high",
		Type:        "intrusion",
		Source:      "ids",
		Description: "Test alert for format verification",
	}

	var capturedBody SIEMAlert
	var capturedSourcetype string
	var capturedIndex string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/receivers/simple":
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

			capturedSourcetype = r.URL.Query().Get("sourcetype")
			capturedIndex = r.URL.Query().Get("index")

			err := json.NewDecoder(r.Body).Decode(&capturedBody)
			require.NoError(t, err)

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"text":"Event received"}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "production-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	err = s.SendAlert(context.Background(), alert)
	require.NoError(t, err)

	assert.Equal(t, "chaos-sec:alert", capturedSourcetype)
	assert.Equal(t, "production-alerts", capturedIndex)
	assert.Equal(t, alert.ID, capturedBody.ID)
	assert.Equal(t, alert.Severity, capturedBody.Severity)
	assert.Equal(t, alert.Type, capturedBody.Type)
	assert.Equal(t, alert.Source, capturedBody.Source)
	assert.Equal(t, alert.Description, capturedBody.Description)
}

// ---------------------------------------------------------------------------
// Verify Connect re-authenticates when existing session is invalid
// ---------------------------------------------------------------------------

func TestSplunkSIEM_Connect_ReauthenticationOnInvalidSession(t *testing.T) {
	authCallCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			authCallCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"new-session-token-%d"}`, authCallCount)

		case "/services/server/info":
			// First health check fails (session expired), second succeeds.
			if r.Header.Get("Authorization") == "Bearer new-session-token-1" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"entry":[{"content":{"isActive":true,"version":"9.0.0"}}]}`)
			} else {
				// Old/expired token returns 401.
				w.WriteHeader(http.StatusUnauthorized)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	// First connect — gets session token.
	err := s.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "new-session-token-1", s.sessionToken)
	assert.Equal(t, 1, authCallCount)

	// Set an invalid session token to simulate expiration.
	s.sessionToken = "expired-token"

	// Second connect — should detect invalid session and re-authenticate.
	err = s.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "new-session-token-2", s.sessionToken)
	assert.Equal(t, 2, authCallCount)
}

// ---------------------------------------------------------------------------
// Verify createSearchJob uses correct search query from buildSearchQuery
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_SearchQueryContent(t *testing.T) {
	sid := "search-content-job"
	var capturedSearch string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			err := r.ParseForm()
			require.NoError(t, err)
			capturedSearch = r.FormValue("search")
			assert.Equal(t, "json", r.FormValue("output_mode"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "security-events",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	expID := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	query := AlertQuery{
		Severity:     "critical",
		Source:       "ids-sensor",
		AlertType:    "malware",
		ExperimentID: expID,
		Pagination: Pagination{
			Offset: 0,
			Limit:  100,
		},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)

	// Verify the search query contains expected components.
	assert.Contains(t, capturedSearch, "index=security-events")
	assert.Contains(t, capturedSearch, "severity=critical")
	assert.Contains(t, capturedSearch, `source="ids-sensor"`)
	assert.Contains(t, capturedSearch, `type="malware"`)
	assert.Contains(t, capturedSearch, fmt.Sprintf(`experiment_id="%s"`, expID.String()))
}

// ---------------------------------------------------------------------------
// Verify URL encoding of search job parameters
// ---------------------------------------------------------------------------

func TestSplunkSIEM_CreateSearchJob_TimeRangeParameters(t *testing.T) {
	sid := "time-range-job"
	var capturedEarliest, capturedLatest string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			err := r.ParseForm()
			require.NoError(t, err)
			capturedEarliest = r.FormValue("earliest_time")
			capturedLatest = r.FormValue("latest_time")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "main",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: from,
			To:   to,
		},
		Pagination: Pagination{Limit: 100},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)

	assert.Equal(t, from.Format(time.RFC3339), capturedEarliest)
	assert.Equal(t, to.Format(time.RFC3339), capturedLatest)
}

// ---------------------------------------------------------------------------
// Verify health check with unparsable JSON still succeeds (200 OK is enough)
// ---------------------------------------------------------------------------

func TestSplunkSIEM_HealthCheck_UnparsableJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `not-json-at-all`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	// A 200 response should be considered healthy even if the body can't be parsed.
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Verify search results server error
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_ResultsServerError(t *testing.T) {
	sid := "results-error-job"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err = s.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

// ---------------------------------------------------------------------------
// Verify search results return multiple alerts
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_MultipleResults(t *testing.T) {
	sid := "multi-results-job"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			results := splunkSearchResponse{
				Results: []map[string]interface{}{
					{
						"_id":         "alert-001",
						"timestamp":   "2024-01-15T08:00:00Z",
						"severity":    "low",
						"type":        "scan",
						"source":      "scanner",
						"description": "Port scan detected",
					},
					{
						"_id":         "alert-002",
						"timestamp":   "2024-01-15T09:30:00Z",
						"severity":    "high",
						"type":        "intrusion",
						"source":      "ids",
						"description": "Intrusion detected",
					},
					{
						"_id":         "alert-003",
						"timestamp":   "2024-01-15T11:15:00Z",
						"severity":    "critical",
						"type":        "breach",
						"source":      "siem",
						"description": "Data breach detected",
					},
				},
			}
			respBytes, _ := json.Marshal(results)
			w.Write(respBytes)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC),
		},
		Pagination: Pagination{Limit: 100},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, alerts, 3)

	assert.Equal(t, "alert-001", alerts[0].ID)
	assert.Equal(t, "low", alerts[0].Severity)
	assert.Equal(t, "scan", alerts[0].Type)

	assert.Equal(t, "alert-002", alerts[1].ID)
	assert.Equal(t, "high", alerts[1].Severity)
	assert.Equal(t, "intrusion", alerts[1].Type)

	assert.Equal(t, "alert-003", alerts[2].ID)
	assert.Equal(t, "critical", alerts[2].Severity)
	assert.Equal(t, "breach", alerts[2].Type)
}

// ---------------------------------------------------------------------------
// Verify SplunkSIEM respects MaxRetries for search job creation
// ---------------------------------------------------------------------------

func TestSplunkSIEM_QueryAlerts_RetryOnSearchJobFailure(t *testing.T) {
	sid := "retry-job"
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"test-token"}`)

		case "/services/search/jobs":
			callCount++
			if callCount < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"sid":"%s"}`, sid)

		case "/services/search/jobs/" + sid:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"entry":[{"content":{"dispatchState":"DONE"}}]}`)

		case "/services/search/jobs/" + sid + "/results":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"results":[]}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 3,
		Username:   "admin",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	s := NewSplunkSIEM(cfg, WithSplunkLogger(zap.NewNop()))

	err := s.Connect(context.Background())
	require.NoError(t, err)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	alerts, err := s.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	assert.Empty(t, alerts)
	assert.Equal(t, 3, callCount)
}

// ---------------------------------------------------------------------------
// Verify search results with experiment_id and run_id fields
// ---------------------------------------------------------------------------

func TestSplunkSIEM_ParseSplunkResults_WithUUIDs(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	expID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	runID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	results := []map[string]interface{}{
		{
			"_id":           "alert-uuids",
			"timestamp":     "2024-01-15T10:30:00Z",
			"severity":      "high",
			"type":          "intrusion",
			"experiment_id": expID.String(),
			"run_id":        runID.String(),
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)
	assert.Equal(t, expID, alerts[0].ExperimentID)
	assert.Equal(t, runID, alerts[0].RunID)
}

// ---------------------------------------------------------------------------
// Verify search results handle invalid UUIDs gracefully
// ---------------------------------------------------------------------------

func TestSplunkSIEM_ParseSplunkResults_InvalidUUIDs(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:8089",
		Timeout:  5 * time.Second,
	}
	s := NewSplunkSIEM(cfg)

	results := []map[string]interface{}{
		{
			"_id":           "alert-bad-uuid",
			"experiment_id": "not-a-valid-uuid",
			"run_id":        "also-not-valid",
		},
	}

	alerts := s.parseSplunkResults(results)
	require.Len(t, alerts, 1)
	// Invalid UUIDs should result in zero UUID values (parsing fails).
	assert.Equal(t, uuid.Nil, alerts[0].ExperimentID)
	assert.Equal(t, uuid.Nil, alerts[0].RunID)
}

// ---------------------------------------------------------------------------
// Verify that health check with Atom/JSON wrapper format
// ---------------------------------------------------------------------------

func TestSplunkSIEM_HealthCheck_AtomJSONWrapper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
			"entry": [
				{
					"content": {
						"isActive": true,
						"version": "9.2.0"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	s := NewSplunkSIEM(cfg)

	err := s.HealthCheck(context.Background())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Verify that Close properly clears session token
// ---------------------------------------------------------------------------

func TestSplunkSIEM_Close_ClearsSessionToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/auth/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"sessionKey":"to-be-cleared"}`)

		case "/services/authentication/current-context/logout":
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
		Username: "admin",
		Password: "changeme",
	}
	s := NewSplunkSIEM(cfg)

	err := s.Connect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "to-be-cleared", s.sessionToken)

	err = s.Close()
	assert.NoError(t, err)
	assert.Empty(t, s.sessionToken)
}
