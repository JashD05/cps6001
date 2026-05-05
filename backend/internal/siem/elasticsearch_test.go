package siem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestElasticsearchSIEM_ImplementsSIEMConnector(t *testing.T) {
	var connector SIEMConnector
	connector = &ElasticsearchSIEM{}
	assert.NotNil(t, connector)
}

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNewElasticsearchSIEM_Defaults(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  10 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	assert.NotNil(t, es)
	assert.Equal(t, cfg, es.cfg)
	assert.NotNil(t, es.client)
	assert.Equal(t, 10*time.Second, es.client.Timeout)
}

func TestNewElasticsearchSIEM_WithLogger(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	logger := zap.NewNop()
	es := NewElasticsearchSIEM(cfg, WithElasticsearchLogger(logger))

	assert.NotNil(t, es)
}

func TestElasticsearchSIEM_ResolveIndex_Default(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)
	assert.Equal(t, "siem-alerts", es.resolveIndex())
}

func TestElasticsearchSIEM_ResolveIndex_Custom(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
		Index:    "custom-alerts",
	}
	es := NewElasticsearchSIEM(cfg)
	assert.Equal(t, "custom-alerts", es.resolveIndex())
}

// ---------------------------------------------------------------------------
// Connect
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_Connect_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"name":"node-1","cluster_name":"test-cluster","version":{"number":"8.0.0"}}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.Connect(context.Background())
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_Connect_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"internal server error"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestElasticsearchSIEM_Connect_Unreachable(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint:   "http://localhost:1", // port 1 should be unreachable
		Timeout:    1 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.Connect(context.Background())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_HealthCheck_Green(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/_cluster/health", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"green","cluster_name":"test-cluster"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_HealthCheck_Yellow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"yellow","cluster_name":"test-cluster"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_HealthCheck_Red(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"red","cluster_name":"test-cluster"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "red")
}

func TestElasticsearchSIEM_HealthCheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `error`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestElasticsearchSIEM_HealthCheck_Unreachable(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint:   "http://localhost:1",
		Timeout:    1 * time.Second,
		MaxRetries: 0,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// QueryAlerts
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_QueryAlerts_Success(t *testing.T) {
	alertTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	alertID := "alert-001"
	expID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	runID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/siem-alerts/_search", r.URL.Path)

		// Verify the request body is valid JSON with a query.
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		query, ok := reqBody["query"].(map[string]interface{})
		assert.True(t, ok, "request should contain a query")
		assert.NotNil(t, query)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		resp := esSearchResponse{
			Hits: esHitsWrapper{
				Total: esTotal{Value: 1},
				Hits: []esHitItem{
					{
						ID: alertID,
						Source: map[string]interface{}{
							"timestamp":     alertTime.Format(time.RFC3339),
							"severity":      "high",
							"type":          "network_intrusion",
							"source":        "firewall",
							"description":   "Suspicious traffic detected",
							"experiment_id": expID.String(),
							"run_id":        runID.String(),
							"metadata": map[string]interface{}{
								"risk_score": 85.0,
							},
						},
					},
				},
			},
		}

		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: alertTime.Add(-1 * time.Hour),
			To:   alertTime.Add(1 * time.Hour),
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

	alerts, err := es.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	assert.Equal(t, alertID, alerts[0].ID)
	assert.Equal(t, "high", alerts[0].Severity)
	assert.Equal(t, "network_intrusion", alerts[0].Type)
	assert.Equal(t, "firewall", alerts[0].Source)
	assert.Equal(t, "Suspicious traffic detected", alerts[0].Description)
	assert.Equal(t, expID, alerts[0].ExperimentID)
	assert.Equal(t, runID, alerts[0].RunID)
	assert.Equal(t, 85.0, alerts[0].Metadata["risk_score"])
}

func TestElasticsearchSIEM_QueryAlerts_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		resp := esSearchResponse{
			Hits: esHitsWrapper{
				Total: esTotal{Value: 0},
				Hits:  []esHitItem{},
			},
		}

		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-24 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 100},
	}

	alerts, err := es.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
	assert.Empty(t, alerts)
}

