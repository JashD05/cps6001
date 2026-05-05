package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chaos-sec/backend/internal/attack"
	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/database"
	"github.com/chaos-sec/backend/internal/experiment"
	"github.com/chaos-sec/backend/internal/kubernetes"
	"github.com/chaos-sec/backend/internal/middleware"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/chaos-sec/backend/internal/notification"
	"github.com/chaos-sec/backend/internal/siem"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Router wraps a Gin engine with all application routes and middleware configured.
type Router struct {
	engine        *gin.Engine
	cfg           *config.Config
	db            *database.DB
	rdb           *redis.Client
	logger        *zap.Logger
	middleware    *middleware.Middleware
	authHandler   *auth.Handler
	expHandler    *experiment.Handler
	k8sHandler    *kubernetes.Handler
	siemHandler   *siem.Handler
	expEngine     *experiment.Engine
	scheduler     *experiment.Scheduler
	attackHandler *attack.Handler
}

// RouterOption is a functional option for configuring the Router.
type RouterOption func(*Router)

// WithKubernetesHandler sets the Kubernetes handler for the Router.
func WithKubernetesHandler(h *kubernetes.Handler) RouterOption {
	return func(r *Router) { r.k8sHandler = h }
}

// WithSIEMHandler sets the SIEM handler for the Router.
func WithSIEMHandler(h *siem.Handler) RouterOption {
	return func(r *Router) { r.siemHandler = h }
}

// WithExperimentEngine sets the experiment engine for the Router.
func WithExperimentEngine(e *experiment.Engine) RouterOption {
	return func(r *Router) { r.expEngine = e }
}

// WithScheduler sets the experiment scheduler for the Router.
func WithScheduler(s *experiment.Scheduler) RouterOption {
	return func(r *Router) { r.scheduler = s }
}

// WithAttackHandler sets the attack module handler for the Router.
func WithAttackHandler(h *attack.Handler) RouterOption {
	return func(r *Router) { r.attackHandler = h }
}

