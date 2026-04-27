package experiment

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Handler holds dependencies for experiment HTTP handlers.
type Handler struct {
	db     *sql.DB
	rdb    *redis.Client
	cfg    *config.Config
	logger *zap.Logger
}

// NewHandler creates a new experiment handler with the provided dependencies.
func NewHandler(db *sql.DB, rdb *redis.Client, cfg *config.Config, logger *zap.Logger) *Handler {
	return &Handler{
		db:     db,
		rdb:    rdb,
		cfg:    cfg,
		logger: logger.Named("experiment_handler"),
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
		       u.name as creator_name
		FROM experiments e
		LEFT JOIN users u ON u.id = e.created_by
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
		CreatorName string `json:"creator_name,omitempty"`
	}

	experiments := make([]experimentListItem, 0)
	for rows.Next() {
		var exp models.Experiment
		var creatorName sql.NullString
		var scheduleCron sql.NullString
		var description sql.NullString

		err := rows.Scan(
			&exp.ID, &exp.OrganizationID, &exp.Name, &description,
			&exp.Status, &exp.CreatedBy, &scheduleCron,
			&exp.AutoCleanup, &exp.NotificationConfig,
			&exp.CreatedAt, &exp.UpdatedAt, &creatorName,
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
			Experiment:  exp,
			CreatorName: creatorName.String,
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

	c.JSON(http.StatusOK, exp)
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
		VALUES ($1, $2, $3, $4, 'draft', $5, $6, $7)
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
			&et.TargetNamespaces, &et.TargetLabels,
			&et.DurationSeconds, &et.CleanupPolicy,
			&et.SIEMValidation, &et.Enabled, &et.CreatedAt,
		)
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

	c.JSON(http.StatusCreated, exp)
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

	clusterID, err := uuid.Parse(req.ClusterID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_cluster_id",
			Message: "Invalid cluster ID format.",
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

	if expStatus != "draft" && expStatus != "active" {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "conflict",
			Message: fmt.Sprintf("Experiment is in '%s' status and cannot be executed. Only draft or active experiments can be run.", expStatus),
			Code:    http.StatusConflict,
		})
		return
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

	// Check if there is already a running experiment on this cluster
	var runningCount int
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE er.cluster_id = $1 AND e.organization_id = $2 AND er.status = 'running'
	`, clusterID, claims.OrganizationID).Scan(&runningCount)
	if err != nil {
		h.logger.Error("failed to check running experiments", zap.Error(err))
		// Non-fatal: continue with execution
	}

	if runningCount >= h.cfg.Kubernetes.MaxConcurrent {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "max_concurrent_reached",
			Message: fmt.Sprintf("Maximum concurrent experiments (%d) reached on this cluster. Please wait for running experiments to complete.", h.cfg.Kubernetes.MaxConcurrent),
			Code:    http.StatusConflict,
		})
		return
	}

	// Calculate the next run number
	var maxRunNumber sql.NullInt64
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT MAX(run_number) FROM experiment_runs WHERE experiment_id = $1",
		experimentID,
	).Scan(&maxRunNumber)

	nextRunNumber := 1
	if maxRunNumber.Valid {
		nextRunNumber = int(maxRunNumber.Int64) + 1
	}

	// Transition experiment to active status if it's still draft
	if expStatus == "draft" {
		_, err = h.db.ExecContext(c.Request.Context(),
			"UPDATE experiments SET status = 'active', updated_at = NOW() WHERE id = $1",
			experimentID,
		)
		if err != nil {
			h.logger.Error("failed to transition experiment to active", zap.Error(err))
			// Non-fatal: the experiment can still run
		}
	}

	// Create the experiment run record
	now := time.Now()
	var run models.ExperimentRun
	var errorMessage sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO experiment_runs (experiment_id, cluster_id, run_number, status, triggered_by, trigger_type, started_at)
		VALUES ($1, $2, $3, 'running', $4, 'manual', $5)
		RETURNING id, experiment_id, cluster_id, run_number, status, triggered_by, trigger_type,
		          started_at, completed_at, duration_ms, result_summary, error_message,
		          cleanup_status, created_at
	`,
		experimentID, clusterID, nextRunNumber, claims.UserID, now,
	).Scan(
		&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
		&run.Status, &run.TriggeredBy, &run.TriggerType,
		&run.StartedAt, &run.CompletedAt, &run.DurationMs,
		&run.ResultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
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

	if errorMessage.Valid {
		run.ErrorMessage = &errorMessage.String
	}

	// Enqueue the experiment run for async processing by the worker
	runKey := fmt.Sprintf("experiment:queue:%s", run.ID.String())
	runData, err := json.Marshal(map[string]interface{}{
		"run_id":          run.ID.String(),
		"experiment_id":   experimentID.String(),
		"cluster_id":      clusterID.String(),
		"organization_id": claims.OrganizationID.String(),
		"triggered_by":    claims.UserID.String(),
		"started_at":      now,
	})
	if err != nil {
		h.logger.Error("failed to marshal run data for queue", zap.Error(err))
		// The run is still created, the worker can pick it up from the DB
	} else {
		// Push to Redis queue with 24h TTL as a safety net
		queueKey := "experiment:run_queue"
		if err := h.rdb.RPush(c.Request.Context(), queueKey, runKey).Err(); err != nil {
			h.logger.Error("failed to enqueue experiment run", zap.Error(err))
		}
		if err := h.rdb.Set(c.Request.Context(), runKey, runData, 24*time.Hour).Err(); err != nil {
			h.logger.Error("failed to store run data in Redis", zap.Error(err))
		}
	}

	h.logger.Info("experiment execution started",
		zap.String("run_id", run.ID.String()),
		zap.String("experiment_id", experimentID.String()),
		zap.String("cluster_id", clusterID.String()),
		zap.Int("run_number", nextRunNumber),
		zap.String("triggered_by", claims.UserID.String()),
	)

	c.JSON(http.StatusAccepted, run)
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

	// Find the currently running experiment run
	var run models.ExperimentRun
	var errorMessage sql.NullString
	var resultSummary sql.NullString

	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by,
		       trigger_type, started_at, completed_at, duration_ms,
		       result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1 AND status = 'running'
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
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error:   "no_running_run",
				Message: "No running experiment found to stop.",
				Code:    http.StatusConflict,
			})
			return
		}
		h.logger.Error("failed to find running experiment run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to stop experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Verify the experiment belongs to the user's organization
	var orgID uuid.UUID
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT organization_id FROM experiments WHERE id = $1",
		experimentID,
	).Scan(&orgID)
	if err != nil || orgID != claims.OrganizationID {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Experiment not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	// Update the run status to cancelled
	now := time.Now()
	var durationMs int64
	if run.StartedAt != nil {
		durationMs = now.Sub(*run.StartedAt).Milliseconds()
	}

	errMsg := "Cancelled by user"
	_, err = h.db.ExecContext(c.Request.Context(), `
		UPDATE experiment_runs
		SET status = 'cancelled', completed_at = $1, duration_ms = $2, error_message = $3
		WHERE id = $4
	`, now, durationMs, errMsg, run.ID)
	if err != nil {
		h.logger.Error("failed to cancel experiment run", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to stop experiment.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Signal the worker to stop processing this run
	stopKey := fmt.Sprintf("experiment:stop:%s", run.ID.String())
	if err := h.rdb.Set(c.Request.Context(), stopKey, "1", 10*time.Minute).Err(); err != nil {
		h.logger.Error("failed to set stop signal in Redis", zap.Error(err))
		// Non-fatal: the run is already marked as cancelled in the DB
	}

	h.logger.Info("experiment execution stopped",
		zap.String("run_id", run.ID.String()),
		zap.String("experiment_id", experimentID.String()),
		zap.String("stopped_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"message":      "Experiment execution stopped.",
		"run_id":       run.ID,
		"run_number":   run.RunNumber,
		"cancelled_at": now,
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
		err := rows.Scan(
			&et.ID, &et.ExperimentID, &et.AttackTemplateID,
			&et.OrderIndex, &et.Configuration,
			&et.TargetNamespaces, &et.TargetLabels,
			&et.DurationSeconds, &et.CleanupPolicy,
			&et.SIEMValidation, &et.Enabled, &et.CreatedAt,
		)
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

// fetchRecentRuns retrieves the most recent experiment runs.
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
			// resultSummary is already json.RawMessage from Scan
		}

		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment run rows: %w", err)
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
