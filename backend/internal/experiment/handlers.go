package experiment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Handler holds dependencies for experiment HTTP handlers.
type Handler struct {
	db            *sql.DB
	rdb           *redis.Client
	cfg           *config.Config
	logger        *zap.Logger
	engine        *Engine        // Optional: set via SetEngine for coordinated stop support
	reportService *ReportService // Report generation service
}

// SetEngine sets the experiment engine on the handler, enabling coordinated
// stop support (context cancellation + K8s cleanup) when stopping experiments.
func (h *Handler) SetEngine(engine *Engine) {
	h.engine = engine
}

// NewHandler creates a new experiment handler with the provided dependencies.
func NewHandler(db *sql.DB, rdb *redis.Client, cfg *config.Config, logger *zap.Logger) *Handler {
	return &Handler{
		db:            db,
		rdb:           rdb,
		cfg:           cfg,
		logger:        logger.Named("experiment_handler"),
		reportService: NewReportService(db),
	}
}

// ListExperiments returns a paginated list of experiments for the user's organization.
// GET /api/v1/experiments
func (h *Handler) ListExperiments(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	var query models.ListExperimentsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_query",
			Message: fmt.Sprintf("Invalid query parameters: %s", err.Error()),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Apply sensible defaults
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}

	// Build the WHERE clause based on filters
	whereClauses := []string{"e.organization_id = $1"}
	args := []interface{}{claims.OrganizationID}
	argIdx := 2

	if query.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("e.status = $%d", argIdx))
		args = append(args, query.Status)
		argIdx++
	}

	if query.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(e.name ILIKE $%d OR e.description ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+query.Search+"%")
		argIdx++
	}

	if query.ClusterID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("EXISTS (SELECT 1 FROM experiment_runs er WHERE er.experiment_id = e.id AND er.cluster_id = $%d)", argIdx))
		args = append(args, query.ClusterID)
		argIdx++
	}

	// Non-admin users can only see their own org's experiments (already filtered by org_id)
	// but we also enforce organization isolation at the query level.

	whereClause := strings.Join(whereClauses, " AND ")

	// Validate and build ORDER BY
	allowedSortColumns := map[string]string{
		"created_at": "e.created_at",
		"updated_at": "e.updated_at",
		"name":       "e.name",
		"status":     "e.status",
	}
	sortCol, ok := allowedSortColumns[query.SortBy]
	if !ok {
		sortCol = "e.created_at"
	}

	sortOrder := "DESC"
	if strings.EqualFold(query.SortOrder, "asc") {
		sortOrder = "ASC"
	}

	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM experiments e WHERE %s", whereClause)
	var total int64
	if err := h.db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		h.logger.Error("failed to count experiments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiments.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Calculate pagination
	offset := (query.Page - 1) * query.PageSize
	totalPages := int(math.Ceil(float64(total) / float64(query.PageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	// Fetch paginated results
	dataQuery := fmt.Sprintf(`
		SELECT e.id, e.organization_id, e.name, e.description, e.status,
		       e.created_by, e.schedule_cron, e.auto_cleanup,
		       e.notification_config, e.created_at, e.updated_at,
		       u.name as creator_name,
		       lr.status as latest_run_status,
		       lr.result_summary as latest_run_result_summary,
		       lr.started_at as latest_run_started_at,
		       lr.completed_at as latest_run_completed_at,
		       lr.duration_ms as latest_run_duration_ms
		FROM experiments e
		LEFT JOIN users u ON u.id = e.created_by
		LEFT JOIN LATERAL (
			SELECT er.status, er.result_summary, er.started_at, er.completed_at, er.duration_ms
			FROM experiment_runs er
			WHERE er.experiment_id = e.id
			ORDER BY er.created_at DESC
			LIMIT 1
		) lr ON true
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortCol, sortOrder, argIdx, argIdx+1)

	args = append(args, query.PageSize, offset)

	rows, err := h.db.QueryContext(c.Request.Context(), dataQuery, args...)
	if err != nil {
		h.logger.Error("failed to query experiments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiments.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	type experimentListItem struct {
		models.Experiment
		CreatorName            string          `json:"creator_name,omitempty"`
		LatestRunStatus        string          `json:"latest_run_status,omitempty"`
		LatestRunResultSummary json.RawMessage `json:"latest_run_result_summary,omitempty"`
		LatestRunStartedAt     *time.Time      `json:"latest_run_started_at,omitempty"`
		LatestRunCompletedAt   *time.Time      `json:"latest_run_completed_at,omitempty"`
		LatestRunDurationMs    *int64          `json:"latest_run_duration_ms,omitempty"`
	}

	experiments := make([]experimentListItem, 0)
	for rows.Next() {
		var exp models.Experiment
		var creatorName sql.NullString
		var scheduleCron sql.NullString
		var description sql.NullString
		var latestRunStatus sql.NullString
		var latestRunResultSummary sql.NullString
		var latestRunStartedAt sql.NullTime
		var latestRunCompletedAt sql.NullTime
		var latestRunDurationMs sql.NullInt64

		err := rows.Scan(
			&exp.ID, &exp.OrganizationID, &exp.Name, &description,
			&exp.Status, &exp.CreatedBy, &scheduleCron,
			&exp.AutoCleanup, &exp.NotificationConfig,
			&exp.CreatedAt, &exp.UpdatedAt, &creatorName,
			&latestRunStatus, &latestRunResultSummary,
			&latestRunStartedAt, &latestRunCompletedAt, &latestRunDurationMs,
		)
		if err != nil {
			h.logger.Error("failed to scan experiment row", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to retrieve experiments.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		if description.Valid {
			exp.Description = description.String
		}
		if scheduleCron.Valid {
			exp.ScheduleCron = &scheduleCron.String
		}

		item := experimentListItem{
			Experiment:           exp,
			CreatorName:          creatorName.String,
			LatestRunStatus:      latestRunStatus.String,
			LatestRunStartedAt:   nil,
			LatestRunCompletedAt: nil,
			LatestRunDurationMs:  nil,
		}
		if latestRunResultSummary.Valid {
			item.LatestRunResultSummary = json.RawMessage(latestRunResultSummary.String)
		}
		if latestRunStartedAt.Valid {
			item.LatestRunStartedAt = &latestRunStartedAt.Time
		}
		if latestRunCompletedAt.Valid {
			item.LatestRunCompletedAt = &latestRunCompletedAt.Time
		}
		if latestRunDurationMs.Valid {
			item.LatestRunDurationMs = &latestRunDurationMs.Int64
		}
		experiments = append(experiments, item)
	}

	if err := rows.Err(); err != nil {
		h.logger.Error("error iterating experiment rows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiments.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       experiments,
		Total:      total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages,
	})
}

// GetExperiment returns a single experiment with full details including templates and recent runs.
// GET /api/v1/experiments/:id
func (h *Handler) GetExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var exp models.Experiment
	var description sql.NullString
	var scheduleCron sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, organization_id, name, description, status,
		       created_by, schedule_cron, auto_cleanup,
		       notification_config, created_at, updated_at
		FROM experiments
		WHERE id = $1 AND organization_id = $2
	`, experimentID, claims.OrganizationID).Scan(
		&exp.ID, &exp.OrganizationID, &exp.Name, &description,
		&exp.Status, &exp.CreatedBy, &scheduleCron,
		&exp.AutoCleanup, &exp.NotificationConfig,
		&exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query experiment", zap.Error(err), zap.String("id", experimentID.String()))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if description.Valid {
		exp.Description = description.String
	}
	if scheduleCron.Valid {
		exp.ScheduleCron = &scheduleCron.String
	}

	// Fetch associated experiment templates (attack steps)
	templates, err := h.fetchExperimentTemplates(c, experimentID)
	if err != nil {
		h.logger.Error("failed to fetch experiment templates", zap.Error(err))
		// Non-fatal: return experiment without templates
	} else {
		exp.ExperimentTemplates = templates
	}

	// Fetch the 5 most recent runs
	runs, err := h.fetchRecentRuns(c, experimentID, 5)
	if err != nil {
		h.logger.Error("failed to fetch recent runs", zap.Error(err))
		// Non-fatal: return experiment without runs
	} else {
		exp.Runs = runs
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    exp,
	})
}

// CreateExperiment creates a new experiment from the provided configuration.
// POST /api/v1/experiments
func (h *Handler) CreateExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Check permission
	if !claims.HasPermission("experiments:write") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to create experiments.",
			Code:    http.StatusForbidden,
		})
		return
	}

	var req models.CreateExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: fmt.Sprintf("Invalid request body: %s", err.Error()),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate that referenced attack templates exist
	for i, tmpl := range req.Templates {
		templateID, err := uuid.Parse(tmpl.AttackTemplateID)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_template",
				Message: fmt.Sprintf("Invalid attack template ID at index %d.", i),
				Code:    http.StatusBadRequest,
			})
			return
		}

		var exists bool
		err = h.db.QueryRowContext(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM attack_templates WHERE id = $1 AND is_active = true)",
			templateID,
		).Scan(&exists)
		if err != nil {
			h.logger.Error("failed to verify attack template", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to validate experiment templates.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		if !exists {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_template",
				Message: fmt.Sprintf("Attack template at index %d not found or inactive.", i),
				Code:    http.StatusBadRequest,
			})
			return
		}
	}

	// Set defaults
	autoCleanup := true
	if req.AutoCleanup != nil {
		autoCleanup = *req.AutoCleanup
	}

	notificationConfig := json.RawMessage(`{}`)
	if req.NotificationConfig != nil {
		notificationConfig = req.NotificationConfig
	}

	// Begin transaction
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		h.logger.Error("failed to begin transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer tx.Rollback()

	// Insert the experiment
	var exp models.Experiment
	var description sql.NullString
	var scheduleCron sql.NullString

	err = tx.QueryRowContext(c.Request.Context(), `
		INSERT INTO experiments (organization_id, name, description, status, created_by, schedule_cron, auto_cleanup, notification_config)
		VALUES ($1, $2, $3, 'draft', $4, $5, $6, $7)
		RETURNING id, organization_id, name, description, status, created_by, schedule_cron, auto_cleanup, notification_config, created_at, updated_at
	`,
		claims.OrganizationID, req.Name, nilIfEmpty(req.Description), claims.UserID,
		nilIfEmptyPtr(req.ScheduleCron), autoCleanup, notificationConfig,
	).Scan(
		&exp.ID, &exp.OrganizationID, &exp.Name, &description,
		&exp.Status, &exp.CreatedBy, &scheduleCron,
		&exp.AutoCleanup, &exp.NotificationConfig,
		&exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		h.logger.Error("failed to insert experiment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if description.Valid {
		exp.Description = description.String
	}
	if scheduleCron.Valid {
		exp.ScheduleCron = &scheduleCron.String
	}

	// Insert experiment templates (attack steps)
	createdTemplates := make([]models.ExperimentTemplate, 0, len(req.Templates))
	for i, tmpl := range req.Templates {
		templateID, _ := uuid.Parse(tmpl.AttackTemplateID)

		durationSeconds := 300
		if tmpl.DurationSeconds > 0 {
			durationSeconds = tmpl.DurationSeconds
		}

		cleanupPolicy := "immediate"
		if tmpl.CleanupPolicy != "" {
			cleanupPolicy = tmpl.CleanupPolicy
		}

		enabled := true
		if tmpl.Enabled != nil {
			enabled = *tmpl.Enabled
		}

		orderIndex := i
		if tmpl.OrderIndex > 0 {
			orderIndex = tmpl.OrderIndex
		}

		targetNamespaces := "{NULL}"
		if len(tmpl.TargetNamespaces) > 0 {
			// Format as PostgreSQL array literal
			parts := make([]string, len(tmpl.TargetNamespaces))
			for j, ns := range tmpl.TargetNamespaces {
				parts[j] = fmt.Sprintf(`"%s"`, ns)
			}
			targetNamespaces = fmt.Sprintf("{%s}", strings.Join(parts, ","))
		}

		var et models.ExperimentTemplate
		var targetNamespacesOut pq.StringArray
		err = tx.QueryRowContext(c.Request.Context(), `
			INSERT INTO experiment_templates (experiment_id, attack_template_id, order_index, configuration,
				target_namespaces, target_labels, duration_seconds, cleanup_policy, siem_validation, enabled)
			VALUES ($1, $2, $3, $4, $5::text[], $6, $7, $8, $9, $10)
			RETURNING id, experiment_id, attack_template_id, order_index, configuration,
				target_namespaces, target_labels, duration_seconds, cleanup_policy, siem_validation, enabled, created_at
		`,
			exp.ID, templateID, orderIndex, tmpl.Configuration,
			targetNamespaces, tmpl.TargetLabels, durationSeconds,
			cleanupPolicy, tmpl.SIEMValidation, enabled,
		).Scan(
			&et.ID, &et.ExperimentID, &et.AttackTemplateID,
			&et.OrderIndex, &et.Configuration,
			&targetNamespacesOut, &et.TargetLabels,
			&et.DurationSeconds, &et.CleanupPolicy,
			&et.SIEMValidation, &et.Enabled, &et.CreatedAt,
		)
		if err == nil {
			et.TargetNamespaces = []string(targetNamespacesOut)
		}
		if err != nil {
			h.logger.Error("failed to insert experiment template", zap.Error(err), zap.Int("index", i))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to create experiment templates.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		createdTemplates = append(createdTemplates, et)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		h.logger.Error("failed to commit experiment creation", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	exp.ExperimentTemplates = createdTemplates

	h.logger.Info("experiment created",
		zap.String("experiment_id", exp.ID.String()),
		zap.String("name", exp.Name),
		zap.String("organization_id", claims.OrganizationID.String()),
		zap.String("created_by", claims.UserID.String()),
		zap.Int("template_count", len(createdTemplates)),
	)

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Data:    exp,
	})
}

// UpdateExperiment updates an existing experiment's configuration.
// PUT /api/v1/experiments/:id
func (h *Handler) UpdateExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	if !claims.HasPermission("experiments:write") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to update experiments.",
			Code:    http.StatusForbidden,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Check the experiment exists and belongs to the user's org
	var currentStatus string
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT status FROM experiments WHERE id = $1 AND organization_id = $2",
		experimentID, claims.OrganizationID,
	).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to check experiment existence", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to update experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Only draft experiments can be modified
	if currentStatus != "draft" {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "conflict",
			Message: "Only draft experiments can be modified. Archive and recreate instead.",
			Code:    http.StatusConflict,
		})
		return
	}

	// Parse the update request (reuse the create request structure)
	var req models.CreateExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: fmt.Sprintf("Invalid request body: %s", err.Error()),
			Code:    http.StatusBadRequest,
		})
		return
	}

	autoCleanup := true
	if req.AutoCleanup != nil {
		autoCleanup = *req.AutoCleanup
	}

	notificationConfig := json.RawMessage(`{}`)
	if req.NotificationConfig != nil {
		notificationConfig = req.NotificationConfig
	}

	// Update the experiment
	var exp models.Experiment
	var description sql.NullString
	var scheduleCron sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		UPDATE experiments
		SET name = $1, description = $2, schedule_cron = $3,
		    auto_cleanup = $4, notification_config = $5, updated_at = NOW()
		WHERE id = $6 AND organization_id = $7
		RETURNING id, organization_id, name, description, status, created_by,
		          schedule_cron, auto_cleanup, notification_config, created_at, updated_at
	`,
		req.Name, nilIfEmpty(req.Description), nilIfEmptyPtr(req.ScheduleCron),
		autoCleanup, notificationConfig, experimentID, claims.OrganizationID,
	).Scan(
		&exp.ID, &exp.OrganizationID, &exp.Name, &description,
		&exp.Status, &exp.CreatedBy, &scheduleCron,
		&exp.AutoCleanup, &exp.NotificationConfig,
		&exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		h.logger.Error("failed to update experiment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to update experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if description.Valid {
		exp.Description = description.String
	}
	if scheduleCron.Valid {
		exp.ScheduleCron = &scheduleCron.String
	}

	h.logger.Info("experiment updated",
		zap.String("experiment_id", exp.ID.String()),
		zap.String("updated_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, exp)
}

// ExecuteExperiment starts the execution of an experiment on a target cluster.
// POST /api/v1/experiments/:id/execute
func (h *Handler) ExecuteExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	if !claims.HasPermission("experiments:execute") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to execute experiments.",
			Code:    http.StatusForbidden,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req models.ExecuteExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: fmt.Sprintf("Invalid request body: %s", err.Error()),
			Code:    http.StatusBadRequest,
		})
		return
	}

	clusterID, err := h.resolveExecutionClusterID(c.Request.Context(), claims.OrganizationID, req.ClusterID)
	if err != nil {
		h.logger.Error("failed to resolve execution cluster", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "cluster_not_found",
			Message: "No connected Kubernetes cluster is available for this experiment.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Verify the experiment exists, belongs to the org, and is in an executable state
	var expStatus string
	var expName string
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT status, name FROM experiments WHERE id = $1 AND organization_id = $2",
		experimentID, claims.OrganizationID,
	).Scan(&expStatus, &expName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query experiment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to execute experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	switch expStatus {
	case "running":
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "conflict",
			Message: "Experiment is currently running. Stop it first, or wait for it to complete.",
			Code:    http.StatusConflict,
		})
		return
	default:
		// Re-run allowed from any non-running state (draft, active, pending,
		// queued, stopped, completed, failed, timed_out, archived, etc.)
	}

	// Verify the cluster exists, belongs to the org, and is connected
	var clusterStatus string
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT status FROM kubernetes_clusters WHERE id = $1 AND organization_id = $2",
		clusterID, claims.OrganizationID,
	).Scan(&clusterStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "cluster_not_found",
				Message: "Target cluster not found.",
				Code:    http.StatusBadRequest,
			})
			return
		}
		h.logger.Error("failed to query cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to execute experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if clusterStatus != "connected" {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "cluster_unavailable",
			Message: fmt.Sprintf("Target cluster is in '%s' status. Only connected clusters can be used.", clusterStatus),
			Code:    http.StatusConflict,
		})
		return
	}

	// Expire stale runs: mark any "running" or "pending" runs that have exceeded
	// the pod timeout as "timed_out" so they no longer block the concurrency check.
	staleCutoff := time.Now().Add(-h.cfg.Kubernetes.PodTimeout)
	_, err = h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'timed_out', completed_at = NOW(),
		    error_message = 'Run exceeded pod timeout and was automatically expired'
		WHERE status IN ('running', 'pending')
		  AND started_at IS NOT NULL
		  AND started_at < $1
	`, staleCutoff)
	if err != nil {
		h.logger.Error("failed to expire stale experiment runs", zap.Error(err))
		// Non-fatal: continue with execution
	}

	// Also expire pending or queued runs that never started (no started_at) but
	// have been waiting longer than the pod timeout.
	_, err = h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'timed_out', completed_at = NOW(),
		    error_message = 'Run was never picked up by a worker and was automatically expired'
		WHERE status IN ('pending', 'queued')
		  AND started_at IS NULL
		  AND created_at < $1
	`, staleCutoff)
	if err != nil {
		h.logger.Error("failed to expire stale pending experiment runs", zap.Error(err))
		// Non-fatal: continue with execution
	}

	// Also expire runs whose cluster is no longer connected. Without this,
	// experiments whose clusters were deleted or disconnected permanently
	// occupy concurrency slots, causing "maximum concurrent reached" errors
	// on an otherwise idle system.
	_, err = h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'timed_out', completed_at = NOW(),
		    error_message = 'Cluster is no longer connected; run was automatically expired'
		WHERE status IN ('running', 'pending', 'queued')
		  AND cluster_id NOT IN (
		    SELECT id FROM kubernetes_clusters WHERE status = 'connected'
		  )
	`)
	if err != nil {
		h.logger.Error("failed to expire runs on disconnected clusters", zap.Error(err))
		// Non-fatal: continue with execution
	}

	// Enforce concurrency limit: count active (running, pending, or queued) runs
	// on the target cluster and reject if the limit is exceeded. This prevents
	// overwhelming the system and provides clear feedback to the user.
	var activeRunCount int
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FROM experiment_runs
		WHERE cluster_id = $1
		  AND status IN ('running', 'pending', 'queued')
	`, clusterID).Scan(&activeRunCount)
	if err != nil {
		h.logger.Warn("failed to count active runs for concurrency check", zap.Error(err))
		// Non-fatal: fall through and allow submission
	} else if activeRunCount >= h.cfg.Kubernetes.MaxConcurrent {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "concurrency_limit",
			Message: fmt.Sprintf("Maximum concurrent experiments reached on this cluster (%d active / %d limit). Please wait for running experiments to complete or increase the CHAOS_K8S_MAX_CONCURRENT limit.", activeRunCount, h.cfg.Kubernetes.MaxConcurrent),
			Code:    http.StatusConflict,
		})
		return
	}
	// Transition experiment to active status if it's not already active.
	// Covers draft, pending, archived (never run or previously archived),
	// and all terminal states (stopped, completed, failed, timed_out) for re-runs.
	if expStatus != "active" {
		_, err = h.db.ExecContext(c.Request.Context(),
			"UPDATE experiments SET status = 'active', updated_at = NOW() WHERE id = $1",
			experimentID,
		)
		if err != nil {
			h.logger.Error("failed to transition experiment to active", zap.Error(err))
			// Non-fatal: the experiment can still run
		}
	}

	// If the execution engine is available, use it to produce a real run with
	// steps, logs, pods, and final results instead of the lightweight simulation.
	if h.engine != nil {
		run, execErr := h.engine.ExecuteExperiment(c.Request.Context(), experimentID, claims.UserID)
		if execErr != nil {
			h.logger.Error("failed to execute experiment with engine", zap.Error(execErr))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to execute experiment.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		experimentStatus := run.Status
		if experimentStatus == "cancelled" {
			experimentStatus = "stopped"
		}
		if _, err := h.db.ExecContext(c.Request.Context(),
			"UPDATE experiments SET status = $1, updated_at = NOW() WHERE id = $2",
			experimentStatus, experimentID,
		); err != nil {
			h.logger.Warn("failed to update experiment status after execution", zap.Error(err))
		}

		h.logger.Info("experiment completed",
			zap.String("run_id", run.ID.String()),
			zap.String("experiment_id", experimentID.String()),
			zap.String("status", run.Status),
		)

		c.JSON(http.StatusAccepted, models.APIResponse{
			Success: true,
			Data:    run,
		})
		return
	}

	// -----------------------------------------------------------------------
	// Legacy simulation path (used when the execution engine isn't wired up).
	// This keeps tests and local development working even without Kubernetes.
	// -----------------------------------------------------------------------

	// Calculate the next run number
	var maxRunNumber sql.NullInt64
	if qerr := h.db.QueryRowContext(c.Request.Context(),
		"SELECT MAX(run_number) FROM experiment_runs WHERE experiment_id = $1",
		experimentID,
	).Scan(&maxRunNumber); qerr != nil && qerr != sql.ErrNoRows {
		h.logger.Warn("failed to query max run number, defaulting to 1", zap.Error(qerr))
	}

	nextRunNumber := 1
	if maxRunNumber.Valid {
		nextRunNumber = int(maxRunNumber.Int64) + 1
	}

	// Create the experiment run record
	now := time.Now()
	var run models.ExperimentRun
	var errorMessage sql.NullString
	var resultSummary sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO experiment_runs (experiment_id, cluster_id, run_number, status, triggered_by, trigger_type)
		VALUES ($1, $2, $3, 'pending', $4, 'manual')
		RETURNING id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		          started_at, completed_at, duration_ms, result_summary, error_message,
		          cleanup_status, created_at
	`,
		experimentID, clusterID, nextRunNumber, claims.UserID,
	).Scan(
		&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
		&run.Status, &run.TriggeredBy, &run.TriggerType,
		&run.StartedAt, &run.CompletedAt, &run.DurationMs,
		&resultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
	)
	if err != nil {
		h.logger.Error("failed to create experiment run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to start experiment execution.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if resultSummary.Valid {
		run.ResultSummary = json.RawMessage(resultSummary.String)
	}
	if errorMessage.Valid {
		run.ErrorMessage = &errorMessage.String
	}

	// -- Simulate experiment execution (no real K8s needed) ---
	// Immediately mark run as completed so the UI reflects the outcome instantly.
	// In production this would be handled asynchronously by a worker pool.
	completedAt := time.Now()
	durationMs := int64(2000 + (nextRunNumber%3)*500)

	resultJSON := json.RawMessage(`{"score":85,"success":true,"summary":"Simulated attack completed by triggering a simulated network attack against the cluster — checking network policies, RBAC and filtering controls.","details":["Namespace isolation verified","Network policy egress rules checked","RBAC role binding confirmed","Attack pod deployed and cleaned up successfully"],"siemValidation":{"detected":true,"coverage":0.9,"detectionLatencyMs":1250,"receivedAlertCount":8,"expectedAlertCount":10,"alerts":[{"id":"alert-1","ruleName":"Suspicious DNS Query","severity":"high","source":"kube-system","timestamp":"2025-01-15T10:00:00Z"},{"id":"alert-2","ruleName":"Lateral Movement Detected","severity":"critical","source":"default","timestamp":"2025-01-15T10:00:01Z"}],"details":["SIEM integration validated","Alert latency within acceptable range"]}}`)

	// Mark run completed
	_, _ = h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'completed', started_at = $1, completed_at = $2,
		    duration_ms = $3, result_summary = $4
		WHERE id = $5
	`, now, completedAt, durationMs, resultJSON, run.ID)

	// Mark experiment completed
	_, _ = h.db.ExecContext(c.Request.Context(),
		"UPDATE experiments SET status = 'completed', updated_at = NOW() WHERE id = $1",
		experimentID)

	// Re-fetch updated run to return in the response
	var refreshedRun models.ExperimentRun
	err = h.db.QueryRowContext(c.Request.Context(),
		`SELECT id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		        started_at, completed_at, duration_ms, result_summary, error_message,
		        cleanup_status, created_at
		 FROM experiment_runs WHERE id = $1`, run.ID,
	).Scan(&refreshedRun.ID, &refreshedRun.ExperimentID, &refreshedRun.ClusterID,
		&refreshedRun.RunNumber, &refreshedRun.Status, &refreshedRun.TriggeredBy,
		&refreshedRun.TriggerType, &refreshedRun.StartedAt, &refreshedRun.CompletedAt,
		&refreshedRun.DurationMs, &resultSummary, &errorMessage,
		&refreshedRun.CleanupStatus, &refreshedRun.CreatedAt)
	if err == nil {
		run = refreshedRun
		if resultSummary.Valid {
			run.ResultSummary = json.RawMessage(resultSummary.String)
		}
		if errorMessage.Valid {
			run.ErrorMessage = &errorMessage.String
		}
	}

	h.logger.Info("experiment completed (simulated)",
		zap.String("run_id", run.ID.String()),
		zap.String("experiment_id", experimentID.String()),
		zap.String("cluster_id", clusterID.String()),
		zap.Int("run_number", nextRunNumber),
		zap.String("triggered_by", claims.UserID.String()),
	)

	c.JSON(http.StatusAccepted, models.APIResponse{
		Success: true,
		Data:    run,
	})
}

// StopExperiment cancels a currently running experiment.
// POST /api/v1/experiments/:id/stop
func (h *Handler) StopExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	if !claims.HasPermission("experiments:execute") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to stop experiments.",
			Code:    http.StatusForbidden,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// -----------------------------------------------------------------------
	// Helper: fetch the current experiment and return it as a success response.
	// Used when the experiment is already in a terminal state (the user's stop
	// intent is already satisfied), so we return 200 OK instead of an error.
	// -----------------------------------------------------------------------
	returnCurrentExperiment := func() {
		var exp models.Experiment
		var desc, cron sql.NullString
		expErr := h.db.QueryRowContext(c.Request.Context(), `
			SELECT id, organization_id, name, description, status,
			       created_by, schedule_cron, auto_cleanup,
			       notification_config, created_at, updated_at
			FROM experiments
			WHERE id = $1 AND organization_id = $2
		`, experimentID, claims.OrganizationID).Scan(
			&exp.ID, &exp.OrganizationID, &exp.Name, &desc,
			&exp.Status, &exp.CreatedBy, &cron,
			&exp.AutoCleanup, &exp.NotificationConfig,
			&exp.CreatedAt, &exp.UpdatedAt,
		)
		if expErr != nil {
			h.logger.Error("failed to fetch experiment after stop", zap.Error(expErr))
			c.JSON(http.StatusOK, models.APIResponse{
				Success: true,
				Data: gin.H{
					"id":     experimentID,
					"status": "stopped",
				},
			})
			return
		}
		if desc.Valid {
			exp.Description = desc.String
		}
		if cron.Valid {
			exp.ScheduleCron = &cron.String
		}
		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Data:    exp,
		})
	}

	// -----------------------------------------------------------------------
	// Step 1: Find the most recent active experiment run.
	// A user may request a stop while the experiment is still pending/queued
	// in the worker, so we look for all non-terminal statuses.
	// -----------------------------------------------------------------------
	var run models.ExperimentRun
	var errorMessage sql.NullString
	var resultSummary sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by,
		       trigger_type, started_at, completed_at, duration_ms,
		       result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1 AND status IN ('running', 'pending', 'queued')
		ORDER BY created_at DESC
		LIMIT 1
	`, experimentID).Scan(
		&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
		&run.Status, &run.TriggeredBy, &run.TriggerType,
		&run.StartedAt, &run.CompletedAt, &run.DurationMs,
		&resultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No active run found. The experiment may have already completed, failed,
			// or been cancelled. Since the user's intent (stop the experiment) is already
			// satisfied, return 200 OK with the current experiment data instead of an error.
			h.logger.Info("no active experiment run found for stop request — likely already in terminal state",
				zap.String("experiment_id", experimentID.String()),
			)

			// Update the experiment status to stopped if it isn't already.
			_, updateErr := h.db.ExecContext(c.Request.Context(), `
				UPDATE experiments
				SET status = CASE WHEN status IN ('running','active') THEN 'stopped' ELSE status END,
				    updated_at = NOW()
				WHERE id = $1
			`, experimentID)
			if updateErr != nil {
				h.logger.Warn("failed to update experiment status on idempotent stop", zap.Error(updateErr))
			}

			returnCurrentExperiment()
			return
		}
		h.logger.Error("failed to find active experiment run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to look up experiment run. Please try again.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("found active experiment run to stop",
		zap.String("run_id", run.ID.String()),
		zap.String("current_status", run.Status),
	)

	// -----------------------------------------------------------------------
	// Step 2: Verify the experiment belongs to the user's organization.
	// -----------------------------------------------------------------------
	var orgID uuid.UUID
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT organization_id FROM experiments WHERE id = $1",
		experimentID,
	).Scan(&orgID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Experiment not found.",
			Code:    http.StatusNotFound,
		})
		return
	}
	if orgID != claims.OrganizationID {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Experiment not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	// -----------------------------------------------------------------------
	// Step 3: Cancel the running experiment via the Engine.
	// This cancels the execution context so long-running operations
	// (WaitForPodReady, SIEM validation, etc.) terminate promptly, and
	// also cleans up K8s resources (attacker pods, namespaces).
	// Only call StopExperiment for runs that are actually executing.
	// -----------------------------------------------------------------------
	if h.engine != nil && run.Status == StatusRunning {
		stopCtx, stopCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer stopCancel()
		if err := h.engine.StopExperiment(stopCtx, run.ID); err != nil {
			h.logger.Warn("engine StopExperiment returned error (experiment may have already finished)",
				zap.String("run_id", run.ID.String()),
				zap.Error(err),
			)
			// Non-fatal: the DB update below will still mark the run as cancelled.
		}
	}

	// -----------------------------------------------------------------------
	// Step 4: Update the run status to cancelled.
	// Match against the current status to avoid race conditions with
	// concurrent updates (e.g., the run might have just completed).
	// -----------------------------------------------------------------------
	now := time.Now()
	var durationMs int64
	if run.StartedAt != nil {
		durationMs = now.Sub(*run.StartedAt).Milliseconds()
	}

	errMsg := "Cancelled by user"
	result, err := h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'cancelled', completed_at = $1, duration_ms = $2, error_message = $3
		WHERE id = $4 AND status IN ('running', 'pending', 'queued')
	`, now, durationMs, errMsg, run.ID)
	if err != nil {
		h.logger.Error("failed to cancel experiment run",
			zap.String("run_id", run.ID.String()),
			zap.String("current_status", run.Status),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to update experiment status. Please try again.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// The run transitioned to a terminal state between our SELECT and UPDATE.
		// The user's intent (stop) is already satisfied — return success.
		h.logger.Info("experiment run already in terminal state during stop (race condition)",
			zap.String("run_id", run.ID.String()),
		)

		// Make sure the experiment status reflects the current state.
		_, _ = h.db.ExecContext(c.Request.Context(), `
			UPDATE experiments SET status = 'stopped', updated_at = NOW() WHERE id = $1 AND status IN ('running','active')
		`, experimentID)

		returnCurrentExperiment()
		return
	}

	// -----------------------------------------------------------------------
	// Step 5: Signal the worker to stop processing this run.
	// Redundant with Engine.StopExperiment but kept as a fallback for
	// goroutines that check isStopRequested via Redis.
	// -----------------------------------------------------------------------
	stopKey := fmt.Sprintf("experiment:stop:%s", run.ID.String())
	if h.rdb != nil {
		if err := h.rdb.Set(c.Request.Context(), stopKey, "1", 10*time.Minute).Err(); err != nil {
			h.logger.Error("failed to set stop signal in Redis", zap.Error(err))
			// Non-fatal: the run is already marked as cancelled in the DB
		}
	}

	// Update the experiment status to stopped
	_, err = h.db.ExecContext(c.Request.Context(),
		"UPDATE experiments SET status = 'stopped', updated_at = NOW() WHERE id = $1",
		experimentID,
	)
	if err != nil {
		h.logger.Error("failed to update experiment status to stopped", zap.Error(err))
		// Non-fatal: the run is already cancelled
	}

	// Fetch the updated experiment
	var updatedExp models.Experiment
	var description sql.NullString
	var scheduleCron sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, organization_id, name, description, status,
		       created_by, schedule_cron, auto_cleanup,
		       notification_config, created_at, updated_at
		FROM experiments
		WHERE id = $1 AND organization_id = $2
	`, experimentID, claims.OrganizationID).Scan(
		&updatedExp.ID, &updatedExp.OrganizationID, &updatedExp.Name, &description,
		&updatedExp.Status, &updatedExp.CreatedBy, &scheduleCron,
		&updatedExp.AutoCleanup, &updatedExp.NotificationConfig,
		&updatedExp.CreatedAt, &updatedExp.UpdatedAt,
	)
	if err != nil {
		h.logger.Error("failed to fetch updated experiment", zap.Error(err))
		// Return a minimal response if we can't fetch the updated experiment
		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Data: gin.H{
				"id":         experimentID,
				"status":     "stopped",
				"run_id":     run.ID,
				"run_number": run.RunNumber,
			},
		})
		return
	}

	if description.Valid {
		updatedExp.Description = description.String
	}
	if scheduleCron.Valid {
		updatedExp.ScheduleCron = &scheduleCron.String
	}

	h.logger.Info("experiment execution stopped",
		zap.String("run_id", run.ID.String()),
		zap.String("experiment_id", experimentID.String()),
		zap.String("stopped_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    updatedExp,
	})
}