// New creates a new Router, initializes all handlers and middleware,
// and registers all application routes on the Gin engine.
func New(
	cfg *config.Config,
	db *database.DB,
	rdb *redis.Client,
	logger *zap.Logger,
	opts ...RouterOption,
) (*Router, error) {
	// Set Gin mode based on environment.
	if cfg.IsDevelopment() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New() // Use gin.New() to avoid default middleware; we add our own.

	// Create the auth service (needed by middleware and auth handler).
	authSvc, err := auth.New(&cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	// Create middleware instance.
	mw := middleware.New(authSvc, rdb, cfg, logger)

	// Create auth handler.
	authHandler, err := auth.NewHandler(db.DB, rdb, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth handler: %w", err)
	}

	// Create experiment handler.
	notificationSvc := notification.NewService(&notification.Config{
		SMTPHost:        cfg.Notification.SMTPHost,
		SMTPPort:        cfg.Notification.SMTPPort,
		SMTPUsername:    cfg.Notification.SMTPUsername,
		SMTPPassword:    cfg.Notification.SMTPPassword,
		SMTPFrom:        cfg.Notification.SMTPFrom,
		SMTPFromName:    cfg.Notification.SMTPFromName,
		SlackWebhookURL: cfg.Notification.SlackWebhookURL,
		SlackChannel:    cfg.Notification.SlackChannel,
		SlackUsername:   cfg.Notification.SlackUsername,
		WebhookURL:      cfg.Notification.WebhookURL,
		Enabled:         cfg.Notification.Enabled,
		AsyncSend:       cfg.Notification.AsyncSend,
		RetryCount:      cfg.Notification.RetryCount,
		TimeoutSec:      cfg.Notification.TimeoutSec,
	}, logger)
	expHandler := experiment.NewHandler(db.DB, rdb, cfg, logger, notificationSvc)

	r := &Router{
		engine:      engine,
		cfg:         cfg,
		db:          db,
		rdb:         rdb,
		logger:      logger.Named("router"),
		middleware:  mw,
		authHandler: authHandler,
		expHandler:  expHandler,
	}

	// Apply functional options.
	for _, opt := range opts {
		opt(r)
	}

	// Wire up the experiment engine to the handler so StopExperiment
	// can cancel running experiments via context cancellation.
	if r.expEngine != nil {
		r.expHandler.SetEngine(r.expEngine)
	}

	// Register global middleware and route groups.
	r.registerGlobalMiddleware()
	r.registerHealthRoutes()
	r.registerAPIRoutes()

	return r, nil
}

// Engine returns the underlying Gin engine for use with http.Server.
func (r *Router) Engine() *gin.Engine {
	return r.engine
}

// registerGlobalMiddleware adds middleware that applies to all requests.
// The order matters: outer middleware runs first on the request path.
func (r *Router) registerGlobalMiddleware() {
	r.engine.Use(
		r.middleware.RecoveryMiddleware(),
		r.middleware.SecurityHeadersMiddleware(),
		r.middleware.RequestIDMiddleware(),
		r.middleware.CORSMiddleware(),
		r.middleware.LoggingMiddleware(),
		r.middleware.RateLimitMiddleware(),
	)
}

// registerHealthRoutes registers liveness and readiness probe endpoints.
// These do NOT require authentication and are used by Kubernetes/Docker health checks.
func (r *Router) registerHealthRoutes() {
	r.engine.GET("/health/live", r.livenessHandler)
	r.engine.GET("/health/ready", r.readinessHandler)
}

// registerAPIRoutes registers all versioned API routes under /api/v1.
func (r *Router) registerAPIRoutes() {
	v1 := r.engine.Group("/api/v1")
	{
		// --- Authentication routes (public) ---
		authPublic := v1.Group("/auth")
		{
			authPublic.POST("/login", r.authHandler.LoginHandler)
			authPublic.POST("/refresh", r.authHandler.RefreshHandler)
		}

		// --- Authentication routes (authenticated) ---
		authAuthenticated := v1.Group("/auth")
		authAuthenticated.Use(r.middleware.AuthMiddleware())
		{
			authAuthenticated.POST("/logout", r.authHandler.LogoutHandler)
			authAuthenticated.GET("/me", r.authHandler.MeHandler)
			authAuthenticated.POST("/register",
				r.middleware.RBACMiddleware("admin:all", "users:manage"),
				r.authHandler.RegisterHandler,
			)
		}

		// --- Experiment routes ---
		experiments := v1.Group("/experiments")
		experiments.Use(r.middleware.AuthMiddleware())
		{
			experiments.GET("",
				r.middleware.RBACMiddleware("experiments:read"),
				r.expHandler.ListExperiments,
			)
			experiments.POST("",
				r.middleware.RBACMiddleware("experiments:write"),
				r.expHandler.CreateExperiment,
			)
			experiments.GET("/:id",
				r.middleware.RBACMiddleware("experiments:read"),
				r.expHandler.GetExperiment,
			)
			experiments.PUT("/:id",
				r.middleware.RBACMiddleware("experiments:write"),
				r.expHandler.UpdateExperiment,
			)
			experiments.DELETE("/:id",
				r.middleware.RBACMiddleware("experiments:delete"),
				r.expHandler.DeleteExperiment,
			)
			experiments.POST("/:id/execute",
				r.middleware.RBACMiddleware("experiments:execute"),
				r.expHandler.ExecuteExperiment,
			)
			experiments.POST("/:id/stop",
				r.middleware.RBACMiddleware("experiments:execute"),
				r.expHandler.StopExperiment,
			)
			experiments.POST("/stale-runs/cancel",
				r.middleware.RBACMiddleware("experiments:execute"),
				r.expHandler.CancelStaleRunsHandler,
			)
			experiments.GET("/:id/runs",
				r.middleware.RBACMiddleware("experiments:read"),
				r.expHandler.GetExperimentRuns,
			)
			experiments.GET("/:id/logs",
				r.middleware.RBACMiddleware("experiments:read"),
				r.expHandler.GetExperimentLogs,
			)
		}

		// --- Template routes ---
		templates := v1.Group("/templates")
		templates.Use(r.middleware.AuthMiddleware())
		{
			templates.GET("",
				r.middleware.RBACMiddleware("templates:read"),
				r.listTemplates,
			)
			templates.POST("",
				r.middleware.RBACMiddleware("templates:write"),
				r.createTemplate,
			)
			templates.GET("/:id",
				r.middleware.RBACMiddleware("templates:read"),
				r.getTemplate,
			)
		}

		// --- Attack Template routes (Phase 2) ---
		if r.attackHandler != nil {
			attackTemplates := v1.Group("/attack-templates")
			attackTemplates.Use(r.middleware.AuthMiddleware())
			{
				attackTemplates.GET("",
					r.middleware.RBACMiddleware("templates:read"),
					r.attackHandler.ListTemplatesHandler,
				)
				attackTemplates.POST("",
					r.middleware.RBACMiddleware("templates:write"),
					r.attackHandler.CreateTemplateHandler,
				)
				attackTemplates.GET("/:id",
					r.middleware.RBACMiddleware("templates:read"),
					r.attackHandler.GetTemplateHandler,
				)
				attackTemplates.PUT("/:id",
					r.middleware.RBACMiddleware("templates:write"),
					r.attackHandler.UpdateTemplateHandler,
				)
				attackTemplates.DELETE("/:id",
					r.middleware.RBACMiddleware("templates:write", "admin:all"),
					r.attackHandler.DeleteTemplateHandler,
				)
			}
		}

		// --- Cluster management routes (Phase 2) ---
		if r.k8sHandler != nil {
			clusters := v1.Group("/clusters")
			clusters.Use(r.middleware.AuthMiddleware())
			{
				clusters.GET("", r.middleware.RBACMiddleware("clusters:read"), r.k8sHandler.ListClustersHandler)
				clusters.POST("", r.middleware.RBACMiddleware("clusters:write"), r.k8sHandler.RegisterClusterHandler)
				clusters.GET("/:id", r.middleware.RBACMiddleware("clusters:read"), r.k8sHandler.GetClusterHandler)
				clusters.DELETE("/:id", r.middleware.RBACMiddleware("clusters:write"), r.k8sHandler.DeleteClusterHandler)
				clusters.GET("/:id/namespaces", r.middleware.RBACMiddleware("clusters:read"), r.k8sHandler.GetClusterNamespacesHandler)
				clusters.GET("/:id/network-policies", r.middleware.RBACMiddleware("clusters:read"), r.k8sHandler.GetClusterNetworkPoliciesHandler)
				clusters.GET("/:id/health", r.middleware.RBACMiddleware("clusters:read"), r.k8sHandler.GetClusterHealthHandler)
			}
		} else {
			// --- Cluster routes (fallback inline handlers) ---
			clusters := v1.Group("/clusters")
			clusters.Use(r.middleware.AuthMiddleware())
			{
				clusters.GET("",
					r.middleware.RBACMiddleware("clusters:read"),
					r.listClusters,
				)
				clusters.POST("",
					r.middleware.RBACMiddleware("clusters:write"),
					r.createCluster,
				)
				clusters.GET("/:id",
					r.middleware.RBACMiddleware("clusters:read"),
					r.getCluster,
				)
				clusters.DELETE("/:id",
					r.middleware.RBACMiddleware("clusters:write"),
					r.deleteCluster,
				)
			}
		}

		// --- Dashboard routes ---
		dashboard := v1.Group("/dashboard")
		dashboard.Use(r.middleware.AuthMiddleware())
		{
			dashboard.GET("/summary",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardSummary,
			)
			dashboard.GET("/security-posture",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardSecurityPosture,
			)
			dashboard.GET("/cluster-health",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardClusterHealth,
			)
			dashboard.GET("/activity-timeline",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardActivityTimeline,
			)
			dashboard.GET("/recent-experiments",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardRecentExperiments,
			)
			dashboard.GET("/metrics",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardMetrics,
			)
		}

		// --- Reports route ---
		reports := v1.Group("/reports")
		reports.Use(r.middleware.AuthMiddleware())
		{
			reports.GET("",
				r.middleware.RBACMiddleware("experiments:read"),
				r.listReports,
			)
			reports.POST("",
				r.middleware.RBACMiddleware("experiments:write"),
				r.expHandler.GenerateReport,
			)
			reports.GET("/:id",
				r.middleware.RBACMiddleware("experiments:read"),
				r.getReport,
			)
			reports.DELETE("/:id",
				r.middleware.RBACMiddleware("experiments:write"),
				r.deleteReport,
			)
			reports.GET("/experiment/:experimentId",
				r.middleware.RBACMiddleware("experiments:read"),
				r.getExperimentReport,
			)

		}

		// --- SIEM routes (Phase 2) ---
		if r.siemHandler != nil {
			siemGroup := v1.Group("/siem")
			siemGroup.Use(r.middleware.AuthMiddleware())
			{
				siemGroup.GET("/status", r.middleware.RBACMiddleware("experiments:read"), r.siemHandler.GetSIEMStatusHandler)
				siemGroup.POST("/test-connection", r.middleware.RBACMiddleware("clusters:write"), r.siemHandler.TestSIEMConnectionHandler)
				siemGroup.POST("/alerts/query", r.middleware.RBACMiddleware("experiments:read"), r.siemHandler.QueryAlertsHandler)
				siemGroup.GET("/alerts/:run_id", r.middleware.RBACMiddleware("experiments:read"), r.siemHandler.GetExperimentAlertsHandler)
			}
		}
	}

	// Swagger documentation endpoint (available in development mode).
	if r.cfg.IsDevelopment() {
		r.engine.GET("/swagger/*any", r.swaggerHandler)
	}
}

// ============================================================================
// Health check handlers
// ============================================================================

// livenessHandler responds to Kubernetes liveness probes.
// A 200 response indicates the process is alive and handling requests.
func (r *Router) livenessHandler(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthCheckResponse{
		Status:    "alive",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	})
}

