package siem

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SIEMConnector defines the interface for interacting with SIEM systems.
// Implementations must be safe for concurrent use across goroutines.
type SIEMConnector interface {
	// Connect establishes a connection to the SIEM backend.
	Connect(ctx context.Context) error

	// QueryAlerts queries the SIEM for alerts matching the given query parameters.
	// Returns a slice of SIEMAlerts and any error encountered.
	QueryAlerts(ctx context.Context, query AlertQuery) ([]SIEMAlert, error)

	// SendAlert sends an alert to the SIEM for ingestion.
	SendAlert(ctx context.Context, alert SIEMAlert) error

	// HealthCheck verifies the SIEM backend is reachable and healthy.
	HealthCheck(ctx context.Context) error

	// Close releases any resources held by the connector.
	Close() error
}

// AlertQuery specifies parameters for querying SIEM alerts.
type AlertQuery struct {
	// TimeRange defines the window of time to search for alerts.
	TimeRange TimeRange `json:"time_range"`

	// AlertType filters alerts by type/category (e.g., "network_intrusion").
	AlertType string `json:"alert_type,omitempty"`

	// Severity filters alerts by severity level (e.g., "high", "critical").
	Severity string `json:"severity,omitempty"`

	// Source filters alerts by the originating source system.
	Source string `json:"source,omitempty"`

	// ExperimentID filters alerts associated with a specific experiment.
	ExperimentID uuid.UUID `json:"experiment_id,omitempty"`

	// RunID filters alerts associated with a specific experiment run.
	RunID uuid.UUID `json:"run_id,omitempty"`

	// Pagination controls result pagination.
	Pagination Pagination `json:"pagination"`
}

// TimeRange defines a time window for SIEM queries.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Pagination controls offset-based pagination for query results.
type Pagination struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// SIEMAlert represents a single alert from the SIEM system.
type SIEMAlert struct {
	// ID is the unique identifier of the alert in the SIEM.
	ID string `json:"id"`

	// Timestamp is when the alert was generated.
	Timestamp time.Time `json:"timestamp"`

	// Severity is the alert severity level (e.g., "low", "medium", "high", "critical").
	Severity string `json:"severity"`

	// Type categorises the alert (e.g., "network_intrusion", "privilege_escalation").
	Type string `json:"type"`

	// Source is the system or component that generated the alert.
	Source string `json:"source"`

	// Description provides a human-readable summary of the alert.
	Description string `json:"description"`

	// Metadata holds additional SIEM-specific key-value data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// ExperimentID links this alert to the originating experiment, if applicable.
	ExperimentID uuid.UUID `json:"experiment_id,omitempty"`

	// RunID links this alert to a specific experiment run, if applicable.
	RunID uuid.UUID `json:"run_id,omitempty"`
}

// SIEMConfig holds configuration for connecting to a SIEM system.
// This is distinct from the application-level config.SIEMConfig because
// it contains connector-specific fields like Username, Password, and Index.
type SIEMConfig struct {
	// Endpoint is the base URL of the SIEM API.
	Endpoint string `json:"endpoint"`

	// APIKey is an optional API key for authentication.
	APIKey string `json:"-"`

	// Username is an optional username for basic authentication.
	Username string `json:"username,omitempty"`

	// Password is an optional password for basic authentication.
	Password string `json:"-"`

	// Index specifies the SIEM index or database to query (e.g., Elasticsearch index).
	Index string `json:"index,omitempty"`

	// Timeout is the HTTP client timeout for SIEM requests.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxRetries is the maximum number of retry attempts for failed requests.
	MaxRetries int `json:"max_retries,omitempty"`
}

// Validate checks that the SIEMConfig has the minimum required fields.
func (c SIEMConfig) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("siem endpoint is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("siem timeout must be positive")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("siem max_retries must be non-negative")
	}
	return nil
}

// DefaultSIEMConfig returns a SIEMConfig with sensible defaults applied
// for any zero-valued fields.
func DefaultSIEMConfig(cfg SIEMConfig) SIEMConfig {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	return cfg
}

// NewSIEMConnector creates a new SIEMConnector based on the provider name.
// Supported providers: "mock" (returns MockSIEM).
// For any unrecognised provider, an error is returned.
func NewSIEMConnector(provider string, cfg SIEMConfig) (SIEMConnector, error) {
	cfg = DefaultSIEMConfig(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid siem config for provider %q: %w", provider, err)
	}

	switch provider {
	case "mock":
		return NewMockSIEM(cfg), nil
	case "":
		return nil, fmt.Errorf("SIEM provider must be specified; use \"mock\" for testing")
	default:
		return nil, fmt.Errorf("unsupported siem provider: %q (supported: mock)", provider)
	}
}
