package attack

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaos-sec/backend/internal/middleware"
)

// ---------------------------------------------------------------------------
// Handler – HTTP handlers for attack template CRUD
// ---------------------------------------------------------------------------

// Handler provides HTTP handlers for managing attack templates. It integrates
// with the module Registry so that built-in modules are automatically reflected
// in the template list alongside any user-defined templates stored in the
// database.
type Handler struct {
	db       *sql.DB
	registry *Registry
	logger   *zap.Logger
}

// NewHandler creates a new attack template handler with the provided dependencies.
func NewHandler(db *sql.DB, registry *Registry, logger *zap.Logger) *Handler {
	return &Handler{
		db:       db,
		registry: registry,
		logger:   logger.Named("attack_handler"),
	}
}

// ---------------------------------------------------------------------------
// ListTemplatesHandler
// ---------------------------------------------------------------------------

// ListTemplatesHandler lists all attack templates with optional filtering by
// category and severity. It merges built-in module definitions from the
// Registry with user-defined templates stored in the database.
//
// GET /api/v1/attack-templates?page=1&page_size=20&category=network&severity=high&search=cidr
func (h *Handler) ListTemplatesHandler(c *gin.Context) {
	page := 1
	pageSize := 20
	category := c.Query("category")
	severity := c.Query("severity")
	search := c.Query("search")
	includeModules := c.Query("include_modules")

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
	if err := h.db.QueryRowContext(c.Request.Context(), countQuery, args...).Scan(&total); err != nil {
		h.logger.Error("failed to count attack templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve templates.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Fetch paginated results.
	offset := (page - 1) * pageSize
	dataQuery := `SELECT id, name, slug, category, severity, description, mitre_attack_id,
		k8s_manifest, parameters, prerequisites, expected_behavior, mitigation,
		is_active, is_system, created_at, updated_at
		FROM attack_templates` +
		whereClause +
		" ORDER BY name ASC LIMIT $" + strconv.Itoa(argIdx) +
		" OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, pageSize, offset)

	rows, err := h.db.QueryContext(c.Request.Context(), dataQuery, args...)
	if err != nil {
		h.logger.Error("failed to query attack templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve templates.",
			"code":    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	// unifiedTemplateItem is a common response type for both database templates
	// and built-in attack modules. Using interface{} keeps the response flexible.
	type unifiedTemplateItem struct {
		ID               interface{}     `json:"id"`
		Name             string          `json:"name"`
		Slug             string          `json:"slug"`
		Category         string          `json:"category"`
		Severity         string          `json:"severity"`
		Description      string          `json:"description"`
		MitreAttackID    *string         `json:"mitre_attack_id"`
		K8sManifest      json.RawMessage `json:"k8s_manifest,omitempty"`
		Parameters       interface{}     `json:"parameters"`
		Prerequisites    json.RawMessage `json:"prerequisites,omitempty"`
		ExpectedBehavior string          `json:"expected_behavior,omitempty"`
		Mitigation       string          `json:"mitigation,omitempty"`
		IsActive         bool            `json:"is_active"`
		IsSystem         bool            `json:"is_system"`
		IsModule         bool            `json:"is_module,omitempty"`
		Schema           interface{}     `json:"schema,omitempty"`
		CreatedAt        *time.Time      `json:"created_at,omitempty"`
		UpdatedAt        *time.Time      `json:"updated_at,omitempty"`
	}

	combined := make([]unifiedTemplateItem, 0)

	for rows.Next() {
		var id uuid.UUID
		var name, slug, cat, sev, desc, expectedBehavior string
		var mitreID sql.NullString
		var k8sManifest, parameters json.RawMessage
		var prerequisites sql.NullString
		var mitigation sql.NullString
		var isActive, isSystem bool
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&id, &name, &slug, &cat, &sev,
			&desc, &mitreID, &k8sManifest, &parameters,
			&prerequisites, &expectedBehavior, &mitigation,
			&isActive, &isSystem, &createdAt, &updatedAt,
		); err != nil {
			h.logger.Error("failed to scan template row", zap.Error(err))
			continue
		}

		item := unifiedTemplateItem{
			ID:               id,
			Name:             name,
			Slug:             slug,
			Category:         cat,
			Severity:         sev,
			Description:      desc,
			K8sManifest:      k8sManifest,
			Parameters:       parameters,
			ExpectedBehavior: expectedBehavior,
			IsActive:         isActive,
			IsSystem:         isSystem,
			CreatedAt:        &createdAt,
			UpdatedAt:        &updatedAt,
		}
		if mitreID.Valid {
			item.MitreAttackID = &mitreID.String
		}
		if prerequisites.Valid {
			item.Prerequisites = json.RawMessage(prerequisites.String)
		} else {
			item.Prerequisites = json.RawMessage(`[]`)
		}
		if mitigation.Valid {
			item.Mitigation = mitigation.String
		}

		combined = append(combined, item)
	}

	// Optionally include built-in modules from the registry.
	moduleCount := 0
	if includeModules == "true" || includeModules == "1" {
		var modules []AttackModule
		if category != "" {
			modules = h.registry.ListByCategory(category)
		} else {
			modules = h.registry.List()
		}

		for _, m := range modules {
			if severity != "" && m.Severity() != severity {
				continue
			}
			if search != "" {
				lowerSearch := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(m.Name()), lowerSearch) &&
					!strings.Contains(strings.ToLower(m.Description()), lowerSearch) {
					continue
				}
			}

			schema := ParametersToJSONSchema(m)
			paramDefs := make([]ParameterSchema, len(m.Parameters()))
			for i, p := range m.Parameters() {
				paramDefs[i] = ParameterSchema{
					Name:        p.Name,
					Type:        string(p.Type),
					Required:    p.Required,
					Default:     p.Default,
					Description: p.Description,
					Options:     p.Options,
				}
			}

			combined = append(combined, unifiedTemplateItem{
				ID:          m.ID(),
				Name:        m.Name(),
				Slug:        m.ID(),
				Category:    m.Category(),
				Severity:    m.Severity(),
				Description: m.Description(),
				Parameters:  paramDefs,
				IsSystem:    true,
				IsModule:    true,
				Schema:      schema,
				IsActive:    true,
			})
			moduleCount++
		}

		total += int64(moduleCount)
	}

	// Build combined response.
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        combined,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}

// ---------------------------------------------------------------------------
// GetTemplateHandler
// ---------------------------------------------------------------------------

// GetTemplateHandler returns a single attack template by ID or slug.
//
// GET /api/v1/attack-templates/:identifier
func (h *Handler) GetTemplateHandler(c *gin.Context) {
	identifier := c.Param("id")
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Template identifier is required.",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Try to parse as UUID first; if it fails, treat as slug.
	templateID, parseErr := uuid.Parse(identifier)
	if parseErr != nil {
		// It's a slug – look up by slug.
		h.getTemplateBySlug(c, identifier)
		return
	}

	h.getTemplateByID(c, templateID)
}

// getTemplateByID fetches a template by its UUID.
func (h *Handler) getTemplateByID(c *gin.Context, templateID uuid.UUID) {
	var t struct {
		ID               uuid.UUID       `json:"id"`
		Name             string          `json:"name"`
		Slug             string          `json:"slug"`
		Category         string          `json:"category"`
		Severity         string          `json:"severity"`
		Description      string          `json:"description"`
		MitreAttackID    *string         `json:"mitre_attack_id"`
		K8sManifest      json.RawMessage `json:"k8s_manifest"`
		Parameters       json.RawMessage `json:"parameters"`
		Prerequisites    json.RawMessage `json:"prerequisites"`
		ExpectedBehavior string          `json:"expected_behavior"`
		Mitigation       string          `json:"mitigation"`
		IsActive         bool            `json:"is_active"`
		IsSystem         bool            `json:"is_system"`
		CreatedAt        time.Time       `json:"created_at"`
		UpdatedAt        time.Time       `json:"updated_at"`
	}

	var mitreID sql.NullString
	var prerequisites sql.NullString
	var mitigation sql.NullString

	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, name, slug, category, severity, description, mitre_attack_id,
		       k8s_manifest, parameters, prerequisites, expected_behavior, mitigation,
		       is_active, is_system, created_at, updated_at
		FROM attack_templates
		WHERE id = $1
	`, templateID).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.Severity, &t.Description, &mitreID,
		&t.K8sManifest, &t.Parameters, &prerequisites, &t.ExpectedBehavior, &mitigation,
		&t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Attack template not found.",
				"code":    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if mitreID.Valid {
		t.MitreAttackID = &mitreID.String
	}
	if prerequisites.Valid {
		t.Prerequisites = json.RawMessage(prerequisites.String)
	} else {
		t.Prerequisites = json.RawMessage(`[]`)
	}
	if mitigation.Valid {
		t.Mitigation = mitigation.String
	}

	c.JSON(http.StatusOK, t)
}

// getTemplateBySlug fetches a template by its slug. If no database row matches,
// it falls back to checking the built-in module registry.
func (h *Handler) getTemplateBySlug(c *gin.Context, slug string) {
	var t struct {
		ID               uuid.UUID       `json:"id"`
		Name             string          `json:"name"`
		Slug             string          `json:"slug"`
		Category         string          `json:"category"`
		Severity         string          `json:"severity"`
		Description      string          `json:"description"`
		MitreAttackID    *string         `json:"mitre_attack_id"`
		K8sManifest      json.RawMessage `json:"k8s_manifest"`
		Parameters       json.RawMessage `json:"parameters"`
		Prerequisites    json.RawMessage `json:"prerequisites"`
		ExpectedBehavior string          `json:"expected_behavior"`
		Mitigation       string          `json:"mitigation"`
		IsActive         bool            `json:"is_active"`
		IsSystem         bool            `json:"is_system"`
		CreatedAt        time.Time       `json:"created_at"`
		UpdatedAt        time.Time       `json:"updated_at"`
	}

	var mitreID sql.NullString
	var prerequisites sql.NullString
	var mitigation sql.NullString

	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, name, slug, category, severity, description, mitre_attack_id,
		       k8s_manifest, parameters, prerequisites, expected_behavior, mitigation,
		       is_active, is_system, created_at, updated_at
		FROM attack_templates
		WHERE slug = $1
	`, slug).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.Severity, &t.Description, &mitreID,
		&t.K8sManifest, &t.Parameters, &prerequisites, &t.ExpectedBehavior, &mitigation,
		&t.IsActive, &t.IsSystem, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == nil {
		// Found in the database.
		if mitreID.Valid {
			t.MitreAttackID = &mitreID.String
		}
		if prerequisites.Valid {
			t.Prerequisites = json.RawMessage(prerequisites.String)
		} else {
			t.Prerequisites = json.RawMessage(`[]`)
		}
		if mitigation.Valid {
			t.Mitigation = mitigation.String
		}
		c.JSON(http.StatusOK, t)
		return
	}

	if err != sql.ErrNoRows {
		h.logger.Error("failed to query attack template by slug", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Not in database – check the built-in module registry.
	module, modErr := h.registry.Get(slug)
	if modErr != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "Attack template not found.",
			"code":    http.StatusNotFound,
		})
		return
	}

	// Build a response from the built-in module definition.
	schema := ParametersToJSONSchema(module)
	schemaJSON, _ := json.Marshal(schema)
	paramDefs := make([]ParameterSchema, len(module.Parameters()))
	for i, p := range module.Parameters() {
		paramDefs[i] = ParameterSchema{
			Name:        p.Name,
			Type:        string(p.Type),
			Required:    p.Required,
			Default:     p.Default,
			Description: p.Description,
			Options:     p.Options,
		}
	}
	paramsJSON, _ := json.Marshal(paramDefs)

	c.JSON(http.StatusOK, gin.H{
		"id":                slug,
		"name":              module.Name(),
		"slug":              module.ID(),
		"category":          module.Category(),
		"severity":          module.Severity(),
		"description":       module.Description(),
		"mitre_attack_id":   nil,
		"k8s_manifest":      nil,
		"parameters":        json.RawMessage(paramsJSON),
		"prerequisites":     json.RawMessage(`[]`),
		"expected_behavior": module.Description(),
		"mitigation":        "",
		"is_active":         true,
		"is_system":         true,
		"is_module":         true,
		"schema":            json.RawMessage(schemaJSON),
	})
}

