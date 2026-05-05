package siem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SplunkSIEM implements the SIEMConnector interface for Splunk.
// It communicates with Splunk via its REST API, authenticating with
// username/password to obtain a session token for subsequent requests.
type SplunkSIEM struct {
	cfg    SIEMConfig
	client *http.Client
	logger *zap.Logger

	// sessionToken holds the Splunk session key obtained during Connect.
	// It is set after a successful call to Connect and used as a Bearer
	// token in subsequent API requests.
	sessionToken string
}

// NewSplunkSIEM creates a new SplunkSIEM connector with the provided
// configuration. The HTTP client is configured with the timeout from cfg
// and reasonable transport defaults.
func NewSplunkSIEM(cfg SIEMConfig, opts ...SplunkOption) *SplunkSIEM {
	s := &SplunkSIEM{
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
		opt(s)
	}

	return s
}

// SplunkOption is a functional option for configuring SplunkSIEM.
type SplunkOption func(*SplunkSIEM)

// WithSplunkLogger sets the logger for the SplunkSIEM connector.
func WithSplunkLogger(logger *zap.Logger) SplunkOption {
	return func(s *SplunkSIEM) {
		s.logger = logger.Named("splunk_siem")
	}
}

// ---------------------------------------------------------------------------
// Splunk response types
// ---------------------------------------------------------------------------

// splunkAuthResponse represents the response from the Splunk auth/login endpoint.
type splunkAuthResponse struct {
	SessionKey string `json:"sessionKey"`
}

// splunkSearchResponse represents the response from the Splunk search results endpoint.
type splunkSearchResponse struct {
	Results []map[string]interface{} `json:"results"`
}

// splunkServerInfo represents relevant fields from the server/info endpoint.
type splunkServerInfo struct {
	IsActive bool   `json:"isActive"`
	Version  string `json:"version"`
}

// ---------------------------------------------------------------------------
// SIEMConnector interface implementation
// ---------------------------------------------------------------------------