func TestElasticsearchSIEM_QueryAlerts_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":{"root_cause":[{"type":"search_phase_execution_exception"}]}}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
	}

	_, err := es.QueryAlerts(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestElasticsearchSIEM_QueryAlerts_CustomIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the custom index is used in the URL path.
		assert.Equal(t, "/my-custom-index/_search", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		resp := esSearchResponse{
			Hits: esHitsWrapper{
				Total: esTotal{Value: 0},
				Hits:  []esHitItem{},
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "my-custom-index",
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 100},
	}

	_, err := es.QueryAlerts(context.Background(), query)
	require.NoError(t, err)
}

func TestElasticsearchSIEM_QueryAlerts_Pagination(t *testing.T) {
	var receivedFrom *int
	var receivedSize *int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		if v, ok := reqBody["from"]; ok {
			from := int(v.(float64))
			receivedFrom = &from
		}
		if v, ok := reqBody["size"]; ok {
			size := int(v.(float64))
			receivedSize = &size
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		resp := esSearchResponse{
			Hits: esHitsWrapper{
				Total: esTotal{Value: 0},
				Hits:  []esHitItem{},
			},
		}
		respBytes, _ := json.Marshal(resp)
		w.Write(respBytes)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{
			Offset: 50,
			Limit:  25,
		},
	}

	_, err := es.QueryAlerts(context.Background(), query)
	require.NoError(t, err)

	assert.NotNil(t, receivedFrom)
	assert.Equal(t, 50, *receivedFrom)
	assert.NotNil(t, receivedSize)
	assert.Equal(t, 25, *receivedSize)
}

// ---------------------------------------------------------------------------
// SendAlert
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_SendAlert_Success(t *testing.T) {
	alert := SIEMAlert{
		ID:          "alert-123",
		Timestamp:   time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
		Severity:    "critical",
		Type:        "privilege_escalation",
		Source:      "host-ids",
		Description: "Privilege escalation attempt detected",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/siem-alerts/_doc", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var receivedAlert SIEMAlert
		err := json.NewDecoder(r.Body).Decode(&receivedAlert)
		assert.NoError(t, err)
		assert.Equal(t, alert.ID, receivedAlert.ID)
		assert.Equal(t, alert.Severity, receivedAlert.Severity)
		assert.Equal(t, alert.Type, receivedAlert.Type)
		assert.Equal(t, alert.Source, receivedAlert.Source)
		assert.Equal(t, alert.Description, receivedAlert.Description)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"_id":"alert-123","result":"created","_version":1}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_SendAlert_ServerError(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-456",
		Timestamp: time.Now(),
		Severity:  "low",
		Type:      "test",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"index not found"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.SendAlert(context.Background(), alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestElasticsearchSIEM_SendAlert_WithAPIKey(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-789",
		Timestamp: time.Now(),
		Severity:  "medium",
		Type:      "scan",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-es-api-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"_id":"alert-789","result":"updated","_version":2}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		APIKey:     "my-es-api-key",
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_SendAlert_WithBasicAuth(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-basic-auth",
		Timestamp: time.Now(),
		Severity:  "low",
		Type:      "test",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		assert.True(t, ok, "basic auth should be set")
		assert.Equal(t, "elastic", username)
		assert.Equal(t, "changeme", password)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"_id":"alert-basic-auth","result":"updated","_version":1}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "elastic",
		Password:   "changeme",
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

func TestElasticsearchSIEM_SendAlert_CustomIndex(t *testing.T) {
	alert := SIEMAlert{
		ID:        "alert-custom",
		Timestamp: time.Now(),
		Severity:  "info",
		Type:      "audit",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/custom-index/_doc", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"_id":"alert-custom","result":"created","_version":1}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Index:      "custom-index",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_Close(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.Close()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// BuildSearchQuery
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_BuildSearchQuery_TimeRange(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	query := AlertQuery{
		TimeRange:  TimeRange{From: from, To: to},
		Pagination: Pagination{Offset: 0, Limit: 50},
	}

	result := es.buildSearchQuery(query)

	// Verify the query structure.
	queryMap, ok := result["query"].(map[string]interface{})
	require.True(t, ok, "result should have a query key")

	boolMap, ok := queryMap["bool"].(map[string]interface{})
	require.True(t, ok, "query should have a bool key")

	mustClauses, ok := boolMap["must"].([]interface{})
	require.True(t, ok, "bool query should have must clauses")
	require.Len(t, mustClauses, 1, "should have exactly 1 must clause for time range")

	// Verify pagination.
	assert.Equal(t, 50, result["size"])
}

func TestElasticsearchSIEM_BuildSearchQuery_AllFilters(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	expID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	runID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC),
		},
		Severity:     "critical",
		Source:       "ids-sensor",
		AlertType:    "malware_detection",
		ExperimentID: expID,
		RunID:        runID,
		Pagination: Pagination{
			Offset: 100,
			Limit:  25,
		},
	}

	result := es.buildSearchQuery(query)

	queryMap, ok := result["query"].(map[string]interface{})
	require.True(t, ok)

	boolMap, ok := queryMap["bool"].(map[string]interface{})
	require.True(t, ok)

	mustClauses, ok := boolMap["must"].([]interface{})
	require.True(t, ok)
	// Should have: time range + severity + source + type + experiment_id + run_id = 6
	assert.Len(t, mustClauses, 6)

	// Verify pagination.
	assert.Equal(t, 100, result["from"])
	assert.Equal(t, 25, result["size"])
}

func TestElasticsearchSIEM_BuildSearchQuery_DefaultLimit(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	query := AlertQuery{
		Pagination: Pagination{Offset: 0, Limit: 0},
	}

	result := es.buildSearchQuery(query)

	// When Limit is 0 (not set), default to 100.
	assert.Equal(t, 100, result["size"])
}

// ---------------------------------------------------------------------------
// ParseHits
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_ParseHits_FullDocument(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	expID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	runID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	hits := []esHitItem{
		{
			ID: "hit-001",
			Source: map[string]interface{}{
				"timestamp":     "2024-01-15T10:30:00Z",
				"severity":      "high",
				"type":          "intrusion",
				"source":        "firewall",
				"description":   "Detected suspicious activity",
				"experiment_id": expID.String(),
				"run_id":        runID.String(),
				"metadata": map[string]interface{}{
					"risk_score": 90.0,
					"region":     "us-east-1",
				},
			},
		},
	}

	alerts, err := es.parseHits(hits)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	alert := alerts[0]
	assert.Equal(t, "hit-001", alert.ID)
	assert.Equal(t, "high", alert.Severity)
	assert.Equal(t, "intrusion", alert.Type)
	assert.Equal(t, "firewall", alert.Source)
	assert.Equal(t, "Detected suspicious activity", alert.Description)
	assert.Equal(t, expID, alert.ExperimentID)
	assert.Equal(t, runID, alert.RunID)
	assert.Equal(t, 90.0, alert.Metadata["risk_score"])
	assert.Equal(t, "us-east-1", alert.Metadata["region"])
}

func TestElasticsearchSIEM_ParseHits_MinimalDocument(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	hits := []esHitItem{
		{
			ID:     "hit-002",
			Source: map[string]interface{}{},
		},
	}

	alerts, err := es.parseHits(hits)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	alert := alerts[0]
	assert.Equal(t, "hit-002", alert.ID)
	assert.Empty(t, alert.Severity)
	assert.Empty(t, alert.Type)
	assert.True(t, alert.Timestamp.IsZero())
}

func TestElasticsearchSIEM_ParseHits_MultipleDocuments(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	hits := []esHitItem{
		{
			ID: "hit-a",
			Source: map[string]interface{}{
				"timestamp":   "2024-01-01T00:00:00Z",
				"severity":    "low",
				"type":        "scan",
				"source":      "scanner",
				"description": "Port scan detected",
			},
		},
		{
			ID: "hit-b",
			Source: map[string]interface{}{
				"timestamp":   "2024-01-02T12:00:00Z",
				"severity":    "critical",
				"type":        "breach",
				"source":      "ids",
				"description": "Data breach detected",
			},
		},
	}

	alerts, err := es.parseHits(hits)
	require.NoError(t, err)
	require.Len(t, alerts, 2)

	assert.Equal(t, "scan", alerts[0].Type)
	assert.Equal(t, "low", alerts[0].Severity)
	assert.Equal(t, "breach", alerts[1].Type)
	assert.Equal(t, "critical", alerts[1].Severity)
}

func TestElasticsearchSIEM_ParseHits_Empty(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}
	es := NewElasticsearchSIEM(cfg)

	alerts, err := es.parseHits([]esHitItem{})
	require.NoError(t, err)
	assert.Empty(t, alerts)
}

// ---------------------------------------------------------------------------
// Auth headers
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_AuthHeaders_APIKey(t *testing.T) {
	var capturedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"green"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		APIKey:     "test-api-key-123",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-api-key-123", capturedAuth)
}

func TestElasticsearchSIEM_AuthHeaders_BasicAuth(t *testing.T) {
	var capturedUser, capturedPass string
	var hasBasicAuth bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser, capturedPass, hasBasicAuth = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"green"}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		Username:   "elastic",
		Password:   "s3cret",
	}
	es := NewElasticsearchSIEM(cfg)

	err := es.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, hasBasicAuth)
	assert.Equal(t, "elastic", capturedUser)
	assert.Equal(t, "s3cret", capturedPass)
}