// ---------------------------------------------------------------------------
// CreateTemplateHandler
// ---------------------------------------------------------------------------

// createTemplateRequest is the request body for creating a new attack template.
type createTemplateRequest struct {
	Name             string          `json:"name" binding:"required,min=2,max=255"`
	Slug             string          `json:"slug" binding:"required,min=2,max=255"`
	Category         string          `json:"category" binding:"required,oneof=network rbac security resource privilege data availability"`
	Severity         string          `json:"severity" binding:"required,oneof=low medium high critical"`
	Description      string          `json:"description" binding:"required"`
	MitreAttackID    *string         `json:"mitre_attack_id"`
	K8sManifest      json.RawMessage `json:"k8s_manifest" binding:"required"`
	Parameters       json.RawMessage `json:"parameters" binding:"required"`
	Prerequisites    json.RawMessage `json:"prerequisites"`
	ExpectedBehavior string          `json:"expected_behavior" binding:"required"`
	Mitigation       string          `json:"mitigation"`
}

// CreateTemplateHandler creates a new custom attack template. Only admin users
// can create templates. System templates cannot be created through this endpoint.
//
// POST /api/v1/attack-templates
func (h *Handler) CreateTemplateHandler(c *gin.Context) {
	// Verify admin permission.
	if !h.hasAdminPermission(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "Only administrators can create attack templates.",
			"code":    http.StatusForbidden,
		})
		return
	}

	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	prerequisites := req.Prerequisites
	if prerequisites == nil {
		prerequisites = json.RawMessage(`[]`)
	}

	// Check slug uniqueness.
	var exists bool
	err := h.db.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM attack_templates WHERE slug = $1)", req.Slug,
	).Scan(&exists)
	if err != nil {
		h.logger.Error("failed to check slug uniqueness", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to validate template slug.",
			"code":    http.StatusInternalServerError,
		})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "conflict",
			"message": "A template with this slug already exists.",
			"code":    http.StatusConflict,
		})
		return
	}

	// Also check against built-in module IDs.
	if _, err := h.registry.Get(req.Slug); err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "conflict",
			"message": "A built-in module with this slug already exists.",
			"code":    http.StatusConflict,
		})
		return
	}

	var id uuid.UUID
	var createdAt, updatedAt time.Time

	err = h.db.QueryRowContext(c.Request.Context(), `
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
		h.logger.Error("failed to insert attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to create template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("attack template created",
		zap.String("template_id", id.String()),
		zap.String("slug", req.Slug),
	)

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"slug":       req.Slug,
		"category":   req.Category,
		"severity":   req.Severity,
		"created_at": createdAt,
		"updated_at": updatedAt,
	})
}

// ---------------------------------------------------------------------------
// UpdateTemplateHandler
// ---------------------------------------------------------------------------

// updateTemplateRequest is the request body for updating an attack template.
type updateTemplateRequest struct {
	Name             *string         `json:"name" binding:"omitempty,min=2,max=255"`
	Slug             *string         `json:"slug" binding:"omitempty,min=2,max=255"`
	Category         *string         `json:"category" binding:"omitempty,oneof=network rbac security resource privilege data availability"`
	Severity         *string         `json:"severity" binding:"omitempty,oneof=low medium high critical"`
	Description      *string         `json:"description"`
	MitreAttackID    *string         `json:"mitre_attack_id"`
	K8sManifest      json.RawMessage `json:"k8s_manifest"`
	Parameters       json.RawMessage `json:"parameters"`
	Prerequisites    json.RawMessage `json:"prerequisites"`
	ExpectedBehavior *string         `json:"expected_behavior"`
	Mitigation       *string         `json:"mitigation"`
	IsActive         *bool           `json:"is_active"`
}

// UpdateTemplateHandler updates an existing attack template. System templates
// cannot be updated through this endpoint (their is_system flag is true).
//
// PUT /api/v1/attack-templates/:id
func (h *Handler) UpdateTemplateHandler(c *gin.Context) {
	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid template ID format.",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check that the template exists and is not a system template.
	var isSystem bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT is_system FROM attack_templates WHERE id = $1", templateID,
	).Scan(&isSystem)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Attack template not found.",
				"code":    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if isSystem {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "System templates cannot be updated.",
			"code":    http.StatusForbidden,
		})
		return
	}

	var req updateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "Invalid request body: " + err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Build dynamic UPDATE query.
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, "name = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Slug != nil {
		// Check new slug uniqueness.
		var slugExists bool
		slugCheckErr := h.db.QueryRowContext(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM attack_templates WHERE slug = $1 AND id != $2)",
			*req.Slug, templateID,
		).Scan(&slugExists)
		if slugCheckErr != nil {
			h.logger.Error("failed to check slug uniqueness", zap.Error(slugCheckErr))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Failed to validate slug.",
				"code":    http.StatusInternalServerError,
			})
			return
		}
		if slugExists {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "conflict",
				"message": "A template with this slug already exists.",
				"code":    http.StatusConflict,
			})
			return
		}
		// Also check against built-in module IDs.
		if _, modErr := h.registry.Get(*req.Slug); modErr == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "conflict",
				"message": "A built-in module with this slug already exists.",
				"code":    http.StatusConflict,
			})
			return
		}
		setClauses = append(setClauses, "slug = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Slug)
		argIdx++
	}
	if req.Category != nil {
		setClauses = append(setClauses, "category = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Category)
		argIdx++
	}
	if req.Severity != nil {
		setClauses = append(setClauses, "severity = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Severity)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, "description = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.MitreAttackID != nil {
		setClauses = append(setClauses, "mitre_attack_id = $"+strconv.Itoa(argIdx))
		args = append(args, *req.MitreAttackID)
		argIdx++
	}
	if req.K8sManifest != nil {
		setClauses = append(setClauses, "k8s_manifest = $"+strconv.Itoa(argIdx))
		args = append(args, req.K8sManifest)
		argIdx++
	}
	if req.Parameters != nil {
		setClauses = append(setClauses, "parameters = $"+strconv.Itoa(argIdx))
		args = append(args, req.Parameters)
		argIdx++
	}
	if req.Prerequisites != nil {
		setClauses = append(setClauses, "prerequisites = $"+strconv.Itoa(argIdx))
		args = append(args, req.Prerequisites)
		argIdx++
	}
	if req.ExpectedBehavior != nil {
		setClauses = append(setClauses, "expected_behavior = $"+strconv.Itoa(argIdx))
		args = append(args, *req.ExpectedBehavior)
		argIdx++
	}
	if req.Mitigation != nil {
		setClauses = append(setClauses, "mitigation = $"+strconv.Itoa(argIdx))
		args = append(args, *req.Mitigation)
		argIdx++
	}
	if req.IsActive != nil {
		setClauses = append(setClauses, "is_active = $"+strconv.Itoa(argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "No fields to update.",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Always update updated_at.
	setClauses = append(setClauses, "updated_at = NOW()")

	query := "UPDATE attack_templates SET " + strings.Join(setClauses, ", ") +
		" WHERE id = $" + strconv.Itoa(argIdx) + " RETURNING updated_at"
	args = append(args, templateID)

	var updatedAt time.Time
	if err := h.db.QueryRowContext(c.Request.Context(), query, args...).Scan(&updatedAt); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Attack template not found.",
				"code":    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to update attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to update template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("attack template updated",
		zap.String("template_id", templateID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"id":         templateID,
		"updated_at": updatedAt,
	})
}

// ---------------------------------------------------------------------------
// DeleteTemplateHandler
// ---------------------------------------------------------------------------

// DeleteTemplateHandler deletes a non-system attack template. System templates
// cannot be deleted. The handler also checks for references in experiment_templates
// and returns a conflict if the template is still in use.
//
// DELETE /api/v1/attack-templates/:id
func (h *Handler) DeleteTemplateHandler(c *gin.Context) {
	templateID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_id",
			"message": "Invalid template ID format.",
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check that the template exists and is not a system template.
	var name string
	var isSystem bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT name, is_system FROM attack_templates WHERE id = $1", templateID,
	).Scan(&name, &isSystem)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Attack template not found.",
				"code":    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to retrieve template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if isSystem {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "System templates cannot be deleted.",
			"code":    http.StatusForbidden,
		})
		return
	}

	// Check for references in experiment_templates.
	var refCount int64
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT COUNT(*) FROM experiment_templates WHERE attack_template_id = $1", templateID,
	).Scan(&refCount)
	if err != nil {
		h.logger.Error("failed to check template references", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to check template references.",
			"code":    http.StatusInternalServerError,
		})
		return
	}
	if refCount > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"error":     "conflict",
			"message":   "Cannot delete template: it is referenced by experiment templates. Remove the references first or deactivate the template instead.",
			"code":      http.StatusConflict,
			"ref_count": refCount,
		})
		return
	}

	// Delete the template.
	result, err := h.db.ExecContext(c.Request.Context(),
		"DELETE FROM attack_templates WHERE id = $1 AND is_system = false", templateID,
	)
	if err != nil {
		h.logger.Error("failed to delete attack template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "Failed to delete template.",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "Attack template not found or is a system template.",
			"code":    http.StatusNotFound,
		})
		return
	}

	h.logger.Info("attack template deleted",
		zap.String("template_id", templateID.String()),
		zap.String("name", name),
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Template deleted successfully.",
		"id":      templateID,
	})
}

// ---------------------------------------------------------------------------
// Helper: check admin permission
// ---------------------------------------------------------------------------

// hasAdminPermission checks if the authenticated user has admin-level
// permissions by inspecting the JWT claims stored in the Gin context.
func (h *Handler) hasAdminPermission(c *gin.Context) bool {
	claimsVal, exists := c.Get(string(middleware.ClaimsContextKey))
	if !exists {
		return false
	}

	// The middleware stores *auth.TokenClaims which has a Permissions field.
	// We use a simple type assertion to check for admin permission.
	type claimsWithPermissions interface {
		GetPermissions() []string
	}

	// Try the interface approach first.
	if claims, ok := claimsVal.(claimsWithPermissions); ok {
		for _, perm := range claims.GetPermissions() {
			if perm == "admin:all" || perm == "templates:write" {
				return true
			}
		}
	}

	// Fallback: use reflection-like approach by checking the underlying struct.
	// The auth.TokenClaims struct has Permissions []string and a helper IsAdmin().
	type adminClaims interface {
		IsAdmin() bool
	}
	if claims, ok := claimsVal.(adminClaims); ok {
		return claims.IsAdmin()
	}

	return false
}