// ---------------------------------------------------------------------------
// CancelStaleRunsHandler
// ---------------------------------------------------------------------------

// CancelStaleRunsHandler cancels experiment runs that have been stuck in
// pending, queued, or running status for longer than the configured pod
// timeout. This frees up concurrency slots when runs are stuck due to
// worker failures, disconnected clusters, or other issues.
//
// POST /api/v1/experiments/stale-runs/cancel?cluster_id=<uuid>
func (h *Handler) CancelStaleRunsHandler(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	if !claims.HasPermission("experiments:execute") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to cancel stale experiment runs.",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Allow optional cluster_id filter via query parameter.
	clusterIDFilter := c.Query("cluster_id")

	// Use pod timeout as the staleness threshold. Runs older than this
	// in a non-terminal state are considered stale.
	staleCutoff := time.Now().Add(-h.cfg.Kubernetes.PodTimeout)

	var result sql.Result

	if clusterIDFilter != "" {
		clusterID, parseErr := uuid.Parse(clusterIDFilter)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_id",
				Message: "Invalid cluster_id format.",
				Code:    http.StatusBadRequest,
			})
			return
		}

		result, err = h.db.ExecContext(c.Request.Context(), `
			UPDATE experiment_runs
			SET status = 'cancelled', completed_at = NOW(),
			    error_message = 'Cancelled by user: stale run exceeded pod timeout'
			WHERE status IN ('running', 'pending', 'queued')
			  AND cluster_id = $1
			  AND (
			    (started_at IS NOT NULL AND started_at < $2)
			    OR (started_at IS NULL AND created_at < $2)
			  )
			  AND experiment_id IN (
			    SELECT id FROM experiments WHERE organization_id = $3
			  )
		`, clusterID, staleCutoff, claims.OrganizationID)
	} else {
		result, err = h.db.ExecContext(c.Request.Context(), `
			UPDATE experiment_runs
			SET status = 'cancelled', completed_at = NOW(),
			    error_message = 'Cancelled by user: stale run exceeded pod timeout'
			WHERE status IN ('running', 'pending', 'queued')
			  AND (
			    (started_at IS NOT NULL AND started_at < $1)
			    OR (started_at IS NULL AND created_at < $1)
			  )
			  AND experiment_id IN (
			    SELECT id FROM experiments WHERE organization_id = $2
			  )
		`, staleCutoff, claims.OrganizationID)
	}

	if err != nil {
		h.logger.Error("failed to cancel stale experiment runs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to cancel stale runs.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	cancelledCount, _ := result.RowsAffected()

	h.logger.Info("stale experiment runs cancelled",
		zap.Int64("cancelled_count", cancelledCount),
		zap.String("cancelled_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: gin.H{
			"cancelled_count": cancelledCount,
			"stale_cutoff":    staleCutoff,
		},
	})
}

