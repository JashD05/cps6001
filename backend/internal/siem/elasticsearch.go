package siem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ElasticsearchSIEM implements the SIEMConnector interface for Elasticsearch/ELK.
// It communicates with Elasticsearch via its REST API, using the configured
// index for alert document storage and retrieval.
type ElasticsearchSIEM struct {
	cfg    SIEMConfig
	client *http.Client
	logger *zap.Logger
}

// NewElasticsearchSIEM creates a new ElasticsearchSIEM connector with the
// provided configuration. The HTTP client is configured with the timeout
// from cfg and reasonable transport defaults.
func NewElasticsearchSIEM(cfg SIEMConfig, opts ...ElasticsearchOption) *ElasticsearchSIEM {
	e := &ElasticsearchSIEM{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
				MaxIdleConnsPerHost: 5,
			},
		},
		logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ElasticsearchOption is a functional option for configuring ElasticsearchSIEM.
type ElasticsearchOption func(*ElasticsearchSIEM)

// WithElasticsearchLogger sets the logger for the ElasticsearchSIEM connector.
func WithElasticsearchLogger(logger *zap.Logger) ElasticsearchOption {
	return func(e *ElasticsearchSIEM) {
		e.logger = logger.Named("elasticsearch_siem")
	}
}

// ---------------------------------------------------------------------------
// Elasticsearch response types
// ---------------------------------------------------------------------------

// esSearchResponse represents the relevant fields from an Elasticsearch
// _search API response.
type esSearchResponse struct {
	Hits esHitsWrapper `json:"hits"`
}

// esHitsWrapper wraps the outer hits object from an Elasticsearch response.
type esHitsWrapper struct {
	Total esTotal     `json:"total"`
	Hits  []esHitItem `json:"hits"`
}

// esTotal represents the total number of hits in an Elasticsearch response.
type esTotal struct {
	Value int `json:"value"`
}

// esHitItem represents a single hit from an Elasticsearch search response.
type esHitItem struct {
	ID     string                 `json:"_id"`
	Source map[string]interface{} `json:"_source"`
}

// esIndexResponse represents the response from indexing a document.
type esIndexResponse struct {
	ID      string `json:"_id"`
	Result  string `json:"result"`
	Version int    `json:"_version"`
}

// esClusterHealth represents relevant fields from the cluster health endpoint.
type esClusterHealth struct {
	Status string `json:"status"`
}

// ---------------------------------------------------------------------------
// SIEMConnector interface implementation
// ---------------------------------------------------------------------------

// Connect tests the connection to Elasticsearch by hitting the root endpoint
// and verifying the cluster is reachable. It retries on transient failures up
// to the configured MaxRetries.
func (e *ElasticsearchSIEM) Connect(ctx context.Context) error {
	return e.withRetry(ctx, e.cfg.MaxRetries, func(ctx context.Context) error {
		reqURL := e.cfg.Endpoint + "/"

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create connect request: %w", err)
		}

		e.setAuthHeaders(req)

		resp, err := e.client.Do(req)
		if err != nil {
			return fmt.Errorf("elasticsearch connect request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read connect response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("elasticsearch connect returned status %d: %s", resp.StatusCode, string(body))
		}

		e.logger.Debug("elasticsearch connection established")
		return nil
	})
}