// Connect authenticates to Splunk by sending credentials to the auth/login
// endpoint and storing the returned session token for subsequent requests.
// It retries on transient failures up to the configured MaxRetries.
func (s *SplunkSIEM) Connect(ctx context.Context) error {
	return s.withRetry(ctx, s.cfg.MaxRetries, func(ctx context.Context) error {
		// If we already have a valid session token, just verify it.
		if s.sessionToken != "" {
			if err := s.HealthCheck(ctx); err != nil {
				s.logger.Warn("existing splunk session is invalid, re-authenticating",
					zap.Error(err),
				)
				s.sessionToken = ""
			} else {
				return nil
			}
		}

		reqURL := fmt.Sprintf("%s/services/auth/login", s.cfg.Endpoint)

		data := url.Values{}
		data.Set("username", s.cfg.Username)
		data.Set("password", s.cfg.Password)
		// output_mode=json tells Splunk to return JSON instead of Atom/XML.
		data.Set("output_mode", "json")

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(data.Encode()))
		if err != nil {
			return fmt.Errorf("failed to create splunk auth request: %w", err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("splunk auth request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read splunk auth response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("splunk auth returned status %d: %s", resp.StatusCode, string(body))
		}

		var authResp splunkAuthResponse
		if err := json.Unmarshal(body, &authResp); err != nil {
			return fmt.Errorf("failed to decode splunk auth response: %w", err)
		}

		if authResp.SessionKey == "" {
			return fmt.Errorf("splunk auth succeeded but no session key returned")
		}

		s.sessionToken = authResp.SessionKey

		s.logger.Debug("splunk authentication successful")
		return nil
	})
}

// QueryAlerts queries Splunk for alerts matching the provided query parameters.
// It uses the search/jobs REST API to execute a Splunk search query, then
// retrieves the results.
//
// The flow is:
//  1. POST /services/search/jobs — create a search job
//  2. GET /services/search/jobs/{sid} — poll for completion
//  3. GET /services/search/jobs/{sid}/results — fetch results
func (s *SplunkSIEM) QueryAlerts(ctx context.Context, query AlertQuery) ([]SIEMAlert, error) {
	if s.sessionToken == "" {
		return nil, fmt.Errorf("splunk connector is not connected; call Connect() first")
	}

	// Build the Splunk search query from the AlertQuery parameters.
	searchQuery := s.buildSearchQuery(query)

	var alerts []SIEMAlert

	err := s.withRetry(ctx, s.cfg.MaxRetries, func(ctx context.Context) error {
		// Step 1: Create a search job.
		sid, err := s.createSearchJob(ctx, searchQuery, query)
		if err != nil {
			return err
		}

		// Step 2: Wait for the search job to complete.
		if err := s.waitForSearchJob(ctx, sid); err != nil {
			return err
		}

		// Step 3: Retrieve the search results.
		alerts, err = s.getSearchResults(ctx, sid)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.Debug("queried splunk alerts",
		zap.Int("count", len(alerts)),
	)
	return alerts, nil
}

// SendAlert sends an alert to Splunk by creating an event via the
// receivers/simple endpoint. The index field in SIEMConfig is used
// as the target Splunk index.
//
// The endpoint is: POST /services/receivers/simple
func (s *SplunkSIEM) SendAlert(ctx context.Context, alert SIEMAlert) error {
	if s.sessionToken == "" {
		return fmt.Errorf("splunk connector is not connected; call Connect() first")
	}

	return s.withRetry(ctx, s.cfg.MaxRetries, func(ctx context.Context) error {
		reqURL := fmt.Sprintf("%s/services/receivers/simple", s.cfg.Endpoint)

		// Build the event body. Splunk's receivers/simple endpoint accepts
		// the raw event text in the body, with sourcetype and index as
		// query parameters.
		eventPayload, err := json.Marshal(alert)
		if err != nil {
			return fmt.Errorf("failed to marshal alert for splunk: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(eventPayload))
		if err != nil {
			return fmt.Errorf("failed to create send alert request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		s.setAuthHeaders(req)

		// Set query parameters for index and sourcetype.
		q := req.URL.Query()
		if s.cfg.Index != "" {
			q.Set("index", s.cfg.Index)
		}
		q.Set("sourcetype", "chaos-sec:alert")
		req.URL.RawQuery = q.Encode()

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("splunk send alert request failed: %w", err)
		}
		defer resp.Body.Close()

		// Drain the body to allow connection reuse.
		_, _ = io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("splunk send alert returned status %d", resp.StatusCode)
		}

		s.logger.Debug("alert sent to splunk",
			zap.String("alert_id", alert.ID),
			zap.String("alert_type", alert.Type),
		)
		return nil
	})
}

// HealthCheck verifies that the Splunk server is reachable and operational
// by calling the server/info endpoint.
func (s *SplunkSIEM) HealthCheck(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/services/server/info", s.cfg.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	s.setAuthHeaders(req)

	// Request JSON output from Splunk.
	q := req.URL.Query()
	q.Set("output_mode", "json")
	req.URL.RawQuery = q.Encode()

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("splunk health check failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read splunk health check response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("splunk health check returned status %d: %s", resp.StatusCode, string(body))
	}

	var info splunkServerInfo
	// Splunk's /server/info endpoint returns an entry wrapper; try direct
	// parse first, then fall back to extracting from the wrapper format.
	if err := json.Unmarshal(body, &info); err != nil {
		// Try the Splunk Atom/JSON wrapper format.
		var wrapper struct {
			Entry []struct {
				Content splunkServerInfo `json:"content"`
			} `json:"entry"`
		}
		if err2 := json.Unmarshal(body, &wrapper); err2 != nil {
			// Even if parsing fails, a 200 response is sufficient to consider
			// the health check as passed — we just won't log the version.
			s.logger.Debug("splunk health check passed (unparsed response)")
			return nil
		}
		if len(wrapper.Entry) > 0 {
			info = wrapper.Entry[0].Content
		}
	}

	s.logger.Debug("splunk health check passed",
		zap.Bool("active", info.IsActive),
		zap.String("version", info.Version),
	)
	return nil
}

// Close logs out of Splunk by invalidating the session token, then closes
// idle HTTP connections.
func (s *SplunkSIEM) Close() error {
	if s.sessionToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		reqURL := fmt.Sprintf("%s/services/authentication/current-context/logout", s.cfg.Endpoint)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, nil)
		if err != nil {
			s.logger.Warn("failed to create splunk logout request", zap.Error(err))
		} else {
			s.setAuthHeaders(req)
			resp, err := s.client.Do(req)
			if err != nil {
				s.logger.Warn("splunk logout request failed", zap.Error(err))
			} else {
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}

		s.sessionToken = ""
	}

	s.client.CloseIdleConnections()
	s.logger.Debug("splunk SIEM connector closed")
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// setAuthHeaders sets the appropriate authentication headers on the request.
// If a session token is available (post-Connect), it is used as a Bearer
// token. Otherwise, it falls back to API key or basic auth from config.
func (s *SplunkSIEM) setAuthHeaders(req *http.Request) {
	if s.sessionToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.sessionToken)
		return
	}
	if s.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}
	if s.cfg.Username != "" && s.cfg.Password != "" {
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}
}

// buildSearchQuery constructs a Splunk search query string from the
// AlertQuery parameters.
func (s *SplunkSIEM) buildSearchQuery(query AlertQuery) string {
	// Start with a base search for the configured index (or "main" as default).
	indexName := "main"
	if s.cfg.Index != "" {
		indexName = s.cfg.Index
	}

	parts := []string{fmt.Sprintf("index=%s", indexName)}

	// Time range.
	if !query.TimeRange.From.IsZero() || !query.TimeRange.To.IsZero() {
		var timeParts []string
		if !query.TimeRange.From.IsZero() {
			timeParts = append(timeParts, fmt.Sprintf("earliest=%s", query.TimeRange.From.Format(splunkTimeFormat)))
		}
		if !query.TimeRange.To.IsZero() {
			timeParts = append(timeParts, fmt.Sprintf("latest=%s", query.TimeRange.To.Format(splunkTimeFormat)))
		}
		if len(timeParts) > 0 {
			parts = append(parts, strings.Join(timeParts, " "))
		}
	}

	// Severity filter.
	if query.Severity != "" {
		parts = append(parts, fmt.Sprintf("severity=%s", query.Severity))
	}

	// Source filter.
	if query.Source != "" {
		parts = append(parts, fmt.Sprintf("source=\"%s\"", query.Source))
	}

	// Alert type filter.
	if query.AlertType != "" {
		parts = append(parts, fmt.Sprintf("type=\"%s\"", query.AlertType))
	}

	// Experiment ID filter.
	if query.ExperimentID.String() != "00000000-0000-0000-0000-000000000000" {
		parts = append(parts, fmt.Sprintf("experiment_id=\"%s\"", query.ExperimentID.String()))
	}

	// Run ID filter.
	if query.RunID.String() != "00000000-0000-0000-0000-000000000000" {
		parts = append(parts, fmt.Sprintf("run_id=\"%s\"", query.RunID.String()))
	}

	return strings.Join(parts, " ")
}

// splunkTimeFormat is the time format used by Splunk for time ranges.
const splunkTimeFormat = "01/02/2006:15:04:05"

// createSearchJob submits a search job to Splunk and returns the search ID (sid).
func (s *SplunkSIEM) createSearchJob(ctx context.Context, searchQuery string, query AlertQuery) (string, error) {
	reqURL := fmt.Sprintf("%s/services/search/jobs", s.cfg.Endpoint)

	data := url.Values{}
	data.Set("search", searchQuery)
	data.Set("output_mode", "json")

	// Set time range on the job if specified.
	if !query.TimeRange.From.IsZero() {
		data.Set("earliest_time", query.TimeRange.From.Format(time.RFC3339))
	}
	if !query.TimeRange.To.IsZero() {
		data.Set("latest_time", query.TimeRange.To.Format(time.RFC3339))
	}

	// Set result count based on pagination.
	if query.Pagination.Limit > 0 {
		data.Set("max_count", fmt.Sprintf("%d", query.Pagination.Limit))
	} else {
		data.Set("max_count", "100")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create search job request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.setAuthHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("splunk search job request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read search job response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("splunk search job returned status %d: %s", resp.StatusCode, string(body))
	}

	// Splunk returns the sid in a JSON response. Try the flat format first,
	// then fall back to the Splunk Atom/JSON wrapper format if the flat
	// unmarshal fails or yields an empty SID.
	var jobResp struct {
		SID string `json:"sid"`
	}
	flatErr := json.Unmarshal(body, &jobResp)

	var wrapper struct {
		Entry []struct {
			Content struct {
				SID string `json:"sid"`
			} `json:"content"`
		} `json:"entry"`
	}
	wrapperErr := json.Unmarshal(body, &wrapper)

	// Prefer the flat format when it yields a non-empty SID; otherwise
	// fall back to the wrapper format.
	if flatErr != nil || jobResp.SID == "" {
		if wrapperErr != nil {
			return "", fmt.Errorf("failed to decode splunk search job response: flat=%w, wrapper=%w", flatErr, wrapperErr)
		}
		if len(wrapper.Entry) == 0 {
			return "", fmt.Errorf("splunk search job response contained no entries")
		}
		jobResp.SID = wrapper.Entry[0].Content.SID
	}

	if jobResp.SID == "" {
		return "", fmt.Errorf("splunk search job returned empty sid")
	}

	s.logger.Debug("splunk search job created",
		zap.String("sid", jobResp.SID),
	)
	return jobResp.SID, nil
}

// waitForSearchJob polls the Splunk search job until it completes or the
// context is cancelled. It checks at regular intervals.
func (s *SplunkSIEM) waitForSearchJob(ctx context.Context, sid string) error {
	reqURL := fmt.Sprintf("%s/services/search/jobs/%s", s.cfg.Endpoint, sid)

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create search job status request: %w", err)
		}

		s.setAuthHeaders(req)
		q := req.URL.Query()
		q.Set("output_mode", "json")
		req.URL.RawQuery = q.Encode()

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("splunk search job status request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read search job status response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("splunk search job status returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse the job status from the response.
		// Splunk returns dispatchState in the entry content.
		var wrapper struct {
			Entry []struct {
				Content struct {
					DispatchState string `json:"dispatchState"`
				} `json:"content"`
			} `json:"entry"`
		}
		dispatchState := ""
		if err := json.Unmarshal(body, &wrapper); err != nil || len(wrapper.Entry) == 0 {
			// Try a flat response format when wrapper unmarshal fails
			// or succeeds with no entries (e.g. {"dispatchState":"DONE"}).
			var flat struct {
				DispatchState string `json:"dispatchState"`
			}
			if err2 := json.Unmarshal(body, &flat); err2 != nil {
				return fmt.Errorf("failed to decode splunk search job status: %w", err)
			}
			dispatchState = flat.DispatchState
		} else {
			dispatchState = wrapper.Entry[0].Content.DispatchState
		}

		switch dispatchState {
		case "DONE":
			return nil
		case "FAILED":
			return fmt.Errorf("splunk search job %s failed", sid)
		default:
			// Job still running or unknown state — fall through to wait and retry.
		}

		// Wait before polling again, respecting context cancellation.
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for search job: %w", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// getSearchResults retrieves the results of a completed Splunk search job
// and parses them into SIEMAlert objects.
func (s *SplunkSIEM) getSearchResults(ctx context.Context, sid string) ([]SIEMAlert, error) {
	reqURL := fmt.Sprintf("%s/services/search/jobs/%s/results", s.cfg.Endpoint, sid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search results request: %w", err)
	}

	s.setAuthHeaders(req)
	q := req.URL.Query()
	q.Set("output_mode", "json")
	req.URL.RawQuery = q.Encode()

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("splunk search results request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search results response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("splunk search results returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp splunkSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode splunk search results: %w", err)
	}

	alerts := s.parseSplunkResults(searchResp.Results)
	return alerts, nil
}

// parseSplunkResults converts Splunk search result rows into SIEMAlert objects.
// Splunk returns results as an array of objects where each key maps to an
// array of values (or a single value string depending on output_mode).
func (s *SplunkSIEM) parseSplunkResults(results []map[string]interface{}) []SIEMAlert {
	alerts := make([]SIEMAlert, 0, len(results))

	for _, row := range results {
		alert := SIEMAlert{}

		// Helper to extract a string field from a Splunk result.
		// Splunk JSON results can have values as either a string or
		// []interface{} (for multi-value fields).
		extractString := func(key string) string {
			val, ok := row[key]
			if !ok {
				return ""
			}
			switch v := val.(type) {
			case string:
				return v
			case []interface{}:
				if len(v) > 0 {
					if s, ok := v[0].(string); ok {
						return s
					}
				}
				return ""
			default:
				return fmt.Sprintf("%v", v)
			}
		}

		// Extract _raw field as the description fallback.
		alert.ID = extractString("_id")
		if alert.ID == "" {
			alert.ID = extractString("id")
		}

		// Timestamp.
		tsStr := extractString("timestamp")
		if tsStr != "" {
			if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
				alert.Timestamp = parsed
			} else if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				alert.Timestamp = parsed
			} else if parsed, err := time.Parse(splunkTimeFormat, tsStr); err == nil {
				alert.Timestamp = parsed
			} else {
				// Use _time (epoch) as fallback.
				alert.Timestamp = time.Now()
			}
		} else {
			// Try _time (Splunk's default time field, an epoch float).
			if timeVal := extractString("_time"); timeVal != "" {
				alert.Timestamp = time.Now() // best effort
			}
		}

		alert.Severity = extractString("severity")
		alert.Type = extractString("type")
		if alert.Type == "" {
			alert.Type = extractString("alert_type")
		}
		alert.Source = extractString("source")
		if alert.Source == "" {
			alert.Source = extractString("_source")
		}
		alert.Description = extractString("description")
		if alert.Description == "" {
			alert.Description = extractString("_raw")
		}

		// Experiment ID.
		expIDStr := extractString("experiment_id")
		if expIDStr != "" {
			if parsed, err := uuid.Parse(expIDStr); err == nil {
				alert.ExperimentID = parsed
			}
		}

		// Run ID.
		runIDStr := extractString("run_id")
		if runIDStr != "" {
			if parsed, err := uuid.Parse(runIDStr); err == nil {
				alert.RunID = parsed
			}
		}

		// Metadata — copy any remaining fields that aren't in the standard
		// alert schema into the metadata map.
		knownFields := map[string]bool{
			"id": true, "_id": true, "timestamp": true, "_time": true,
			"severity": true, "type": true, "alert_type": true,
			"source": true, "_source": true, "description": true, "_raw": true,
			"experiment_id": true, "run_id": true, "metadata": true,
		}
		metadata := make(map[string]interface{})
		for k, v := range row {
			if !knownFields[k] {
				metadata[k] = v
			}
		}
		if len(metadata) > 0 {
			alert.Metadata = metadata
		}

		alerts = append(alerts, alert)
	}

	return alerts
}

// withRetry executes the given operation with exponential backoff retries.
// It stops retrying on success, context cancellation, or after maxRetries
// attempts. The backoff delay is: 1s, 2s, 4s, 8s... capped at 30 seconds.
func (s *SplunkSIEM) withRetry(ctx context.Context, maxRetries int, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			s.logger.Debug("retrying splunk operation",
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

	return fmt.Errorf("splunk operation failed after %d retries: %w", maxRetries, lastErr)
}