// ---------------------------------------------------------------------------
// Retry logic
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_Retry_SuccessAfterRetry(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"name":"node-1","cluster_name":"test-cluster","version":{"number":"8.0.0"}}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 3,
	}
	es := NewElasticsearchSIEM(cfg, WithElasticsearchLogger(zap.NewNop()))

	// Connect() uses withRetry, so it will retry on transient failures.
	err := es.Connect(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestElasticsearchSIEM_Retry_Exhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
	}
	es := NewElasticsearchSIEM(cfg, WithElasticsearchLogger(zap.NewNop()))

	// Connect() uses withRetry, so retries will be attempted before giving up.
	err := es.Connect(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 2 retries")
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestElasticsearchSIEM_QueryAlerts_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{}`)
	}))
	defer server.Close()

	cfg := SIEMConfig{
		Endpoint:   server.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
		Index:      "siem-alerts",
	}
	es := NewElasticsearchSIEM(cfg, WithElasticsearchLogger(zap.NewNop()))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	query := AlertQuery{
		TimeRange: TimeRange{
			From: time.Now().Add(-1 * time.Hour),
			To:   time.Now(),
		},
		Pagination: Pagination{Limit: 10},
	}

	_, err := es.QueryAlerts(ctx, query)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewSIEMConnector factory integration
// ---------------------------------------------------------------------------

func TestNewSIEMConnector_Elasticsearch(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}

	connector, err := NewSIEMConnector("elasticsearch", cfg)
	require.NoError(t, err)
	assert.NotNil(t, connector)

	// Verify it's an ElasticsearchSIEM.
	es, ok := connector.(*ElasticsearchSIEM)
	assert.True(t, ok)
	assert.NotNil(t, es)
}

func TestNewSIEMConnector_Elasticsearch_InvalidConfig(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "", // empty endpoint should fail validation
		Timeout:  5 * time.Second,
	}

	_, err := NewSIEMConnector("elasticsearch", cfg)
	assert.Error(t, err)
}

func TestNewSIEMConnector_UnsupportedProvider(t *testing.T) {
	cfg := SIEMConfig{
		Endpoint: "http://localhost:9200",
		Timeout:  5 * time.Second,
	}

	_, err := NewSIEMConnector("qradar", cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported siem provider")
	assert.Contains(t, err.Error(), "elasticsearch")
	assert.Contains(t, err.Error(), "splunk")
}
