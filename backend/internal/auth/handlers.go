package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Handler holds dependencies for authentication HTTP handlers.
type Handler struct {
	authSvc *AuthService
	db      *sql.DB
	rdb     *redis.Client
	cfg     *config.Config
	logger  *zap.Logger
}

// NewHandler creates a new auth handler with the provided dependencies.
func NewHandler(db *sql.DB, rdb *redis.Client, cfg *config.Config, logger *zap.Logger) (*Handler, error) {
	authSvc, err := New(&cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}

	return &Handler{
		authSvc: authSvc,
		db:      db,
		rdb:     rdb,
		cfg:     cfg,
		logger:  logger.Named("auth_handler"),
	}, nil
}

// AuthService returns the underlying AuthService for use by middleware.
func (h *Handler) AuthService() *AuthService {
	return h.authSvc
}

// parsePermissions unmarshals a JSON array of permission strings from the database.
func parsePermissions(raw json.RawMessage) []string {
	if raw == nil {
		return nil
	}
	var perms []string
	if err := json.Unmarshal(raw, &perms); err != nil {
		return nil
	}
	return perms
}

// LoginHandler validates user credentials and returns JWT tokens.
// POST /api/v1/auth/login
func (h *Handler) LoginHandler(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Debug("invalid login request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body. Email and password are required.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Fetch user by email with role information
	var user models.User
	var roleName string
	var permissions json.RawMessage

	query := `
		SELECT u.id, u.email, u.password_hash, u.name, u.organization_id,
		       u.role_id, u.is_active, u.last_login_at, u.created_at, u.updated_at,
		       r.name, r.permissions
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.email = $1
	`

	var lastLoginAt sql.NullTime
	err := h.db.QueryRowContext(c.Request.Context(), query, req.Email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.OrganizationID, &user.RoleID, &user.IsActive,
		&lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		&roleName, &permissions,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.logger.Warn("login attempt with unknown email", zap.String("email", req.Email))
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid email or password.",
				Code:    http.StatusUnauthorized,
			})
			return
		}
		h.logger.Error("database error during login", zap.Error(err), zap.String("email", req.Email))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred. Please try again.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	// Check if account is active
	if !user.IsActive {
		h.logger.Warn("login attempt on inactive account", zap.String("user_id", user.ID.String()))
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "account_disabled",
			Message: "Your account has been disabled. Contact your administrator.",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Verify password
	if err := CheckPassword(req.Password, user.PasswordHash); err != nil {
		h.logger.Warn("failed login attempt - wrong password",
			zap.String("email", req.Email),
			zap.String("user_id", user.ID.String()),
		)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Invalid email or password.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Parse permissions from JSONB
	perms := parsePermissions(permissions)

	// Generate tokens using AuthService
	accessToken, _, err := h.authSvc.GenerateAccessToken(
		user.ID, user.Email, roleName, user.OrganizationID, perms,
	)
	if err != nil {
		h.logger.Error("failed to generate access token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_error",
			Message: "Failed to generate authentication token.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	refreshToken, _, err := h.authSvc.GenerateRefreshToken(
		user.ID, user.Email, roleName, user.OrganizationID,
	)
	if err != nil {
		h.logger.Error("failed to generate refresh token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_error",
			Message: "Failed to generate authentication token.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Update last login timestamp
	_, err = h.db.ExecContext(c.Request.Context(),
		"UPDATE users SET last_login_at = NOW() WHERE id = $1",
		user.ID,
	)
	if err != nil {
		h.logger.Warn("failed to update last_login_at", zap.Error(err), zap.String("user_id", user.ID.String()))
		// Non-fatal: continue with login
	}

	// Log successful login
	h.logger.Info("user logged in",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
		zap.String("role", roleName),
	)

	c.JSON(http.StatusOK, models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(h.cfg.JWT.Expiry.Seconds()),
		TokenType:    "Bearer",
	})
}

// RefreshHandler exchanges a valid refresh token for new access and refresh tokens.
// POST /api/v1/auth/refresh
func (h *Handler) RefreshHandler(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Refresh token is required.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Check if the token has been blacklisted in Redis
	blacklistKey := fmt.Sprintf("token:blacklist:%s", req.RefreshToken)
	blacklisted, err := h.rdb.Exists(c.Request.Context(), blacklistKey).Result()
	if err != nil && err != redis.Nil {
		h.logger.Error("redis error checking token blacklist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if blacklisted > 0 {
		h.logger.Warn("attempted use of blacklisted refresh token")
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "token_revoked",
			Message: "This refresh token has been revoked.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Validate and parse the refresh token using AuthService
	claims, err := h.authSvc.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		h.logger.Warn("invalid refresh token presented", zap.Error(err))
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "invalid_token",
			Message: "Invalid or expired refresh token.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Verify user still exists and is active
	var isActive bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT is_active FROM users WHERE id = $1",
		claims.UserID,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error:   "user_not_found",
				Message: "User account no longer exists.",
				Code:    http.StatusUnauthorized,
			})
			return
		}
		h.logger.Error("database error during token refresh", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if !isActive {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "account_disabled",
			Message: "Your account has been disabled.",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Blacklist the old refresh token to prevent reuse
	if claims.ExpiresAt != nil {
		oldRefreshTTL := time.Until(claims.ExpiresAt.Time)
		if oldRefreshTTL > 0 {
			if err := h.rdb.Set(c.Request.Context(), blacklistKey, "1", oldRefreshTTL).Err(); err != nil {
				h.logger.Error("failed to blacklist old refresh token", zap.Error(err))
				// Continue: this is non-fatal, though token reuse is slightly more risky
			}
		}
	}

	// Fetch fresh user data and role/permissions for new tokens
	var user models.User
	var roleName string
	var permissions json.RawMessage

	query := `
		SELECT u.id, u.email, u.password_hash, u.name, u.organization_id,
		       u.role_id, u.is_active, u.last_login_at, u.created_at, u.updated_at,
		       r.name, r.permissions
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.id = $1
	`

	var lastLoginAt sql.NullTime
	err = h.db.QueryRowContext(c.Request.Context(), query, claims.UserID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.OrganizationID, &user.RoleID, &user.IsActive,
		&lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		&roleName, &permissions,
	)
	if err != nil {
		h.logger.Error("failed to fetch user for token refresh", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	// Parse fresh permissions from the database (may have changed since last token)
	perms := parsePermissions(permissions)

	// Generate new token pair
	accessToken, _, err := h.authSvc.GenerateAccessToken(
		user.ID, user.Email, roleName, user.OrganizationID, perms,
	)
	if err != nil {
		h.logger.Error("failed to generate access token on refresh", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_error",
			Message: "Failed to generate authentication token.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	newRefreshToken, _, err := h.authSvc.GenerateRefreshToken(
		user.ID, user.Email, roleName, user.OrganizationID,
	)
	if err != nil {
		h.logger.Error("failed to generate refresh token on refresh", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "token_error",
			Message: "Failed to generate authentication token.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("token refreshed",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email),
	)

	c.JSON(http.StatusOK, models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(h.cfg.JWT.Expiry.Seconds()),
		TokenType:    "Bearer",
	})
}

// LogoutHandler invalidates the current session by blacklisting the token.
// POST /api/v1/auth/logout
func (h *Handler) LogoutHandler(c *gin.Context) {
	// Extract the access token from the Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "missing_token",
			Message: "No authorization token provided.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	tokenString := extractBearerToken(authHeader)
	if tokenString == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_token_format",
			Message: "Invalid authorization header format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate the token to get its expiry
	claims, err := h.authSvc.ValidateToken(tokenString)
	if err != nil {
		// Token is already invalid/expired — nothing to blacklist
		h.logger.Debug("logout with already-invalid token", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"message": "logged out"})
		return
	}

	// Blacklist the access token in Redis until its natural expiry
	if claims.ExpiresAt != nil {
		tokenTTL := time.Until(claims.ExpiresAt.Time)
		if tokenTTL > 0 {
			blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenString)
			if err := h.rdb.Set(c.Request.Context(), blacklistKey, "1", tokenTTL).Err(); err != nil {
				h.logger.Error("failed to blacklist access token on logout", zap.Error(err))
				// Still return success to the client — the token will expire naturally
			}
		}
	}

	h.logger.Info("user logged out",
		zap.String("user_id", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// MeHandler returns the currently authenticated user's profile information.
// GET /api/v1/auth/me
func (h *Handler) MeHandler(c *gin.Context) {
	// Retrieve claims from context (set by AuthMiddleware)
	claimsVal, exists := c.Get("auth_claims")
	if !exists {
		h.logger.Error("auth claims not found in context")
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	claims, ok := claimsVal.(*TokenClaims)
	if !ok {
		h.logger.Error("invalid auth claims type in context")
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Authentication context error.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Fetch fresh user data from database
	var user models.User
	var roleName string
	var roleDescription string
	var permissions json.RawMessage

	query := `
		SELECT u.id, u.email, u.password_hash, u.name, u.organization_id,
		       u.role_id, u.is_active, u.last_login_at, u.created_at, u.updated_at,
		       r.name, r.description, r.permissions
		FROM users u
		JOIN roles r ON r.id = u.role_id
		WHERE u.id = $1
	`

	var lastLoginAt sql.NullTime
	err := h.db.QueryRowContext(c.Request.Context(), query, claims.UserID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.OrganizationID, &user.RoleID, &user.IsActive,
		&lastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		&roleName, &roleDescription, &permissions,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "user_not_found",
				Message: "User account not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("database error fetching current user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	response := models.UserResponse{
		ID:             user.ID,
		Email:          user.Email,
		Name:           user.Name,
		OrganizationID: user.OrganizationID,
		RoleID:         user.RoleID,
		IsActive:       user.IsActive,
		LastLoginAt:    user.LastLoginAt,
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
		Role: &models.RoleResponse{
			ID:          user.RoleID,
			Name:        roleName,
			Description: roleDescription,
			Permissions: permissions,
		},
	}

	c.JSON(http.StatusOK, response)
}

// RegisterHandler creates a new user account. Requires admin:all or users:manage permission.
// POST /api/v1/auth/register
func (h *Handler) RegisterHandler(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Debug("invalid registration request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: fmt.Sprintf("Invalid request body: %s", err.Error()),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Get the requesting user's claims from context (set by AuthMiddleware)
	claimsVal, exists := c.Get("auth_claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	claims, ok := claimsVal.(*TokenClaims)
	if !ok {
		h.logger.Error("invalid auth claims type in context during registration")
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Authentication context error.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Verify the requesting user has permission to create users
	if !claims.HasPermission("admin:all") && !claims.HasPermission("users:manage") {
		h.logger.Warn("unauthorized user registration attempt",
			zap.String("actor_id", claims.UserID.String()),
		)
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have permission to create users.",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Parse and validate the target organization ID
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid organization ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Verify the target organization exists and is active
	var orgExists bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1 AND status = 'active')",
		orgID,
	).Scan(&orgExists)
	if err != nil {
		h.logger.Error("database error checking organization", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if !orgExists {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_organization",
			Message: "Organization not found or inactive.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Parse and validate the target role ID
	roleID, err := uuid.Parse(req.RoleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid role ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Verify the target role exists
	var roleExists bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)",
		roleID,
	).Scan(&roleExists)
	if err != nil {
		h.logger.Error("database error checking role", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if !roleExists {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_role",
			Message: "Role not found.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Non-admin users can only create users within their own organization
	if !claims.IsAdmin() && claims.OrganizationID != orgID {
		h.logger.Warn("user attempted to create user in different organization",
			zap.String("actor_id", claims.UserID.String()),
			zap.String("actor_org_id", claims.OrganizationID.String()),
			zap.String("target_org_id", orgID.String()),
		)
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "forbidden",
			Message: "You can only create users within your own organization.",
			Code:    http.StatusForbidden,
		})
		return
	}

	// Check if email is already taken
	var emailTaken bool
	err = h.db.QueryRowContext(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)",
		req.Email,
	).Scan(&emailTaken)
	if err != nil {
		h.logger.Error("database error checking email uniqueness", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "An internal error occurred.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	if emailTaken {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "email_exists",
			Message: "A user with this email already exists.",
			Code:    http.StatusConflict,
		})
		return
	}

	// Hash the password
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password during registration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create user account.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Insert the new user and return the created record
	var newUser models.User
	var newLastLoginAt sql.NullTime

	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO users (email, password_hash, name, organization_id, role_id, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING id, email, password_hash, name, organization_id, role_id, is_active, last_login_at, created_at, updated_at
	`, req.Email, passwordHash, req.Name, orgID, roleID).Scan(
		&newUser.ID, &newUser.Email, &newUser.PasswordHash, &newUser.Name,
		&newUser.OrganizationID, &newUser.RoleID, &newUser.IsActive,
		&newLastLoginAt, &newUser.CreatedAt, &newUser.UpdatedAt,
	)
	if err != nil {
		h.logger.Error("failed to insert new user", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create user account.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if newLastLoginAt.Valid {
		newUser.LastLoginAt = &newLastLoginAt.Time
	}

	h.logger.Info("new user registered",
		zap.String("user_id", newUser.ID.String()),
		zap.String("email", newUser.Email),
		zap.String("organization_id", orgID.String()),
		zap.String("role_id", roleID.String()),
		zap.String("registered_by", claims.UserID.String()),
	)

	// Return the created user (without password hash — it's tagged json:"-")
	response := models.UserResponse{
		ID:             newUser.ID,
		Email:          newUser.Email,
		Name:           newUser.Name,
		OrganizationID: newUser.OrganizationID,
		RoleID:         newUser.RoleID,
		IsActive:       newUser.IsActive,
		LastLoginAt:    newUser.LastLoginAt,
		CreatedAt:      newUser.CreatedAt,
		UpdatedAt:      newUser.UpdatedAt,
	}

	c.JSON(http.StatusCreated, response)
}

// extractBearerToken extracts the Bearer token from an Authorization header.
// Returns an empty string if the header format is invalid.
func extractBearerToken(authHeader string) string {
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return ""
}
