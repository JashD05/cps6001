package siem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// MockSIEM implements the SIEMConnector interface by communicating with
// a mock SIEM REST service. This is intended for development and testing
// environments where a real SIEM (Splunk, Elastic, QRadar, etc.) is not
// available.
type MockSIEM struct {
	cfg    SIEMConfig
	client *http.Client
	logger *zap.Logger
}

// NewMockSIEM creates a new MockSIEM connector with the provided configuration.
// The HTTP client is configured with the timeout from cfg and reasonable
// transport defaults.
func NewMockSIEM(cfg SIEMConfig, opts ...MockSIEMOption) *MockSIEM {
	m := &MockSIEM{
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
		opt(m)
	}

	return m
}

// MockSIEMOption is a functional option for configuring MockSIEM.
type MockSIEMOption func(*MockSIEM)

// WithMockLogger sets the logger for the MockSIEM connector.
func WithMockLogger(logger *zap.Logger) MockSIEMOption {
	return func(m *MockSIEM) {
		m.logger = logger.Named("mock_siem")
	}
}

// Connect validates that the mock SIEM endpoint is reachable by
// performing a health check. It retries on transient failures up
// to the configured MaxRetries.
func (m *MockSIEM) Connect(ctx context.Context) error {
	return m.withRetry(ctx, m.cfg.MaxRetries, func(ctx context.Context) error {
		return m.HealthCheck(ctx)
	})
}

// QueryAlerts queries the mock SIEM REST API for alerts matching the
// provided query parameters. The endpoint is:
//
//	GET {endpoint}/api/alerts
//
// Query parameters:
//   - from       — ISO8601 timestamp
//   - to         — ISO8601 timestamp
//   - alert_type — alert type filter
//   - severity   — severity filter
//   - source     — source filter
//   - experiment_id — experiment ID filter
//   - run_id     — run ID filter
//   - offset     — pagination offset
//   - limit      — pagination limit
func (m *MockSIEM) QueryAlerts(ctx context.Context, query AlertQuery) ([]SIEMAlert, error) {
	reqURL := fmt.Sprintf("%s/api/alerts", m.cfg.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create query alerts request: %w", err)
	}

	// Set query parameters.
	q := req.URL.Query()
	if !query.TimeRange.From.IsZero() {
		q.Set("from", query.TimeRange.From.Format(time.RFC3339))
	}
	if !query.TimeRange.To.IsZero() {
		q.Set("to", query.TimeRange.To.Format(time.RFC3339))
	}
	if query.AlertType != "" {
		q.Set("alert_type", query.AlertType)
	}
	if query.Severity != "" {
		q.Set("severity", query.Severity)
	}
	if query.Source != "" {
		q.Set("source", query.Source)
	}
	if query.ExperimentID.String() != "00000000-0000-0000-0000-000000000000" {
		q.Set("experiment_id", query.ExperimentID.String())
	}
	if query.RunID.String() != "00000000-0000-0000-0000-000000000000" {
		q.Set("run_id", query.RunID.String())
	}
	if query.Pagination.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", query.Pagination.Offset))
	}
	if query.Pagination.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", query.Pagination.Limit))
	}
	req.URL.RawQuery = q.Encode()

	m.setAuthHeaders(req)

	var alerts []SIEMAlert

	err = m.withRetry(ctx, m.cfg.MaxRetries, func(ctx context.Context) error {
		resp, err := m.client.Do(req)
		if err != nil {
			return fmt.Errorf("query alerts request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read query alerts response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("query alerts returned status %d: %s", resp.StatusCode, string(body))
		}

		if err := json.Unmarshal(body, &alerts); err != nil {
			return fmt.Errorf("failed to decode query alerts response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	m.logger.Debug("queried mock SIEM alerts",
		zap.Int("count", len(alerts)),
	)
	return alerts, nil
}

// SendAlert posts an alert to the mock SIEM REST API. The endpoint is:
//
//	POST {endpoint}/api/alerts
//
// The alert is sent as a JSON body.
func (m *MockSIEM) SendAlert(ctx context.Context, alert SIEMAlert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	reqURL := fmt.Sprintf("%s/api/alerts", m.cfg.Endpoint)

	return m.withRetry(ctx, m.cfg.MaxRetries, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create send alert request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		m.setAuthHeaders(req)

		resp, err := m.client.Do(req)
		if err != nil {
			return fmt.Errorf("send alert request failed: %w", err)
		}
		defer resp.Body.Close()

		// Drain the body to allow connection reuse.
		_, _ = io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("send alert returned status %d", resp.StatusCode)
		}

		m.logger.Debug("alert sent to mock SIEM",
			zap.String("alert_id", alert.ID),
			zap.String("alert_type", alert.Type),
		)
		return nil
	})
}

// HealthCheck verifies that the mock SIEM service is healthy by calling
// the /health endpoint.
func (m *MockSIEM) HealthCheck(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/health", m.cfg.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	m.setAuthHeaders(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mock SIEM health check failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain the body to allow connection reuse.
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mock SIEM health check returned status %d", resp.StatusCode)
	}

	m.logger.Debug("mock SIEM health check passed")
	return nil
}

// Close is a no-op for the mock SIEM connector since there are no
// persistent connections or resources to release beyond the HTTP client,
// which is garbage-collected automatically.
func (m *MockSIEM) Close() error {
	m.client.CloseIdleConnections()
	m.logger.Debug("mock SIEM connector closed")
	return nil
}

// setAuthHeaders sets the appropriate authentication headers on the request
// based on the SIEM configuration. It supports API key and basic auth.
func (m *MockSIEM) setAuthHeaders(req *http.Request) {
	if m.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	}
	if m.cfg.Username != "" && m.cfg.Password != "" {
		req.SetBasicAuth(m.cfg.Username, m.cfg.Password)
	}
}

// withRetry executes the given operation with exponential backoff retries.
// It stops retrying on success, context cancellation, or after maxRetries
// attempts. The backoff delay is: 1s, 2s, 4s, 8s... capped at 30 seconds.
func (m *MockSIEM) withRetry(ctx context.Context, maxRetries int, fn func(context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff: 1s, 2s, 4s, 8s...
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			m.logger.Debug("retrying SIEM operation",
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

		// Don't retry on context cancellation.
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w (last error: %v)", ctx.Err(), lastErr)
		}
	}

	return fmt.Errorf("SIEM operation failed after %d retries: %w", maxRetries, lastErr)
}