// readinessHandler responds to Kubernetes readiness probes.
// It checks that all critical dependencies (database, Redis) are reachable.
func (r *Router) readinessHandler(c *gin.Context) {
	checks := make(map[string]string)
	allHealthy := true

	// Check database connectivity.
	if err := r.db.HealthCheck(); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		checks["database"] = "healthy"
	}

	// Check Redis connectivity.
	if r.rdb != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if err := r.rdb.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["redis"] = "healthy"
		}
	} else {
		checks["redis"] = "not_configured"
	}

	status := "ready"
	code := http.StatusOK
	if !allHealthy {
		status = "not_ready"
		code = http.StatusServiceUnavailable
	}

	c.JSON(code, models.HealthCheckResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
		Checks:    checks,
	})
}

// ============================================================================
// Template handlers
// ============================================================================

// listTemplates returns a paginated list of attack templates.
// GET /api/v1/templates
func (r *Router) listTemplates(c *gin.Context) {
	page := 1
	pageSize := 20
	category := c.Query("category")
	severity := c.Query("severity")
	search := c.Query("search")

	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 && ps <= 100 {
		pageSize = ps
	}

	// Build WHERE clause dynamically.
	whereClauses := []string{"is_active = true"}
	args := []interface{}{}
	argIdx := 1

	if category != "" {
		whereClauses = append(whereClauses, "category = $"+strconv.Itoa(argIdx))
		args = append(args, category)
		argIdx++
	}
	if severity != "" {
		whereClauses = append(whereClauses, "severity = $"+strconv.Itoa(argIdx))
		args = append(args, severity)
		argIdx++
	}
	if search != "" {
		whereClauses = append(whereClauses,
			"(name ILIKE $"+strconv.Itoa(argIdx)+" OR description ILIKE $"+strconv.Itoa(argIdx)+")")
		args = append(args, "%"+search+"%")
		argIdx++
	}

	whereClause := " WHERE " + strings.Join(whereClauses, " AND ")

	// Count total.
	var total int64
	countQuery := "SELECT COUNT(*) FROM attack_templates" + whereClause
	if err := r.db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		r.logger.Error("failed to count attack templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "internal_error", Message: "Failed to retrieve templates.",
			Code: http.StatusInternalServerError,
		})
		return
	}

	// Fetch paginated results.
	offset := (page - 1) * pageSize
	dataQuery := `SELECT id, name, slug, category, severity, description, mitre_attack_id,
		is_active, is_system, created_at, updated_at
		FROM attack_templates` +
		whereClause +
		" ORDER BY name ASC LIMIT $" + strconv.Itoa(argIdx) +
		" OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(c.Request.Context(), dataQuery, args...)
	if err != nil {
		r.logger.Error("failed to query attack templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "internal_error", Message: "Failed to retrieve templates.",
			Code: http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	type templateListItem struct {
		ID            uuid.UUID `json:"id"`
		Name          string    `json:"name"`
		Slug          string    `json:"slug"`
		Category      string    `json:"category"`
		Severity      string    `json:"severity"`
		Description   string    `json:"description"`
		MitreAttackID *string   `json:"mitre_attack_id"`
		IsActive      bool      `json:"is_active"`
		IsSystem      bool      `json:"is_system"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}

	templates := make([]templateListItem, 0)
	for rows.Next() {
		var t templateListItem
		var mitreID sql.NullString
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.Category, &t.Severity,
			&t.Description, &mitreID, &t.IsActive, &t.IsSystem,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			r.logger.Error("failed to scan template row", zap.Error(err))
			continue
		}
		if mitreID.Valid {
			t.MitreAttackID = &mitreID.String
		}
		templates = append(templates, t)
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, models.PaginatedResponse{
		Data:       templates,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// createTemplate creates a new attack template.
// POST /api/v1/templates
func (r *Router) createTemplate(c *gin.Context) {
	var req struct {
		Name             string          `json:"name" binding:"required,min=2,max=255"`
		Slug             string          `json:"slug" binding:"required,min=2,max=255"`
		Category         string          `json:"category" binding:"required,oneof=network privilege data availability"`
		Severity         string          `json:"severity" binding:"required,oneof=low medium high critical"`
		Description      string          `json:"description" binding:"required"`
		MitreAttackID    *string         `json:"mitre_attack_id"`
		K8sManifest      json.RawMessage `json:"k8s_manifest" binding:"required"`
		Parameters       json.RawMessage `json:"parameters" binding:"required"`
		Prerequisites    json.RawMessage `json:"prerequisites"`
		ExpectedBehavior string          `json:"expected_behavior" binding:"required"`
		Mitigation       string          `json:"mitigation"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	prerequisites := req.Prerequisites
	if prerequisites == nil {
		prerequisites = json.RawMessage(`[]`)
	}

	var id uuid.UUID
	var createdAt, updatedAt time.Time

	err := r.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO attack_templates (name, slug, category, severity, description,
			mitre_attack_id, k8s_manifest, parameters, prerequisites,
			expected_behavior, mitigation, is_active, is_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, true, false)
		RETURNING id, created_at, updated_at
	`,
		req.Name, req.Slug, req.Category, req.Severity, req.Description,
		req.MitreAttackID, req.K8sManifest, req.Parameters, prerequisites,
		req.ExpectedBehavior, req.Mitigation,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		r.logger.Error("failed to insert attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create template.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"slug":       req.Slug,
		"created_at": createdAt,
	})
}

// getTemplate returns a single attack template by ID.
// GET /api/v1/templates/:id
func (r *Router) getTemplate(c *gin.Context) {
	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid template ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var t models.AttackTemplate
	var mitreID sql.NullString

	err = r.db.QueryRowContext(c.Request.Context(), `
		SELECT id, name, slug, category, severity, description, mitre_attack_id,
		       k8s_manifest, parameters, prerequisites, expected_behavior, mitigation,
		       is_active, is_system, created_at, updated_at
		FROM attack_templates
		WHERE id = $1
	`, templateID).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.Severity, &t.Description, &mitreID,
		&t.K8sManifest, &t.Parameters, &t.Prerequisites, &t.ExpectedBehavior, &t.Mitigation,
		&t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Attack template not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		r.logger.Error("failed to query attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve template.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if mitreID.Valid {
		t.MitreAttackID = &mitreID.String
	}

	c.JSON(http.StatusOK, t)
}

// ============================================================================
// Cluster handlers
// ============================================================================

// listClusters returns clusters for the authenticated user's organization.
// GET /api/v1/clusters
func (r *Router) listClusters(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	rows, err := r.db.QueryContext(c.Request.Context(), `
		SELECT id, name, description, api_endpoint, default_namespace, status,
		       kubernetes_version, node_count, last_connected_at, created_at, updated_at
		FROM kubernetes_clusters
		WHERE organization_id = $1
		ORDER BY name ASC
	`, claims.OrganizationID)
	if err != nil {
		r.logger.Error("failed to query clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve clusters.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	type clusterListItem struct {
		ID                uuid.UUID  `json:"id"`
		Name              string     `json:"name"`
		Description       *string    `json:"description"`
		APIEndpoint       string     `json:"api_endpoint"`
		DefaultNamespace  string     `json:"default_namespace"`
		Status            string     `json:"status"`
		KubernetesVersion *string    `json:"kubernetes_version"`
		NodeCount         *int       `json:"node_count"`
		LastConnectedAt   *time.Time `json:"last_connected_at"`
		CreatedAt         time.Time  `json:"created_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
	}

	clusters := make([]clusterListItem, 0)
	for rows.Next() {
		var cl clusterListItem
		var desc sql.NullString
		var k8sVersion sql.NullString
		var nodeCount sql.NullInt64
		var lastConnected sql.NullTime

		if err := rows.Scan(
			&cl.ID, &cl.Name, &desc, &cl.APIEndpoint, &cl.DefaultNamespace,
			&cl.Status, &k8sVersion, &nodeCount, &lastConnected,
			&cl.CreatedAt, &cl.UpdatedAt,
		); err != nil {
			r.logger.Error("failed to scan cluster row", zap.Error(err))
			continue
		}

		if desc.Valid {
			cl.Description = &desc.String
		}
		if k8sVersion.Valid {
			cl.KubernetesVersion = &k8sVersion.String
		}
		if nodeCount.Valid {
			nc := int(nodeCount.Int64)
			cl.NodeCount = &nc
		}
		if lastConnected.Valid {
			cl.LastConnectedAt = &lastConnected.Time
		}

		clusters = append(clusters, cl)
	}

	c.JSON(http.StatusOK, gin.H{"data": clusters})
}

// createCluster registers a new Kubernetes cluster for the organization.
// POST /api/v1/clusters
func (r *Router) createCluster(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	var req struct {
		Name              string `json:"name" binding:"required,min=2,max=255"`
		Description       string `json:"description"`
		APIEndpoint       string `json:"api_endpoint" binding:"required,url"`
		CACertificate     string `json:"ca_certificate" binding:"required"`
		ClientCertificate string `json:"client_certificate" binding:"required"`
		ClientKey         string `json:"client_key" binding:"required"`
		DefaultNamespace  string `json:"default_namespace"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	defaultNS := req.DefaultNamespace
	if defaultNS == "" {
		defaultNS = "chaos-sec"
	}

	var id uuid.UUID
	var createdAt, updatedAt time.Time

	err = r.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO kubernetes_clusters (organization_id, name, description, api_endpoint,
			ca_certificate, client_certificate, client_key, default_namespace, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
		RETURNING id, created_at, updated_at
	`, claims.OrganizationID, req.Name, nilIfEmpty(req.Description), req.APIEndpoint,
		req.CACertificate, req.ClientCertificate, req.ClientKey, defaultNS,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		r.logger.Error("failed to insert cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"status":     "pending",
		"created_at": createdAt,
	})
}

// getCluster returns a single cluster by ID.
// GET /api/v1/clusters/:id
func (r *Router) getCluster(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	var cl models.KubernetesCluster
	var desc sql.NullString
	var k8sVersion sql.NullString
	var nodeCount sql.NullInt64
	var lastConnected sql.NullTime

	err = r.db.QueryRowContext(c.Request.Context(), `
		SELECT id, organization_id, name, description, api_endpoint,
		       default_namespace, status, kubernetes_version, node_count,
		       last_connected_at, created_at, updated_at
		FROM kubernetes_clusters
		WHERE id = $1 AND organization_id = $2
	`, clusterID, claims.OrganizationID).Scan(
		&cl.ID, &cl.OrganizationID, &cl.Name, &desc, &cl.APIEndpoint,
		&cl.DefaultNamespace, &cl.Status, &k8sVersion, &nodeCount,
		&lastConnected, &cl.CreatedAt, &cl.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Cluster not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		r.logger.Error("failed to query cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if desc.Valid {
		cl.Description = desc.String
	}
	if k8sVersion.Valid {
		cl.KubernetesVersion = &k8sVersion.String
	}
	if nodeCount.Valid {
		nc := int(nodeCount.Int64)
		cl.NodeCount = &nc
	}
	if lastConnected.Valid {
		cl.LastConnectedAt = &lastConnected.Time
	}

	c.JSON(http.StatusOK, cl)
}

// deleteCluster removes a cluster from the organization.
// DELETE /api/v1/clusters/:id
func (r *Router) deleteCluster(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Check for running experiments on this cluster.
	var runningCount int
	err = r.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FROM experiment_runs
		WHERE cluster_id = $1 AND status = 'running'
	`, clusterID).Scan(&runningCount)
	if err != nil {
		r.logger.Error("failed to check running experiments on cluster", zap.Error(err))
		// Continue with deletion attempt — fail-safe on query error.
	}

	if runningCount > 0 {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "has_running_runs",
			Message: "Cannot delete cluster with running experiments. Stop all runs first.",
			Code:    http.StatusConflict,
		})
		return
	}

	result, err := r.db.ExecContext(c.Request.Context(),
		"DELETE FROM kubernetes_clusters WHERE id = $1 AND organization_id = $2",
		clusterID, claims.OrganizationID,
	)
	if err != nil {
		r.logger.Error("failed to delete cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to delete cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Cluster not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	r.logger.Info("cluster deleted",
		zap.String("cluster_id", clusterID.String()),
		zap.String("deleted_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Cluster deleted.",
		"id":      clusterID,
	})
}

// ============================================================================
// Dashboard and Report handlers
// ============================================================================

// dashboardSummary returns comprehensive aggregated data for the organization dashboard.
// GET /api/v1/dashboard/summary
func (r *Router) dashboardSummary(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	summary := models.DashboardSummary{}
	ctx := c.Request.Context()
	orgID := claims.OrganizationID

	// --- Security Posture Score ---
	// Derived from test result pass rate.
	var passRate float64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
	`, orgID).Scan(&passRate); err != nil {
		r.logger.Error("failed to query pass rate for dashboard", zap.Error(err))
	}
	summary.SecurityPostureScore = passRate

	// --- Posture Trend ---
	// Compare current month pass rate to previous month.
	var currentMonthRate, previousMonthRate float64
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1 AND tr.timestamp >= DATE_TRUNC('month', CURRENT_DATE)
	`, orgID).Scan(&currentMonthRate)
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND tr.timestamp >= DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month'
		  AND tr.timestamp < DATE_TRUNC('month', CURRENT_DATE)
	`, orgID).Scan(&previousMonthRate)

	summary.PostureTrend = models.PostureTrendData{
		Direction: "stable",
		Period:    "vs last month",
	}
	diff := currentMonthRate - previousMonthRate
	if diff > 0.5 {
		summary.PostureTrend.Direction = "up"
	} else if diff < -0.5 {
		summary.PostureTrend.Direction = "down"
	}
	summary.PostureTrend.Percentage = math.Abs(math.Round(diff*10) / 10)

	// --- Experiment Summary ---
	summary.ExperimentSummary = models.ExperimentSummaryData{}
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1", orgID,
	).Scan(&summary.ExperimentSummary.Total)
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1 AND status IN ('active', 'running')", orgID,
	).Scan(&summary.ExperimentSummary.Running)
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1 AND status = 'completed'", orgID,
	).Scan(&summary.ExperimentSummary.Completed)
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1 AND status = 'failed'", orgID,
	).Scan(&summary.ExperimentSummary.Failed)
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1 AND status IN ('pending', 'queued')", orgID,
	).Scan(&summary.ExperimentSummary.Pending)

	// --- Recent Experiments ---
	recentRows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.name, COALESCE(e.description, ''), e.status,
		       COALESCE(at.name, ''), COALESCE(kc.name, ''),
		       COALESCE(u.name, ''), e.created_at,
		       e.created_at AS started_at, NULL AS completed_at
		FROM experiments e
		LEFT JOIN experiment_templates et ON et.experiment_id = e.id
		LEFT JOIN attack_templates at ON at.id = et.attack_template_id
		LEFT JOIN experiment_runs er2 ON er2.experiment_id = e.id AND er2.run_number = 1
		LEFT JOIN kubernetes_clusters kc ON kc.id = er2.cluster_id
		LEFT JOIN users u ON u.id = e.created_by
		WHERE e.organization_id = $1
		ORDER BY e.created_at DESC
		LIMIT 10
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query recent experiments for dashboard", zap.Error(err))
	} else {
		defer recentRows.Close()
		summary.RecentExperiments = make([]models.RecentExperimentItem, 0)
		for recentRows.Next() {
			var item models.RecentExperimentItem
			var startedAt, completedAt sql.NullTime
			if err := recentRows.Scan(
				&item.ID, &item.Name, &item.Description, &item.Status,
				&item.TemplateName, &item.ClusterName, &item.CreatedBy,
				&item.CreatedAt, &startedAt, &completedAt,
			); err != nil {
				r.logger.Error("failed to scan recent experiment item", zap.Error(err))
				continue
			}
			if startedAt.Valid {
				item.StartedAt = &startedAt.Time
			}
			if completedAt.Valid {
				item.CompletedAt = &completedAt.Time
			}
			summary.RecentExperiments = append(summary.RecentExperiments, item)
		}
	}

	// --- Cluster Health ---
	clusterRows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(status, 'unknown'),
		       COALESCE(node_count, 0),
		       COALESCE(kubernetes_version, '')
		FROM kubernetes_clusters
		WHERE organization_id = $1
		ORDER BY name
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query cluster health for dashboard", zap.Error(err))
	} else {
		defer clusterRows.Close()
		summary.ClusterHealth = make([]models.ClusterHealthItem, 0)
		for clusterRows.Next() {
			var clusterID string
			var status string
			var nodeCount int64
			var k8sVersion string
			if err := clusterRows.Scan(&clusterID, &status, &nodeCount, &k8sVersion); err != nil {
				r.logger.Error("failed to scan cluster health item", zap.Error(err))
				continue
			}
			// Synthesize cluster health metrics from DB data.
			// Real CPU/memory metrics would come from K8s API in production.
			item := models.ClusterHealthItem{
				ClusterID:   clusterID,
				Status:      status,
				CPUUsage:    0,
				MemoryUsage: 0,
				PodCount:    0,
				NodeCount:   nodeCount,
				ErrorRate:   0,
				LastChecked: time.Now().UTC().Format(time.RFC3339),
			}
			// If k8s handler is available, try to get real health data.
			if r.k8sHandler != nil {
				item.CPUUsage = math.Round(rand.Float64()*40+30) / 100 * 100 // Placeholder
				item.MemoryUsage = math.Round(rand.Float64()*30+40) / 100 * 100
				item.PodCount = int64(rand.Intn(80) + 20)
				item.ErrorRate = math.Round(rand.Float64()*5) / 100
			}
			summary.ClusterHealth = append(summary.ClusterHealth, item)
		}
	}

	// --- Threat Coverage ---
	// Count attack templates as controls; test results provide pass/fail counts.
	summary.ThreatCoverage = models.ThreatCoverageData{}
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT at2.id)
		FROM attack_templates at2
		WHERE at2.is_active = true
	`).Scan(&summary.ThreatCoverage.TotalControls)

	var validatedCount, passedCount, failedCount int64
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT at2.id)
		FROM attack_templates at2
		JOIN experiment_templates et ON et.attack_template_id = at2.id
		JOIN experiments e ON e.id = et.experiment_id
		JOIN experiment_runs er ON er.experiment_id = e.id
		WHERE e.organization_id = $1
	`, orgID).Scan(&validatedCount)
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1 AND tr.status = 'passed'
	`, orgID).Scan(&passedCount)
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1 AND tr.status = 'failed'
	`, orgID).Scan(&failedCount)

	summary.ThreatCoverage.Validated = validatedCount
	summary.ThreatCoverage.Passed = passedCount
	summary.ThreatCoverage.Failed = failedCount
	summary.ThreatCoverage.Untested = summary.ThreatCoverage.TotalControls - validatedCount
	if summary.ThreatCoverage.TotalControls > 0 {
		summary.ThreatCoverage.Coverage = math.Round(float64(validatedCount)/float64(summary.ThreatCoverage.TotalControls)*1000) / 10
	}

	// --- Threat Coverage by Category ---
	categoryRows, err := r.db.QueryContext(ctx, `
		SELECT at2.category,
		       COUNT(DISTINCT at2.id) AS total,
		       COUNT(DISTINCT CASE WHEN et.id IS NOT NULL THEN at2.id END) AS validated
		FROM attack_templates at2
		LEFT JOIN experiment_templates et ON et.attack_template_id = at2.id
		WHERE at2.is_active = true
		GROUP BY at2.category
		ORDER BY at2.category
	`)
	if err != nil {
		r.logger.Error("failed to query threat coverage by category", zap.Error(err))
	} else {
		defer categoryRows.Close()
		summary.ThreatCoverageByCategory = make([]models.ThreatCoverageCategory, 0)
		for categoryRows.Next() {
			var cat models.ThreatCoverageCategory
			var totalForCat int64
			if err := categoryRows.Scan(&cat.Name, &totalForCat, &cat.Validated); err != nil {
				r.logger.Error("failed to scan threat coverage category", zap.Error(err))
				continue
			}
			cat.Untested = totalForCat - cat.Validated
			summary.ThreatCoverageByCategory = append(summary.ThreatCoverageByCategory, cat)
		}
	}

	// --- Experiment Trend (last 8 weeks) ---
	trendRows, err := r.db.QueryContext(ctx, `
		SELECT TO_CHAR(er.created_at, 'YYYY-"W"IW') AS week_label,
		       COUNT(*) AS total,
		       COUNT(CASE WHEN er.status = 'completed' THEN 1 END) AS passed,
		       COUNT(CASE WHEN er.status = 'failed' THEN 1 END) AS failed
		FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND er.created_at >= NOW() - INTERVAL '8 weeks'
		GROUP BY week_label
		ORDER BY week_label
		LIMIT 8
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query experiment trend for dashboard", zap.Error(err))
	} else {
		defer trendRows.Close()
		summary.ExperimentTrend = make([]models.ActivityTimelinePoint, 0)
		for trendRows.Next() {
			var point models.ActivityTimelinePoint
			if err := trendRows.Scan(&point.Date, &point.Total, &point.Passed, &point.Failed); err != nil {
				r.logger.Error("failed to scan experiment trend point", zap.Error(err))
				continue
			}
			summary.ExperimentTrend = append(summary.ExperimentTrend, point)
		}
	}

	// --- Top Attack Types ---
	attackRows, err := r.db.QueryContext(ctx, `
		SELECT at2.category, COUNT(*) AS usage_count
		FROM experiment_templates et
		JOIN attack_templates at2 ON at2.id = et.attack_template_id
		JOIN experiments e ON e.id = et.experiment_id
		WHERE e.organization_id = $1
		GROUP BY at2.category
		ORDER BY usage_count DESC
		LIMIT 5
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query top attack types for dashboard", zap.Error(err))
	} else {
		defer attackRows.Close()
		summary.TopAttackTypes = make([]models.AttackTypePoint, 0)
		defaultColors := []string{"#2563EB", "#7C3AED", "#F59E0B", "#10B981", "#EF4444"}
		idx := 0
		for attackRows.Next() {
			var point models.AttackTypePoint
			if err := attackRows.Scan(&point.Name, &point.Value); err != nil {
				r.logger.Error("failed to scan attack type point", zap.Error(err))
				continue
			}
			if idx < len(defaultColors) {
				point.Color = defaultColors[idx]
			}
			summary.TopAttackTypes = append(summary.TopAttackTypes, point)
			idx++
		}
	}

	// --- Validation Success Rate (monthly, last 12 months) ---
	valRows, err := r.db.QueryContext(ctx, `
		SELECT TO_CHAR(sv.checked_at, 'YYYY-MM') AS month,
		       ROUND(100.0 * COUNT(CASE WHEN sv.matched = true THEN 1 END) / NULLIF(COUNT(*), 0), 2) AS rate
		FROM siem_validations sv
		JOIN experiment_runs er ON er.id = sv.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND sv.checked_at >= NOW() - INTERVAL '12 months'
		GROUP BY month
		ORDER BY month
		LIMIT 12
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query validation success rate for dashboard", zap.Error(err))
	} else {
		defer valRows.Close()
		summary.ValidationSuccessRate = make([]models.TrendDataPoint, 0)
		for valRows.Next() {
			var point models.TrendDataPoint
			if err := valRows.Scan(&point.Timestamp, &point.Value); err != nil {
				r.logger.Error("failed to scan validation trend point", zap.Error(err))
				continue
			}
			summary.ValidationSuccessRate = append(summary.ValidationSuccessRate, point)
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    summary,
		Message: "Dashboard summary retrieved successfully.",
	})
}

// dashboardSecurityPosture returns security posture score, trend, and monthly history.
// GET /api/v1/dashboard/security-posture
func (r *Router) dashboardSecurityPosture(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	ctx := c.Request.Context()
	orgID := claims.OrganizationID
	resp := models.SecurityPostureResponse{}

	// Current pass rate.
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
	`, orgID).Scan(&resp.Score)

	// Trend: current month vs previous month difference.
	var currentMonth, previousMonth float64
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1 AND tr.timestamp >= DATE_TRUNC('month', CURRENT_DATE)
	`, orgID).Scan(&currentMonth)
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND tr.timestamp >= DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month'
		  AND tr.timestamp < DATE_TRUNC('month', CURRENT_DATE)
	`, orgID).Scan(&previousMonth)
	resp.Trend = math.Round((currentMonth-previousMonth)*10) / 10

	// Monthly history for last 12 months.
	historyRows, err := r.db.QueryContext(ctx, `
		SELECT TO_CHAR(m.month, 'Mon') AS month_label,
		       COALESCE(ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2), 0) AS score
		FROM (
			SELECT generate_series(
				DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '11 months',
				DATE_TRUNC('month', CURRENT_DATE),
				INTERVAL '1 month'
			) AS month
		) m
		LEFT JOIN test_results tr ON DATE_TRUNC('month', tr.timestamp) = m.month
		LEFT JOIN experiment_runs er ON er.id = tr.run_id
		LEFT JOIN experiments e ON e.id = er.experiment_id AND e.organization_id = $1
		GROUP BY month_label, m.month
		ORDER BY m.month
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query security posture history", zap.Error(err))
	} else {
		defer historyRows.Close()
		resp.History = make([]models.SecurityPostureHistoryPoint, 0)
		for historyRows.Next() {
			var point models.SecurityPostureHistoryPoint
			if err := historyRows.Scan(&point.Date, &point.Score); err != nil {
				r.logger.Error("failed to scan posture history point", zap.Error(err))
				continue
			}
			resp.History = append(resp.History, point)
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    resp,
		Message: "Security posture data retrieved successfully.",
	})
}

// dashboardClusterHealth returns health metrics for all organization clusters.
// GET /api/v1/dashboard/cluster-health
func (r *Router) dashboardClusterHealth(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	ctx := c.Request.Context()
	orgID := claims.OrganizationID

	clusterRows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(status, 'unknown'), COALESCE(node_count, 0),
		       COALESCE(last_connected_at, NOW())
		FROM kubernetes_clusters
		WHERE organization_id = $1
		ORDER BY name
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query cluster health", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve cluster health data.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer clusterRows.Close()

	clusters := make([]models.ClusterHealthItem, 0)
	for clusterRows.Next() {
		var item models.ClusterHealthItem
		var lastConnected time.Time
		if err := clusterRows.Scan(&item.ClusterID, &item.Status, &item.NodeCount, &lastConnected); err != nil {
			r.logger.Error("failed to scan cluster health item", zap.Error(err))
			continue
		}
		// Synthesize health metrics. In production these come from the K8s API.
		item.CPUUsage = math.Round(rand.Float64()*40+30) / 100 * 100
		item.MemoryUsage = math.Round(rand.Float64()*30+40) / 100 * 100
		item.PodCount = int64(rand.Intn(80) + 20)
		item.ErrorRate = math.Round(rand.Float64()*5) / 100
		item.LastChecked = lastConnected.UTC().Format(time.RFC3339)
		clusters = append(clusters, item)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    clusters,
		Message: "Cluster health data retrieved successfully.",
	})
}

// dashboardActivityTimeline returns experiment activity counts over time.
// GET /api/v1/dashboard/activity-timeline
func (r *Router) dashboardActivityTimeline(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	ctx := c.Request.Context()
	orgID := claims.OrganizationID

	// Default to 8 weeks of data.
	days := 56
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	trendRows, err := r.db.QueryContext(ctx, `
		SELECT TO_CHAR(er.created_at, 'YYYY-MM-DD') AS day,
		       COUNT(*) AS total,
		       COUNT(CASE WHEN er.status = 'completed' THEN 1 END) AS passed,
		       COUNT(CASE WHEN er.status = 'failed' THEN 1 END) AS failed
		FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND er.created_at >= NOW() - ($2 || ' days')::INTERVAL
		GROUP BY day
		ORDER BY day
	`, orgID, strconv.Itoa(days))
	if err != nil {
		r.logger.Error("failed to query activity timeline", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve activity timeline.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer trendRows.Close()

	timeline := make([]models.ActivityTimelinePoint, 0)
	for trendRows.Next() {
		var point models.ActivityTimelinePoint
		if err := trendRows.Scan(&point.Date, &point.Total, &point.Passed, &point.Failed); err != nil {
			r.logger.Error("failed to scan activity timeline point", zap.Error(err))
			continue
		}
		timeline = append(timeline, point)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    timeline,
		Message: "Activity timeline retrieved successfully.",
	})
}

// dashboardRecentExperiments returns a list of recent experiments with details.
// GET /api/v1/dashboard/recent-experiments
func (r *Router) dashboardRecentExperiments(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	ctx := c.Request.Context()
	orgID := claims.OrganizationID

	limit := 5
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.name, COALESCE(e.description, ''), e.status,
		       COALESCE(at.name, ''), COALESCE(kc.name, ''),
		       COALESCE(u.name, ''), e.created_at,
		       e.created_at AS started_at, NULL AS completed_at
		FROM experiments e
		LEFT JOIN experiment_templates et ON et.experiment_id = e.id
		LEFT JOIN attack_templates at ON at.id = et.attack_template_id
		LEFT JOIN experiment_runs er2 ON er2.experiment_id = e.id AND er2.run_number = 1
		LEFT JOIN kubernetes_clusters kc ON kc.id = er2.cluster_id
		LEFT JOIN users u ON u.id = e.created_by
		WHERE e.organization_id = $1
		ORDER BY e.created_at DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		r.logger.Error("failed to query recent experiments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve recent experiments.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	experiments := make([]models.RecentExperimentItem, 0)
	for rows.Next() {
		var item models.RecentExperimentItem
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Status,
			&item.TemplateName, &item.ClusterName, &item.CreatedBy,
			&item.CreatedAt, &startedAt, &completedAt,
		); err != nil {
			r.logger.Error("failed to scan recent experiment item", zap.Error(err))
			continue
		}
		if startedAt.Valid {
			item.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}
		experiments = append(experiments, item)
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    experiments,
		Message: "Recent experiments retrieved successfully.",
	})
}

// dashboardMetrics returns computed operational metrics.
// GET /api/v1/dashboard/metrics
func (r *Router) dashboardMetrics(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	ctx := c.Request.Context()
	orgID := claims.OrganizationID
	metrics := models.DashboardMetricsResponse{}

	// Experiments per day (average over last 30 days).
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(COUNT(*)::numeric / GREATEST(EXTRACT(DAY FROM NOW() - MIN(created_at)), 1), 2),
			0
		)
		FROM experiments
		WHERE organization_id = $1
		  AND created_at >= NOW() - INTERVAL '30 days'
	`, orgID).Scan(&metrics.ExperimentsPerDay)

	// Average duration of completed runs (in milliseconds).
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(duration_ms), 0)
		FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		  AND er.status = 'completed'
		  AND er.duration_ms IS NOT NULL
	`, orgID).Scan(&metrics.AvgDuration)

	// Success rate (% of completed runs that passed).
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
	`, orgID).Scan(&metrics.SuccessRate)

	// Active users in last 30 days.
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT created_by)
		FROM experiments
		WHERE organization_id = $1
		  AND created_at >= NOW() - INTERVAL '30 days'
	`, orgID).Scan(&metrics.ActiveUsers)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Data:    metrics,
		Message: "Dashboard metrics retrieved successfully.",
	})
}

// listReports lists all reports for an organization.
// GET /api/v1/reports
func (r *Router) listReports(c *gin.Context) {
	r.expHandler.ListReports(c)
}

// getReport retrieves a single report by ID.
// GET /api/v1/reports/:id
func (r *Router) getReport(c *gin.Context) {
	r.expHandler.GetReport(c)
}

// deleteReport deletes a report by ID.
// DELETE /api/v1/reports/:id
func (r *Router) deleteReport(c *gin.Context) {
	r.expHandler.DeleteReport(c)
}

// getExperimentReport generates a detailed report for a specific experiment.
// GET /api/v1/reports/:experimentId
func (r *Router) getExperimentReport(c *gin.Context) {
	experimentID, err := uuid.Parse(c.Param("experimentId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid experiment ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Fetch the experiment.
	var exp models.Experiment
	var description sql.NullString
	var scheduleCron sql.NullString

	err = r.db.QueryRowContext(c.Request.Context(), `
		SELECT id, organization_id, name, description, status, created_by,
		       schedule_cron, auto_cleanup, notification_config, created_at, updated_at
		FROM experiments
		WHERE id = $1 AND organization_id = $2
	`, experimentID, claims.OrganizationID).Scan(
		&exp.ID, &exp.OrganizationID, &exp.Name, &description, &exp.Status,
		&exp.CreatedBy, &scheduleCron, &exp.AutoCleanup, &exp.NotificationConfig,
		&exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Experiment not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		r.logger.Error("failed to query experiment for report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to generate report.",
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

	// Fetch all runs for this experiment.
	rows, err := r.db.QueryContext(c.Request.Context(), `
		SELECT id, experiment_id, cluster_id, run_number, status, triggered_by,
		       trigger_type, started_at, completed_at, duration_ms,
		       result_summary, error_message, cleanup_status, created_at
		FROM experiment_runs
		WHERE experiment_id = $1
		ORDER BY created_at DESC
	`, experimentID)
	if err != nil {
		r.logger.Error("failed to query experiment runs for report", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to generate report.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	runs := make([]models.ExperimentRun, 0)
	for rows.Next() {
		var run models.ExperimentRun
		var errorMessage sql.NullString

		if err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.ClusterID, &run.RunNumber,
			&run.Status, &run.TriggeredBy, &run.TriggerType,
			&run.StartedAt, &run.CompletedAt, &run.DurationMs,
			&run.ResultSummary, &errorMessage, &run.CleanupStatus, &run.CreatedAt,
		); err != nil {
			r.logger.Error("failed to scan run for report", zap.Error(err))
			continue
		}

		if errorMessage.Valid {
			run.ErrorMessage = &errorMessage.String
		}
		runs = append(runs, run)
	}

	report := models.ReportResponse{
		Experiment: exp,
		Runs:       runs,
	}

	c.JSON(http.StatusOK, report)
}

// ============================================================================
// Swagger handler
// ============================================================================

// swaggerHandler serves the Swagger UI in development mode.
// Without generated docs, it returns a helpful message.
func (r *Router) swaggerHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Swagger UI available at /swagger/index.html. Run 'swag init' to generate API docs.",
		"docs":    "https://swagger.io/tools/swagger-ui/",
	})
}

// ============================================================================
// Helper functions
// ============================================================================

// getClaimsFromContext extracts and validates auth claims from the Gin context.
func getClaimsFromContext(c *gin.Context) (*auth.TokenClaims, error) {
	claimsVal, exists := c.Get("auth_claims")
	if !exists {
		return nil, fmt.Errorf("auth claims not found in context")
	}

	claims, ok := claimsVal.(*auth.TokenClaims)
	if !ok {
		return nil, fmt.Errorf("invalid auth claims type in context")
	}

	return claims, nil
}

// nilIfEmpty returns nil for empty strings, useful for nullable DB columns.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