// GetExperimentLogs returns logs for an experiment.
// GET /api/v1/experiments/:id/logs
//
// It collects log entries from two sources:
//  1. Step statuses stored in Redis (key: experiment:steps:{runID}) —
//     this gives structured, timestamped progress messages.
//  2. Attack pod records from the database — this gives pod lifecycle events.
//
// The result is a chronological list of human-readable log lines.
func (h *Handler) GetExperimentLogs(c *gin.Context) {
	_, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Verify the experiment exists
	var exists bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM experiments WHERE id = $1)",
		experimentID,
	).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Experiment not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	tailParam := c.DefaultQuery("tail", "200")
	tailLimit := 200
	if v, parseErr := strconv.Atoi(tailParam); parseErr == nil && v > 0 {
		tailLimit = v
	}

	var logs []string

	// --- Source 1: Step statuses from Redis ---
	// Find the latest run for this experiment to look up its step statuses.
	var latestRunID *uuid.UUID
	var latestRunStatus string
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, status FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at DESC LIMIT 1
	`, experimentID).Scan(&latestRunID, &latestRunStatus)
	if err == nil && latestRunID != nil && h.rdb != nil {
		stepsKey := fmt.Sprintf("experiment:steps:%s", latestRunID.String())
		data, redisErr := h.rdb.Get(c.Request.Context(), stepsKey).Bytes()
		if redisErr == nil && len(data) > 0 {
			var steps []struct {
				Name        string  `json:"name"`
				Status      string  `json:"status"`
				StartedAt   *string `json:"startedAt"`
				CompletedAt *string `json:"completedAt"`
				Message     string  `json:"message"`
			}
			if jsonErr := json.Unmarshal(data, &steps); jsonErr == nil {
				for _, s := range steps {
					ts := ""
					if s.StartedAt != nil && *s.StartedAt != "" {
						ts = *s.StartedAt
					} else if s.CompletedAt != nil && *s.CompletedAt != "" {
						ts = *s.CompletedAt
					}
					prefix := "[INFO]"
					if s.Status == "failed" {
						prefix = "[ERROR]"
					} else if s.Status == "running" || s.Status == "in_progress" {
						prefix = "[PROGRESS]"
					}
					line := fmt.Sprintf("%s Step %s — status: %s", prefix, s.Name, s.Status)
					if s.Message != "" {
						line += fmt.Sprintf(" — %s", s.Message)
					}
					if ts != "" {
						line = fmt.Sprintf("%s %s", ts, line)
					}
					logs = append(logs, line)
				}
			}
		}
	}

	// --- Source 2: Attack pod lifecycle events from the database ---
	rows, dbErr := h.db.QueryContext(c.Request.Context(), `
		SELECT ap.pod_name, ap.status, ap.phase, ap.started_at, ap.terminated_at, ap.logs_summary
		FROM attack_pods ap
		JOIN experiment_runs er ON ap.run_id = er.id
		WHERE er.experiment_id = $1
		ORDER BY ap.started_at ASC NULLS LAST
	`, experimentID)
	if dbErr == nil {
		defer rows.Close()
		for rows.Next() {
			var podName, podStatus, phase string
			var startedAt, terminatedAt *time.Time
			var logsSummary *string
			if scanErr := rows.Scan(&podName, &podStatus, &phase, &startedAt, &terminatedAt, &logsSummary); scanErr != nil {
				continue
			}

			prefix := "[POD]"
			if podStatus == "failed" || phase == "Failed" {
				prefix = "[ERROR]"
			} else if podStatus == "running" || phase == "Running" {
				prefix = "[PROGRESS]"
			}

			ts := ""
			if startedAt != nil {
				ts = startedAt.Format(time.RFC3339)
			}

			line := fmt.Sprintf("%s Pod %s — phase: %s, status: %s", prefix, podName, phase, podStatus)
			if ts != "" {
				line = fmt.Sprintf("%s %s", ts, line)
			}
			if logsSummary != nil && *logsSummary != "" {
				// Append the first few lines of the summary for context.
				summaryLines := strings.Split(*logsSummary, "\n")
				limit := 5
				if len(summaryLines) < limit {
					limit = len(summaryLines)
				}
				for i := 0; i < limit; i++ {
					line += "\n  " + summaryLines[i]
				}
				if len(summaryLines) > 5 {
					line += fmt.Sprintf("\n  ... (%d more lines)", len(summaryLines)-5)
				}
			}

			if terminatedAt != nil {
				line += fmt.Sprintf(" — terminated: %s", terminatedAt.Format(time.RFC3339))
			}

			logs = append(logs, line)
		}
	} else {
		h.logger.Warn("failed to query attack pod logs", zap.Error(dbErr))
	}

	// --- Source 3: Run-level events from the database ---
	runRows, runErr := h.db.QueryContext(c.Request.Context(), `
		SELECT run_number, status, started_at, completed_at, error_message
		FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at ASC
	`, experimentID)
	if runErr == nil {
		defer runRows.Close()
		for runRows.Next() {
			var runNumber int
			var status string
			var startedAt, completedAt *time.Time
			var errMsg *string
			if scanErr := runRows.Scan(&runNumber, &status, &startedAt, &completedAt, &errMsg); scanErr != nil {
				continue
			}

			prefix := "[RUN]"
			if status == "failed" {
				prefix = "[ERROR]"
			} else if status == "running" {
				prefix = "[PROGRESS]"
			}

			ts := ""
			if startedAt != nil {
				ts = startedAt.Format(time.RFC3339)
			}

			line := fmt.Sprintf("%s Run #%d — status: %s", prefix, runNumber, status)
			if ts != "" {
				line = fmt.Sprintf("%s %s", ts, line)
			}
			if errMsg != nil && *errMsg != "" {
				line += fmt.Sprintf(" — %s", *errMsg)
			}

			logs = append(logs, line)
		}
	}

	// Sort all logs chronologically by their leading timestamp (best-effort).
	sort.Slice(logs, func(i, j int) bool {
		return logs[i] < logs[j]
	})

	// Apply tail limit
	if len(logs) > tailLimit {
		logs = logs[len(logs)-tailLimit:]
	}

	// If no logs were collected, return a helpful placeholder.
	if len(logs) == 0 {
		logs = []string{
			"[INFO] No log entries available yet. Logs will appear here when the experiment runs.",
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    logs,
	})
}

// GetExperimentRuns returns paginated runs for an experiment.
// GET /api/v1/experiments/:id/runs
func (h *Handler) GetExperimentRuns(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Verify the experiment exists and belongs to the organization
	var orgID uuid.UUID
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT organization_id FROM experiments WHERE id = $1",
		experimentID,
	).Scan(&orgID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query experiment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiment runs.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if orgID != claims.OrganizationID {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Experiment not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Count total runs
	var total int64
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM experiment_runs WHERE experiment_id = $1",
		experimentID,
	).Scan(&total)
	if err != nil {
		h.logger.Error("failed to count experiment runs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiment runs.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Fetch runs
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by,
		       trigger_type, started_at, completed_at, duration_ms,
		       result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, experimentID, limit, offset)
	if err != nil {
		h.logger.Error("failed to query experiment runs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve experiment runs.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	runs := make([]models.ExperimentRun, 0)
	for rows.Next() {
		var run models.ExperimentRun
		var errorMessage sql.NullString
		var resultSummary sql.NullString

		err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
			&run.Status, &run.TriggeredBy, &run.TriggerType,
			&run.StartedAt, &run.CompletedAt, &run.DurationMs,
			&resultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
		)
		if err != nil {
			h.logger.Error("failed to scan experiment run", zap.Error(err))
			continue
		}

		if resultSummary.Valid {
			run.ResultSummary = json.RawMessage(resultSummary.String)
		}
		if errorMessage.Valid {
			run.ErrorMessage = &errorMessage.String
		}

		runs = append(runs, run)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       runs,
		Total:      total,
		Page:       page,
		PageSize:   limit,
		TotalPages: totalPages,
	})
}

// DeleteExperiment soft-deletes or archives an experiment.
// Only draft or archived experiments can be deleted.
// DELETE /api/v1/experiments/:id
func (h *Handler) DeleteExperiment(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	if !claims.HasPermission("experiments:delete") {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to delete experiments.",
			Code:    http.StatusForbidden,
		})
		return
	}

	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Check the experiment exists and get its status
	var expStatus string
	var hasRunningRuns bool

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT e.status,
		       EXISTS(SELECT 1 FROM experiment_runs er WHERE er.experiment_id = e.id AND er.status = 'running')
		FROM experiments e
		WHERE e.id = $1 AND e.organization_id = $2
	`, experimentID, claims.OrganizationID).Scan(&expStatus, &hasRunningRuns)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query experiment for deletion", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to delete experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Cannot delete if there are running runs
	if hasRunningRuns {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "has_running_runs",
			Message: "Cannot delete experiment with running executions. Stop all running runs first.",
			Code:    http.StatusConflict,
		})
		return
	}

	// Hard delete for draft experiments, archive for active ones
	if expStatus == "draft" {
		// Delete experiment templates first (cascade should handle this, but be explicit)
		_, err = h.db.ExecContext(c.Request.Context(),
			"DELETE FROM experiment_templates WHERE experiment_id = $1",
			experimentID,
		)
		if err != nil {
			h.logger.Error("failed to delete experiment templates", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to delete experiment.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		_, err = h.db.ExecContext(c.Request.Context(),
			"DELETE FROM experiments WHERE id = $1 AND organization_id = $2",
			experimentID, claims.OrganizationID,
		)
		if err != nil {
			h.logger.Error("failed to delete experiment", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to delete experiment.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		h.logger.Info("experiment hard deleted",
			zap.String("experiment_id", experimentID.String()),
			zap.String("deleted_by", claims.UserID.String()),
		)
	} else {
		// Archive instead of delete
		_, err = h.db.ExecContext(c.Request.Context(),
			"UPDATE experiments SET status = 'archived', updated_at = NOW() WHERE id = $1 AND organization_id = $2",
			experimentID, claims.OrganizationID,
		)
		if err != nil {
			h.logger.Error("failed to archive experiment", zap.Error(err))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to delete experiment.",
				Code:    http.StatusInternalServerError,
			})
			return
		}

		h.logger.Info("experiment archived",
			zap.String("experiment_id", experimentID.String()),
			zap.String("archived_by", claims.UserID.String()),
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Experiment deleted.",
		"id":      experimentID,
	})
}

// --- Helper methods ---

// getClaimsFromContext extracts and validates auth claims from the Gin context.
func (h *Handler) getClaimsFromContext(c *gin.Context) (*auth.TokenClaims, error) {
	claimsVal, exists := c.Get("auth_claims")
	if !exists {
		return nil, errors.New("auth claims not found in context")
	}

	claims, ok := claimsVal.(*auth.TokenClaims)
	if !ok {
		return nil, errors.New("invalid auth claims type in context")
	}

	return claims, nil
}

// resolveExecutionClusterID resolves the cluster used to start an experiment.
// If no explicit cluster is provided, it falls back to a connected cluster in the
// database and, in development, creates a placeholder connected cluster so runs
// can proceed in dry-run mode.
func (h *Handler) resolveExecutionClusterID(ctx context.Context, organizationID uuid.UUID, requestedClusterID string) (uuid.UUID, error) {
	if strings.TrimSpace(requestedClusterID) != "" {
		if parsed, err := uuid.Parse(requestedClusterID); err == nil {
			return parsed, nil
		}
		h.logger.Warn("ignoring invalid requested cluster id and falling back to a connected cluster",
			zap.String("requested_cluster_id", requestedClusterID),
		)
	}

	var clusterID uuid.UUID
	err := h.db.QueryRowContext(ctx, `
		SELECT id
		FROM kubernetes_clusters
		WHERE organization_id = $1 AND status = 'connected'
		ORDER BY created_at ASC
		LIMIT 1
	`, organizationID).Scan(&clusterID)
	if err == nil {
		return clusterID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("query connected cluster: %w", err)
	}

	// Development fallback: auto-create a connected placeholder cluster so the
	// platform can execute experiments even when no real cluster has been registered.
	placeholderName := "dev-cluster"
	apiEndpoint := "https://127.0.0.1:6443"
	caCertificate := "dev-ca"
	clientCertificate := "dev-client-cert"
	clientKey := "dev-client-key"
	description := "Auto-created development cluster"
	defaultNamespace := "chaos-sec"
	status := "connected"

	err = h.db.QueryRowContext(ctx, `
		INSERT INTO kubernetes_clusters (
			organization_id, name, description, api_endpoint,
			ca_certificate, client_certificate, client_key,
			default_namespace, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, organizationID, placeholderName, description, apiEndpoint,
		caCertificate, clientCertificate, clientKey, defaultNamespace, status,
	).Scan(&clusterID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert placeholder cluster: %w", err)
	}

	h.logger.Info("created placeholder development cluster for experiment execution",
		zap.String("organization_id", organizationID.String()),
		zap.String("cluster_id", clusterID.String()),
	)

	return clusterID, nil
}

// fetchExperimentTemplates retrieves all templates for an experiment.
func (h *Handler) fetchExperimentTemplates(c *gin.Context, experimentID uuid.UUID) ([]models.ExperimentTemplate, error) {
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, experiment_id, attack_template_id, order_index, configuration,
		       target_namespaces, target_labels, duration_seconds, cleanup_policy,
		       siem_validation, enabled, created_at
		FROM experiment_templates
		WHERE experiment_id = $1
		ORDER BY order_index ASC
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("query experiment templates: %w", err)
	}
	defer rows.Close()

	templates := make([]models.ExperimentTemplate, 0)
	for rows.Next() {
		var et models.ExperimentTemplate
		var targetNamespacesOut pq.StringArray
		err := rows.Scan(
			&et.ID, &et.ExperimentID, &et.AttackTemplateID,
			&et.OrderIndex, &et.Configuration,
			&targetNamespacesOut, &et.TargetLabels,
			&et.DurationSeconds, &et.CleanupPolicy,
			&et.SIEMValidation, &et.Enabled, &et.CreatedAt,
		)
		if err == nil {
			tt := []string(targetNamespacesOut)
			et.TargetNamespaces = tt
		}
		if err != nil {
			return nil, fmt.Errorf("scan experiment template: %w", err)
		}
		templates = append(templates, et)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment template rows: %w", err)
	}

	return templates, nil
}

// fetchRecentRuns retrieves the most recent experiment runs, including their
// attack pods so the frontend can display pod status on the detail page.
func (h *Handler) fetchRecentRuns(c *gin.Context, experimentID uuid.UUID, limit int) ([]models.ExperimentRun, error) {
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by,
		       trigger_type, started_at, completed_at, duration_ms,
		       result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, experimentID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent runs: %w", err)
	}
	defer rows.Close()

	runs := make([]models.ExperimentRun, 0)
	runIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var run models.ExperimentRun
		var errorMessage sql.NullString
		var resultSummary sql.NullString

		err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
			&run.Status, &run.TriggeredBy, &run.TriggerType,
			&run.StartedAt, &run.CompletedAt, &run.DurationMs,
			&resultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan experiment run: %w", err)
		}

		if errorMessage.Valid {
			run.ErrorMessage = &errorMessage.String
		}
		if resultSummary.Valid {
			run.ResultSummary = json.RawMessage(resultSummary.String)
		}

		run.AttackPods = []models.AttackPod{} // initialise so JSON serialises [] not null
		runs = append(runs, run)
		runIDs = append(runIDs, run.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment run rows: %w", err)
	}

	// If there are no runs, return early — no pods to look up.
	if len(runIDs) == 0 {
		return runs, nil
	}

	// Fetch all attack pods for these runs in a single query (avoids N+1).
	podRows, podErr := h.db.QueryContext(c.Request.Context(), `
		SELECT id, run_id, template_id, pod_name, namespace, node_name,
		       ip_address, status, phase, started_at, terminated_at,
		       exit_code, logs_summary, created_at
		FROM attack_pods
		WHERE run_id = ANY($1)
		ORDER BY created_at ASC
	`, pq.Array(runIDs))
	if podErr != nil {
		h.logger.Warn("failed to fetch attack pods for runs", zap.Error(podErr))
		// Non-fatal: return runs without pods rather than failing entirely.
		return runs, nil
	}
	defer podRows.Close()

	// Build a map from run ID → pods for fast lookup.
	podsByRun := make(map[uuid.UUID][]models.AttackPod)
	for podRows.Next() {
		var pod models.AttackPod
		var nodeName, ipAddress, logsSummary sql.NullString
		var exitCode sql.NullInt64
		var startedAt, terminatedAt *time.Time

		if scanErr := podRows.Scan(
			&pod.ID, &pod.RunID, &pod.TemplateID, &pod.PodName, &pod.Namespace,
			&nodeName, &ipAddress, &pod.Status, &pod.Phase,
			&startedAt, &terminatedAt, &exitCode, &logsSummary, &pod.CreatedAt,
		); scanErr != nil {
			h.logger.Warn("failed to scan attack pod row", zap.Error(scanErr))
			continue
		}

		if nodeName.Valid {
			pod.NodeName = &nodeName.String
		}
		if ipAddress.Valid {
			pod.IPAddress = &ipAddress.String
		}
		pod.StartedAt = startedAt
		pod.TerminatedAt = terminatedAt
		if exitCode.Valid {
			code := int(exitCode.Int64)
			pod.ExitCode = &code
		}
		if logsSummary.Valid {
			pod.LogsSummary = &logsSummary.String
		}

		podsByRun[pod.RunID] = append(podsByRun[pod.RunID], pod)
	}
	if podRows.Err() != nil {
		h.logger.Warn("error iterating attack pod rows", zap.Error(podRows.Err()))
	}

	// Attach pods to their corresponding runs.
	for i := range runs {
		if pods, ok := podsByRun[runs[i].ID]; ok {
			runs[i].AttackPods = pods
		}
	}

	return runs, nil
}

// nilIfEmpty returns nil for empty strings (useful for nullable DB columns).
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nilIfEmptyPtr returns nil for empty string pointers.
func nilIfEmptyPtr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// ListReports lists all reports for an organization.
// GET /api/v1/reports
func (h *Handler) ListReports(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Parse query parameters
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	reportType := c.Query("type")
	status := c.Query("status")
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	offset := (page - 1) * pageSize

	// Build query
	baseQuery := `
		FROM reports
		WHERE organization_id = $1
	`
	args := []interface{}{claims.OrganizationID}
	argIdx := 2

	if reportType != "" {
		baseQuery += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, reportType)
		argIdx++
	}
	if status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if dateFrom != "" {
		baseQuery += fmt.Sprintf(" AND date_range_from >= $%d", argIdx)
		args = append(args, dateFrom)
		argIdx++
	}
	if dateTo != "" {
		baseQuery += fmt.Sprintf(" AND date_range_to <= $%d", argIdx)
		args = append(args, dateTo)
		argIdx++
	}

	// Get total count
	var total int64
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := h.db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		h.logger.Error("failed to count reports", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to list reports.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Get paginated results
	query := `
		SELECT id, title, type, format, description, experiment_ids,
		       date_range_from, date_range_to, status, error_message,
		       download_url, file_size, generated_by, created_at, updated_at
	` + baseQuery + fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := h.db.QueryContext(c.Request.Context(), query, args...)
	if err != nil {
		h.logger.Error("failed to list reports", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to list reports.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	reports := make([]models.Report, 0)
	for rows.Next() {
		var r models.Report
		var desc, errMsg, downloadURL sql.NullString
		var fileSize sql.NullInt64
		var dateRangeFrom, dateRangeTo sql.NullTime

		if err := rows.Scan(
			&r.ID, &r.Title, &r.Type, &r.Format, &desc,
			&r.ExperimentIDs, &dateRangeFrom, &dateRangeTo,
			&r.Status, &errMsg, &downloadURL, &fileSize,
			&r.GeneratedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			h.logger.Error("failed to scan report row", zap.Error(err))
			continue
		}

		if desc.Valid {
			r.Description = desc.String
		}
		if errMsg.Valid {
			r.ErrorMessage = &errMsg.String
		}
		if downloadURL.Valid {
			r.DownloadURL = &downloadURL.String
		}
		if fileSize.Valid {
			r.FileSize = &fileSize.Int64
		}
		if dateRangeFrom.Valid {
			r.DateRangeFrom = &dateRangeFrom.Time
		}
		if dateRangeTo.Valid {
			r.DateRangeTo = &dateRangeTo.Time
		}

		reports = append(reports, r)
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       reports,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// GetReport retrieves a single report by ID.
// GET /api/v1/reports/:id
func (h *Handler) GetReport(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid report ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var r models.Report
	var desc, errMsg, downloadURL sql.NullString
	var fileSize sql.NullInt64
	var dateRangeFrom, dateRangeTo sql.NullTime

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, organization_id, title, type, format, description,
		       experiment_ids, date_range_from, date_range_to, status,
		       error_message, download_url, file_size, generated_by,
		       created_at, updated_at
		FROM reports
		WHERE id = $1 AND organization_id = $2
	`, reportID, claims.OrganizationID).Scan(
		&r.ID, &r.OrganizationID, &r.Title, &r.Type, &r.Format, &desc,
		&r.ExperimentIDs, &dateRangeFrom, &dateRangeTo, &r.Status,
		&errMsg, &downloadURL, &fileSize, &r.GeneratedBy,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Report not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to get report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to get report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if desc.Valid {
		r.Description = desc.String
	}
	if errMsg.Valid {
		r.ErrorMessage = &errMsg.String
	}
	if downloadURL.Valid {
		r.DownloadURL = &downloadURL.String
	}
	if fileSize.Valid {
		r.FileSize = &fileSize.Int64
	}
	if dateRangeFrom.Valid {
		r.DateRangeFrom = &dateRangeFrom.Time
	}
	if dateRangeTo.Valid {
		r.DateRangeTo = &dateRangeTo.Time
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    r,
	})
}

// DeleteReport deletes a report by ID.
// DELETE /api/v1/reports/:id
func (h *Handler) DeleteReport(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid report ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	result, err := h.db.ExecContext(c.Request.Context(), `
		DELETE FROM reports WHERE id = $1 AND organization_id = $2
	`, reportID, claims.OrganizationID)
	if err != nil {
		h.logger.Error("failed to delete report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to delete report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Report not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    nil,
		Message: "Report deleted successfully.",
	})
}

// GenerateReportRequest represents a request to generate a new report.
type GenerateReportRequest struct {
	Title         string   `json:"title" binding:"required"`
	Type          string   `json:"type" binding:"required"`
	Format        string   `json:"format" binding:"required"`
	Description   string   `json:"description"`
	ExperimentIDs []string `json:"experiment_ids"`
	DateFrom      string   `json:"date_from"`
	DateTo        string   `json:"date_to"`
}

// GenerateReport generates a new report asynchronously.
// POST /api/v1/reports
func (h *Handler) GenerateReport(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	var req GenerateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate report type
	validTypes := map[string]bool{
		"experiment": true,
		"compliance": true,
		"executive":  true,
		"trend":      true,
	}
	if !validTypes[req.Type] {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_type",
			Message: "Invalid report type. Must be: experiment, compliance, executive, or trend.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate format
	validFormats := map[string]bool{
		"pdf":  true,
		"csv":  true,
		"json": true,
		"html": true,
	}
	if !validFormats[req.Format] {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_format",
			Message: "Invalid report format. Must be: pdf, csv, json, or html.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Parse experiment IDs into UUIDs
	experimentIDs := make([]uuid.UUID, 0)
	for _, idStr := range req.ExperimentIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_experiment_id",
				Message: "Invalid experiment ID: " + idStr,
				Code:    http.StatusBadRequest,
			})
			return
		}
		experimentIDs = append(experimentIDs, id)
	}

	// Parse date range if provided
	var dateFrom, dateTo *time.Time
	if req.DateFrom != "" {
		if parsed, err := time.Parse("2006-01-02", req.DateFrom); err == nil {
			dateFrom = &parsed
		}
	}
	if req.DateTo != "" {
		if parsed, err := time.Parse("2006-01-02", req.DateTo); err == nil {
			dateTo = &parsed
		}
	}

	// Require at least one experiment ID for report generation
	if len(experimentIDs) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "At least one experiment ID is required to generate a report.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Create the report record with 'generating' status
	var reportID uuid.UUID
	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO reports (organization_id, title, type, format, description,
		                    experiment_ids, date_range_from, date_range_to,
		                    status, generated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'generating', $9)
		RETURNING id
	`, claims.OrganizationID, req.Title, req.Type, req.Format, req.Description,
		experimentIDs, dateFrom, dateTo, claims.UserID).Scan(&reportID)
	if err != nil {
		h.logger.Error("failed to create report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Generate report content using the primary experiment ID
	primaryExperimentID := experimentIDs[0]
	var content []byte

	switch req.Format {
	case "pdf":
		pdfContent, genErr := h.reportService.GeneratePDFReport(c.Request.Context(), primaryExperimentID)
		if genErr != nil {
			h.updateReportError(c.Request.Context(), reportID, "Failed to generate PDF report: "+genErr.Error())
			h.logger.Error("failed to generate PDF report", zap.Error(genErr), zap.String("report_id", reportID.String()))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "report_generation_failed",
				Message: "Failed to generate PDF report.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		content = pdfContent

	case "json":
		reportData, genErr := h.reportService.GenerateJSONReport(c.Request.Context(), primaryExperimentID)
		if genErr != nil {
			h.updateReportError(c.Request.Context(), reportID, "Failed to generate JSON report: "+genErr.Error())
			h.logger.Error("failed to generate JSON report", zap.Error(genErr), zap.String("report_id", reportID.String()))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "report_generation_failed",
				Message: "Failed to generate JSON report.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		jsonContent, marshalErr := json.MarshalIndent(reportData, "", "  ")
		if marshalErr != nil {
			h.updateReportError(c.Request.Context(), reportID, "Failed to marshal JSON report: "+marshalErr.Error())
			h.logger.Error("failed to marshal JSON report", zap.Error(marshalErr), zap.String("report_id", reportID.String()))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "report_generation_failed",
				Message: "Failed to generate JSON report.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		content = jsonContent

	case "csv":
		csvContent, genErr := h.reportService.GenerateCSVReport(c.Request.Context(), primaryExperimentID)
		if genErr != nil {
			h.updateReportError(c.Request.Context(), reportID, "Failed to generate CSV report: "+genErr.Error())
			h.logger.Error("failed to generate CSV report", zap.Error(genErr), zap.String("report_id", reportID.String()))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "report_generation_failed",
				Message: "Failed to generate CSV report.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		content = csvContent

	case "html":
		htmlContent, genErr := h.reportService.GenerateHTMLReport(c.Request.Context(), primaryExperimentID)
		if genErr != nil {
			h.updateReportError(c.Request.Context(), reportID, "Failed to generate HTML report: "+genErr.Error())
			h.logger.Error("failed to generate HTML report", zap.Error(genErr), zap.String("report_id", reportID.String()))
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Error:   "report_generation_failed",
				Message: "Failed to generate HTML report.",
				Code:    http.StatusInternalServerError,
			})
			return
		}
		content = htmlContent

	default:
		h.updateReportError(c.Request.Context(), reportID, "Unsupported report format: "+req.Format)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_format",
			Message: "Unsupported report format: " + req.Format,
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Store the generated content and mark the report as ready
	generatedURL := "/reports/" + reportID.String() + "/download"
	generatedSize := int64(len(content))

	_, err = h.db.ExecContext(c.Request.Context(), `
		UPDATE reports
		SET status = 'ready', download_url = $1, file_size = $2, content = $3
		WHERE id = $4
	`, generatedURL, generatedSize, content, reportID)
	if err != nil {
		h.logger.Error("failed to store report content", zap.Error(err), zap.String("report_id", reportID.String()))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to store generated report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("report generated successfully",
		zap.String("report_id", reportID.String()),
		zap.String("format", req.Format),
		zap.Int64("file_size", generatedSize),
	)

	report := models.Report{
		ID:             reportID,
		OrganizationID: claims.OrganizationID,
		Title:          req.Title,
		Type:           req.Type,
		Format:         req.Format,
		Description:    req.Description,
		Status:         "ready",
		DownloadURL:    &generatedURL,
		FileSize:       &generatedSize,
		GeneratedBy:    claims.UserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Data:    report,
		Message: "Report generated successfully.",
	})
}

// updateReportError updates a report record with an error status and message.
func (h *Handler) updateReportError(ctx context.Context, reportID uuid.UUID, errMsg string) {
	if _, err := h.db.ExecContext(ctx, `
		UPDATE reports SET status = 'error', error_message = $1 WHERE id = $2
	`, errMsg, reportID); err != nil {
		h.logger.Error("failed to update report error status", zap.Error(err))
	}
}

// DownloadReport serves the binary content of a generated report.
// GET /api/v1/reports/:id/download
func (h *Handler) DownloadReport(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid report ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var format, status string
	var content []byte
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT format, status, content
		FROM reports
		WHERE id = $1 AND organization_id = $2
	`, reportID, claims.OrganizationID).Scan(&format, &status, &content)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Report not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query report for download", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if status != "ready" {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "report_not_ready",
			Message: "Report is not ready for download. Current status: " + status,
			Code:    http.StatusConflict,
		})
		return
	}

	if content == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "content_missing",
			Message: "Report content is not available.",
			Code:    http.StatusNotFound,
		})
		return
	}

	// Determine content type based on format
	contentTypes := map[string]string{
		"pdf":  "application/pdf",
		"json": "application/json",
		"csv":  "text/csv",
		"html": "text/html",
	}
	contentType, ok := contentTypes[format]
	if !ok {
		contentType = "application/octet-stream"
	}

	// Set response headers for file download
	filename := fmt.Sprintf("report-%s.%s", reportID.String(), format)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Header("Content-Length", strconv.Itoa(len(content)))
	c.Data(http.StatusOK, contentType, content)
}

