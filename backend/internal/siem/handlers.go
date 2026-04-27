package siem

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler holds dependencies for SIEM HTTP handlers.
type Handler struct {
	connector SIEMConnector
	cfg       SIEMConfig
	logger    *zap.Logger
}

// NewHandler creates a new SIEM handler with the provided dependencies.
func NewHandler(connector SIEMConnector, cfg SIEMConfig, logger *zap.Logger) *Handler {
	return &Handler{
		connector: connector,
		cfg:       cfg,
		logger:    logger.Named("siem_handler"),
	}
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// SIEMStatusResponse represents the response for the SIEM status endpoint.
type SIEMStatusResponse struct {
	Connected bool              `json:"connected"`
	Provider  string            `json:"provider"`
	Endpoint  string            `json:"endpoint"`
	Health    string            `json:"health"`
	Latency   *time.Duration    `json:"latency,omitempty"`
	Error     string            `json:"error,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// TestConnectionResponse represents the response for the SIEM connection test endpoint.
type TestConnectionResponse struct {
	Success  bool          `json:"success"`
	Endpoint string        `json:"endpoint"`
	Latency  time.Duration `json:"latency"`
	Error    string        `json:"error,omitempty"`
}

// QueryAlertsRequest represents the request body for querying SIEM alerts.
type QueryAlertsRequest struct {
	From         string `json:"from" binding:"required"`
	To           string `json:"to" binding:"required"`
	AlertType    string `json:"alert_type,omitempty"`
	Severity     string `json:"severity,omitempty"`
	Source       string `json:"source,omitempty"`
	ExperimentID string `json:"experiment_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	Offset       int    `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

// QueryAlertsResponse represents the response for the SIEM alerts query endpoint.
type QueryAlertsResponse struct {
	Alerts []SIEMAlert `json:"alerts"`
	Total  int         `json:"total"`
	From   string      `json:"from"`
	To     string      `json:"to"`
}

// ExperimentAlertsResponse represents the response for the experiment alerts endpoint.
type ExperimentAlertsResponse struct {
	ExperimentID uuid.UUID   `json:"experiment_id"`
	RunID        uuid.UUID   `json:"run_id"`
	Alerts       []SIEMAlert `json:"alerts"`
	Total        int         `json:"total"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GetSIEMStatusHandler returns the current SIEM connection status and health.
// GET /api/v1/siem/status
func (h *Handler) GetSIEMStatusHandler(c *gin.Context) {
	resp := SIEMStatusResponse{
		Provider:  "mock",
		Endpoint:  h.cfg.Endpoint,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	// Attempt a health check to determine current connectivity.
	start := time.Now()
	err := h.connector.HealthCheck(c.Request.Context())
	latency := time.Since(start)

	if err != nil {
		resp.Connected = false
		resp.Health = "unhealthy"
		resp.Error = err.Error()
		h.logger.Warn("SIEM health check failed",
			zap.Error(err),
			zap.Duration("latency", latency),
		)
	} else {
		resp.Connected = true
		resp.Health = "healthy"
		resp.Latency = &latency
		h.logger.Debug("SIEM health check succeeded",
			zap.Duration("latency", latency),
		)
	}

	c.JSON(http.StatusOK, resp)
}

// TestSIEMConnectionHandler tests the connectivity to the configured SIEM
// by establishing a fresh connection and performing a health check.
// POST /api/v1/siem/test-connection
func (h *Handler) TestSIEMConnectionHandler(c *gin.Context) {
	resp := TestConnectionResponse{
		Endpoint: h.cfg.Endpoint,
	}

	// Attempt to connect (which internally performs a health check with retries).
	start := time.Now()
	err := h.connector.Connect(c.Request.Context())
	resp.Latency = time.Since(start)

	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		h.logger.Warn("SIEM connection test failed",
			zap.Error(err),
			zap.Duration("latency", resp.Latency),
		)
		c.JSON(http.StatusBadGateway, resp)
		return
	}

	resp.Success = true
	h.logger.Info("SIEM connection test succeeded",
		zap.Duration("latency", resp.Latency),
	)
	c.JSON(http.StatusOK, resp)
}

// QueryAlertsHandler executes a custom SIEM alert query based on the
// request parameters.
// POST /api/v1/siem/alerts/query
func (h *Handler) QueryAlertsHandler(c *gin.Context) {
	var req QueryAlertsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": fmt.Sprintf("Invalid request body: %s", err.Error()),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Parse time range.
	from, err := time.Parse(time.RFC3339, req.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_time_range",
			"message": fmt.Sprintf("Invalid 'from' timestamp (must be RFC3339): %s", err.Error()),
			"code":    http.StatusBadRequest,
		})
		return
	}

	to, err := time.Parse(time.RFC3339, req.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_time_range",
			"message": fmt.Sprintf("Invalid 'to' timestamp (must be RFC3339): %s", err.Error()),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Set defaults for pagination.
	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Build the alert query.
	query := AlertQuery{
		TimeRange: TimeRange{
			From: from,
			To:   to,
		},
		AlertType: req.AlertType,
		Severity:  req.Severity,
		Source:    req.Source,
		Pagination: Pagination{
			Offset: offset,
			Limit:  limit,
		},
	}

	// Parse optional experiment and run IDs.
	if req.ExperimentID != "" {
		expID, err := uuid.Parse(req.ExperimentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_experiment_id",
				"message": fmt.Sprintf("Invalid experiment_id: %s", err.Error()),
				"code":    http.StatusBadRequest,
			})
			return
		}
		query.ExperimentID = expID
	}

	if req.RunID != "" {
		runID, err := uuid.Parse(req.RunID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_run_id",
				"message": fmt.Sprintf("Invalid run_id: %s", err.Error()),
				"code":    http.StatusBadRequest,
			})
			return
		}
		query.RunID = runID
	}

	alerts, err := h.connector.QueryAlerts(c.Request.Context(), query)
	if err != nil {
		h.logger.Error("SIEM alert query failed",
			zap.Error(err),
			zap.String("from", req.From),
			zap.String("to", req.To),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "siem_query_failed",
			"message": fmt.Sprintf("Failed to query SIEM alerts: %s", err.Error()),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if alerts == nil {
		alerts = []SIEMAlert{}
	}

	c.JSON(http.StatusOK, QueryAlertsResponse{
		Alerts: alerts,
		Total:  len(alerts),
		From:   req.From,
		To:     req.To,
	})
}

// GetExperimentAlertsHandler retrieves all SIEM alerts associated with a
// specific experiment run. The run_id is taken from the URL parameter.
// GET /api/v1/siem/alerts/:run_id
func (h *Handler) GetExperimentAlertsHandler(c *gin.Context) {
	runIDStr := c.Param("run_id")
	if runIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "missing_run_id",
			"message": "Run ID is required.",
			"code":    http.StatusBadRequest,
		})
		return
	}

	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_run_id",
			"message": fmt.Sprintf("Invalid run ID format: %s", err.Error()),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Parse optional experiment_id query parameter.
	experimentIDStr := c.Query("experiment_id")
	var experimentID uuid.UUID
	if experimentIDStr != "" {
		experimentID, err = uuid.Parse(experimentIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_experiment_id",
				"message": fmt.Sprintf("Invalid experiment ID format: %s", err.Error()),
				"code":    http.StatusBadRequest,
			})
			return
		}
	}

	// Parse optional time range query parameters.
	// If not provided, default to the last 24 hours.
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	if fromStr := c.Query("from"); fromStr != "" {
		parsedFrom, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_from",
				"message": fmt.Sprintf("Invalid 'from' timestamp (must be RFC3339): %s", err.Error()),
				"code":    http.StatusBadRequest,
			})
			return
		}
		from = parsedFrom
	}

	if toStr := c.Query("to"); toStr != "" {
		parsedTo, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_to",
				"message": fmt.Sprintf("Invalid 'to' timestamp (must be RFC3339): %s", err.Error()),
				"code":    http.StatusBadRequest,
			})
			return
		}
		to = parsedTo
	}

	// Parse optional pagination.
	offset := 0
	limit := 100
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if v, err := parseInt(offsetStr); err == nil && v >= 0 {
			offset = v
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if v, err := parseInt(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	// Build and execute the query.
	query := AlertQuery{
		TimeRange: TimeRange{
			From: from,
			To:   to,
		},
		ExperimentID: experimentID,
		RunID:        runID,
		Pagination: Pagination{
			Offset: offset,
			Limit:  limit,
		},
	}

	alerts, err := h.connector.QueryAlerts(c.Request.Context(), query)
	if err != nil {
		h.logger.Error("failed to query SIEM alerts for experiment run",
			zap.String("run_id", runID.String()),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "siem_query_failed",
			"message": fmt.Sprintf("Failed to query SIEM alerts: %s", err.Error()),
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if alerts == nil {
		alerts = []SIEMAlert{}
	}

	resp := ExperimentAlertsResponse{
		ExperimentID: experimentID,
		RunID:        runID,
		Alerts:       alerts,
		Total:        len(alerts),
	}

	c.JSON(http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseInt safely parses an integer from a string, returning an error if
// the string is not a valid integer.
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	return v, nil
}

// Ensure Handler satisfies the expected interface patterns by providing
// a marshal helper for consistent error responses.
func siemErrorResponse(errorCode, message string, code int) json.RawMessage {
	resp, _ := json.Marshal(gin.H{
		"error":   errorCode,
		"message": message,
		"code":    code,
	})
	return resp
}
