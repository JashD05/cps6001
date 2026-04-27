package router

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	expHandler := experiment.NewHandler(db.DB, rdb, cfg, logger)

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

		// --- Dashboard route ---
		dashboard := v1.Group("/dashboard")
		dashboard.Use(r.middleware.AuthMiddleware())
		{
			dashboard.GET("/summary",
				r.middleware.RBACMiddleware("experiments:read"),
				r.dashboardSummary,
			)
		}

		// --- Reports route ---
		reports := v1.Group("/reports")
		reports.Use(r.middleware.AuthMiddleware())
		{
			reports.GET("/:experimentId",
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

// dashboardSummary returns aggregated statistics for the organization dashboard.
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

	// Total experiments.
	_ = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1", orgID,
	).Scan(&summary.TotalExperiments)

	// Active experiments.
	_ = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM experiments WHERE organization_id = $1 AND status = 'active'", orgID,
	).Scan(&summary.ActiveExperiments)

	// Total runs.
	_ = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
	`, orgID).Scan(&summary.TotalRuns)

	// Runs in last 24 hours.
	_ = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1 AND er.created_at >= NOW() - INTERVAL '24 hours'
	`, orgID).Scan(&summary.RunsLast24h)

	// Pass rate across all test results.
	_ = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			ROUND(100.0 * COUNT(CASE WHEN tr.status = 'passed' THEN 1 END) / NULLIF(COUNT(*), 0), 2),
			0
		)
		FROM test_results tr
		JOIN experiment_runs er ON er.id = tr.run_id
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
	`, orgID).Scan(&summary.PassRate)

	// Recent runs (last 10).
	rows, err := r.db.QueryContext(ctx, `
		SELECT er.id, er.experiment_id, e.name, er.status,
		       er.started_at, er.completed_at, er.duration_ms
		FROM experiment_runs er
		JOIN experiments e ON e.id = er.experiment_id
		WHERE e.organization_id = $1
		ORDER BY er.created_at DESC
		LIMIT 10
	`, orgID)
	if err != nil {
		r.logger.Error("failed to query recent runs for dashboard", zap.Error(err))
	} else {
		defer rows.Close()
		summary.RecentRuns = make([]models.ExperimentRunSummary, 0)
		for rows.Next() {
			var rs models.ExperimentRunSummary
			if err := rows.Scan(
				&rs.ID, &rs.ExperimentID, &rs.Name, &rs.Status,
				&rs.StartedAt, &rs.CompletedAt, &rs.DurationMs,
			); err != nil {
				r.logger.Error("failed to scan recent run summary", zap.Error(err))
				continue
			}
			summary.RecentRuns = append(summary.RecentRuns, rs)
		}
	}

	c.JSON(http.StatusOK, summary)
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