// QueryAlerts queries Elasticsearch for alerts matching the provided query
// parameters. It uses the _search API with bool queries to filter by time
// range, severity, source, type, experiment ID, and run ID.
//
// The endpoint is: POST {endpoint}/{index}/_search
func (e *ElasticsearchSIEM) QueryAlerts(ctx context.Context, query AlertQuery) ([]SIEMAlert, error) {
	indexName := e.resolveIndex()

	// Build the Elasticsearch bool query.
	esQuery := e.buildSearchQuery(query)

	queryBody, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal elasticsearch query: %w", err)
	}

	reqURL := fmt.Sprintf("%s/%s/_search", e.cfg.Endpoint, indexName)

	var alerts []SIEMAlert

	err = e.withRetry(ctx, e.cfg.MaxRetries, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(queryBody))
		if err != nil {
			return fmt.Errorf("failed to create query alerts request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		e.setAuthHeaders(req)

		resp, err := e.client.Do(req)
		if err != nil {
			return fmt.Errorf("elasticsearch query alerts request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read query alerts response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("elasticsearch query alerts returned status %d: %s", resp.StatusCode, string(body))
		}

		var searchResp esSearchResponse
		if err := json.Unmarshal(body, &searchResp); err != nil {
			return fmt.Errorf("failed to decode elasticsearch search response: %w", err)
		}

		alerts, err = e.parseHits(searchResp.Hits.Hits)
		if err != nil {
			return fmt.Errorf("failed to parse elasticsearch hits: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	e.logger.Debug("queried elasticsearch alerts",
		zap.Int("count", len(alerts)),
	)
	return alerts, nil
}

// SendAlert indexes an alert document in Elasticsearch. It uses the _doc
// API to create a document with an auto-generated ID.
//
// The endpoint is: POST {endpoint}/{index}/_doc
func (e *ElasticsearchSIEM) SendAlert(ctx context.Context, alert SIEMAlert) error {
	indexName := e.resolveIndex()

	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	reqURL := fmt.Sprintf("%s/%s/_doc", e.cfg.Endpoint, indexName)

	return e.withRetry(ctx, e.cfg.MaxRetries, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create send alert request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		e.setAuthHeaders(req)

		resp, err := e.client.Do(req)
		if err != nil {
			return fmt.Errorf("elasticsearch send alert request failed: %w", err)
		}
		defer resp.Body.Close()

		// Drain the body to allow connection reuse.
		_, _ = io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("elasticsearch send alert returned status %d", resp.StatusCode)
		}

		e.logger.Debug("alert indexed in elasticsearch",
			zap.String("alert_id", alert.ID),
			zap.String("alert_type", alert.Type),
		)
		return nil
	})
}

// HealthCheck verifies that the Elasticsearch cluster is healthy by calling
// the _cluster/health endpoint. A cluster status of "red" is considered
// unhealthy; "yellow" and "green" are considered operational.
func (e *ElasticsearchSIEM) HealthCheck(ctx context.Context) error {
	reqURL := e.cfg.Endpoint + "/_cluster/health"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	e.setAuthHeaders(req)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("elasticsearch health check failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read health check response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("elasticsearch health check returned status %d: %s", resp.StatusCode, string(body))
	}

	var health esClusterHealth
	if err := json.Unmarshal(body, &health); err != nil {
		return fmt.Errorf("failed to decode elasticsearch health response: %w", err)
	}

	if health.Status == "red" {
		return fmt.Errorf("elasticsearch cluster status is red")
	}

	e.logger.Debug("elasticsearch health check passed",
		zap.String("cluster_status", health.Status),
	)
	return nil
}

// Close releases resources held by the Elasticsearch connector by closing
// idle HTTP connections.
func (e *ElasticsearchSIEM) Close() error {
	e.client.CloseIdleConnections()
	e.logger.Debug("elasticsearch SIEM connector closed")
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// setAuthHeaders sets the appropriate authentication headers on the request
// based on the SIEM configuration. It supports API key and basic auth.
func (e *ElasticsearchSIEM) setAuthHeaders(req *http.Request) {
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}
	if e.cfg.Username != "" && e.cfg.Password != "" {
		req.SetBasicAuth(e.cfg.Username, e.cfg.Password)
	}
}

// resolveIndex returns the Elasticsearch index name from the configuration,
// defaulting to "siem-alerts" if not set.
func (e *ElasticsearchSIEM) resolveIndex() string {
	if e.cfg.Index != "" {
		return e.cfg.Index
	}
	return "siem-alerts"
}

// buildSearchQuery constructs an Elasticsearch bool query from the AlertQuery
// parameters. It maps each filter field to the appropriate ES query clause.
func (e *ElasticsearchSIEM) buildSearchQuery(query AlertQuery) map[string]interface{} {
	mustClauses := make([]interface{}, 0)

	// Time range filter using the @timestamp field (standard ES convention).
	if !query.TimeRange.From.IsZero() || !query.TimeRange.To.IsZero() {
		rangeClause := map[string]interface{}{
			"range": map[string]interface{}{
				"timestamp": map[string]interface{}{},
			},
		}
		rangeFields := rangeClause["range"].(map[string]interface{})["timestamp"].(map[string]interface{})
		if !query.TimeRange.From.IsZero() {
			rangeFields["gte"] = query.TimeRange.From.Format(time.RFC3339)
		}
		if !query.TimeRange.To.IsZero() {
			rangeFields["lte"] = query.TimeRange.To.Format(time.RFC3339)
		}
		mustClauses = append(mustClauses, rangeClause)
	}

	// Severity filter.
	if query.Severity != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"severity": query.Severity,
			},
		})
	}

	// Source filter.
	if query.Source != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"source": query.Source,
			},
		})
	}

	// Alert type filter.
	if query.AlertType != "" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"type": query.AlertType,
			},
		})
	}

	// Experiment ID filter.
	if query.ExperimentID.String() != "00000000-0000-0000-0000-000000000000" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"experiment_id": query.ExperimentID.String(),
			},
		})
	}

	// Run ID filter.
	if query.RunID.String() != "00000000-0000-0000-0000-000000000000" {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"run_id": query.RunID.String(),
			},
		})
	}

	// Build the final query with pagination.
	esQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustClauses,
			},
		},
	}

	// Add pagination via from/size.
	if query.Pagination.Offset > 0 {
		esQuery["from"] = query.Pagination.Offset
	}
	if query.Pagination.Limit > 0 {
		esQuery["size"] = query.Pagination.Limit
	} else {
		esQuery["size"] = 100 // default page size
	}

	return esQuery
}