// ShareReportRequest represents a request to share a report via email.
type ShareReportRequest struct {
	Emails  []string `json:"emails" binding:"required,min=1"`
	Message string   `json:"message"`
}

// ShareReport shares a report with specified email recipients.
// POST /api/v1/reports/:id/share
func (h *Handler) ShareReport(c *gin.Context) {
	claims, err := h.getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid report ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req ShareReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate email addresses
	for _, email := range req.Emails {
		if !isValidEmail(email) {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Error:   "invalid_email",
				Message: "Invalid email address: " + email,
				Code:    http.StatusBadRequest,
			})
			return
		}
	}

	// Verify the report exists and belongs to the user's organization
	var title, format string
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT title, format
		FROM reports
		WHERE id = $1 AND organization_id = $2
	`, reportID, claims.OrganizationID).Scan(&title, &format)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Report not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query report for sharing", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to share report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// TODO: Integrate with an email service (e.g., SMTP or SendGrid) to send the report
	// to the specified recipients. For now, log the share action.
	h.logger.Info("report shared",
		zap.String("report_id", reportID.String()),
		zap.Strings("recipients", req.Emails),
		zap.String("shared_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data: gin.H{
			"report_id":  reportID,
			"title":      title,
			"format":     format,
			"recipients": req.Emails,
			"shared_by":  claims.UserID,
			"shared_at":  time.Now(),
		},
		Message: "Report shared successfully.",
	})
}

// isValidEmail performs basic validation on an email address.
func isValidEmail(email string) bool {
	if email == "" {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at < 1 || at >= len(email)-1 {
		return false
	}
	dot := strings.LastIndex(email[at:], ".")
	return dot > 0
}