// parseHits converts Elasticsearch hit items into SIEMAlert objects.
func (e *ElasticsearchSIEM) parseHits(hits []esHitItem) ([]SIEMAlert, error) {
	alerts := make([]SIEMAlert, 0, len(hits))

	for _, hit := range hits {
		alert := SIEMAlert{
			ID: hit.ID,
		}

		src := hit.Source

		// Extract timestamp.
		if ts, ok := src["timestamp"].(string); ok {
			parsed, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				// Try as a generic date string; best effort.
				parsed, _ = time.Parse(time.RFC3339Nano, ts)
			}
			alert.Timestamp = parsed
		}

		// Extract severity.
		if severity, ok := src["severity"].(string); ok {
			alert.Severity = severity
		}

		// Extract type.
		if typ, ok := src["type"].(string); ok {
			alert.Type = typ
		}

		// Extract source.
		if source, ok := src["source"].(string); ok {
			alert.Source = source
		}

		// Extract description.
		if desc, ok := src["description"].(string); ok {
			alert.Description = desc
		}

		// Extract experiment_id.
		if expID, ok := src["experiment_id"].(string); ok {
			parsed, err := parseUUID(expID)
			if err == nil {
				alert.ExperimentID = parsed
			}
		}

		// Extract run_id.
		if runID, ok := src["run_id"].(string); ok {
			parsed, err := parseUUID(runID)
			if err == nil {
				alert.RunID = parsed
			}
		}

		// Extract metadata as a nested object.
		if metadata, ok := src["metadata"].(map[string]interface{}); ok {
			alert.Metadata = metadata
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// parseUUID parses a UUID string using the google/uuid package.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// withRetry executes the given operation with exponential backoff retries.
// It stops retrying on success, context cancellation, or after maxRetries
// attempts. The backoff delay is: 1s, 2s, 4s, 8s... capped at 30 seconds.
func (e *ElasticsearchSIEM) withRetry(ctx context.Context, maxRetries int, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			e.logger.Debug("retrying elasticsearch operation",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
				zap.Error(lastErr),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w (last error: %v)", ctx.Err(), lastErr)
			case <-time.After(delay):
			}
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w (last error: %v)", ctx.Err(), lastErr)
		}
	}

	return fmt.Errorf("elasticsearch operation failed after %d retries: %w", maxRetries, lastErr)
}
