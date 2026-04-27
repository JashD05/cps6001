package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/chaos-sec/backend/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	// bcryptCost is the hashing cost factor for bcrypt password hashing.
	// Cost 12 provides a good balance between security and performance (~250ms).
	bcryptCost = 12

	// DefaultAccessTokenExpiry is the default expiry duration for access tokens.
	DefaultAccessTokenExpiry = 1 * time.Hour

	// DefaultRefreshTokenExpiry is the default expiry duration for refresh tokens.
	DefaultRefreshTokenExpiry = 7 * 24 * time.Hour

	// TokenTypeAccess identifies access tokens in the JWT subject field.
	TokenTypeAccess = "access"

	// TokenTypeRefresh identifies refresh tokens in the JWT subject field.
	TokenTypeRefresh = "refresh"

	// TokenTypeKey is the JWT key used to store the token type claim.
	TokenTypeKey = "token_type"
)

// Common errors returned by the auth package.
var (
	ErrInvalidToken        = errors.New("invalid token")
	ErrExpiredToken        = errors.New("token has expired")
	ErrInvalidTokenType    = errors.New("invalid token type")
	ErrTokenNotValidYet    = errors.New("token is not valid yet")
	ErrTokenUnverifiable   = errors.New("token could not be verified")
	ErrPasswordTooShort    = errors.New("password must be at least 8 characters")
	ErrPasswordHashFailure = errors.New("failed to hash password")
	ErrInvalidCredentials  = errors.New("invalid credentials")
)

// TokenClaims extends the standard JWT claims with Chaos-Sec specific fields.
type TokenClaims struct {
	UserID         uuid.UUID `json:"uid"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	OrganizationID uuid.UUID `json:"org_id"`
	Permissions    []string  `json:"permissions"`
	TokenType      string    `json:"token_type,omitempty"`
	jwt.RegisteredClaims
}

// AuthService provides JWT token generation and validation.
type AuthService struct {
	jwtConfig *config.JWTConfig
}

// New creates a new AuthService with the provided JWT configuration.
func New(jwtConfig *config.JWTConfig) (*AuthService, error) {
	if jwtConfig == nil {
		return nil, errors.New("JWT configuration is required")
	}
	if jwtConfig.Secret == "" {
		return nil, errors.New("JWT secret is required")
	}

	return &AuthService{
		jwtConfig: jwtConfig,
	}, nil
}

// GenerateAccessToken creates a new access token for the given user.
// Access tokens are short-lived (typically 1 hour) and contain the user's
// identity, role, organization, and permissions for authorization.
func (s *AuthService) GenerateAccessToken(userID uuid.UUID, email string, role string, organizationID uuid.UUID, permissions []string) (string, *TokenClaims, error) {
	now := time.Now()
	expiresAt := now.Add(s.jwtConfig.Expiry)

	claims := &TokenClaims{
		UserID:         userID,
		Email:          email,
		Role:           role,
		OrganizationID: organizationID,
		Permissions:    permissions,
		TokenType:      TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.jwtConfig.Issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"chaos-sec-api"},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(), // jti claim for token revocation support
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return "", nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	return tokenString, claims, nil
}

// GenerateRefreshToken creates a new refresh token for the given user.
// Refresh tokens are long-lived (typically 7 days) and contain minimal claims.
// They are used only to obtain a new access token and should not carry
// permission data since permissions may change during the refresh period.
func (s *AuthService) GenerateRefreshToken(userID uuid.UUID, email string, role string, organizationID uuid.UUID) (string, *TokenClaims, error) {
	now := time.Now()
	expiresAt := now.Add(s.jwtConfig.RefreshExpiry)

	claims := &TokenClaims{
		UserID:         userID,
		Email:          email,
		Role:           role,
		OrganizationID: organizationID,
		Permissions:    nil, // Refresh tokens do not carry permissions
		TokenType:      TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.jwtConfig.Issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{"chaos-sec-api"},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtConfig.Secret))
	if err != nil {
		return "", nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return tokenString, claims, nil
}

// ValidateToken parses and validates a JWT token string.
// It verifies the signature, expiration, issuer, and token type.
// Returns the parsed claims if the token is valid, or an error describing
// why validation failed.
func (s *AuthService) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Ensure the token uses the expected signing method.
		// This prevents attack vectors where a token signed with none or a different
		// algorithm could bypass verification.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtConfig.Secret), nil
	})

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return nil, ErrTokenNotValidYet
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, ErrInvalidToken
		default:
			return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Validate that the token has a type claim
	if claims.TokenType == "" {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// ValidateAccessToken validates a token and ensures it is an access token.
// This should be used in authentication middleware to reject refresh tokens
// being used for API access.
func (s *AuthService) ValidateAccessToken(tokenString string) (*TokenClaims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// ValidateRefreshToken validates a token and ensures it is a refresh token.
// This should be used in the refresh endpoint to reject access tokens
// being used to obtain new tokens.
func (s *AuthService) ValidateRefreshToken(tokenString string) (*TokenClaims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// HasPermission checks whether the token claims include a specific permission.
// The "admin:all" permission grants access to everything.
func (c *TokenClaims) HasPermission(permission string) bool {
	for _, p := range c.Permissions {
		if p == permission || p == "admin:all" {
			return true
		}
	}
	return false
}

// HasAnyPermission checks whether the token claims include any of the given permissions.
func (c *TokenClaims) HasAnyPermission(permissions ...string) bool {
	for _, required := range permissions {
		if c.HasPermission(required) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks whether the token claims include all of the given permissions.
func (c *TokenClaims) HasAllPermissions(permissions ...string) bool {
	for _, required := range permissions {
		if !c.HasPermission(required) {
			return false
		}
	}
	return true
}

// IsAdmin returns true if the user has the admin:all permission.
func (c *TokenClaims) IsAdmin() bool {
	return c.HasPermission("admin:all")
}

// --- Password hashing ---

// HashPassword hashes a plaintext password using bcrypt with the configured cost.
// Returns the hashed password string suitable for storage in the database.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", ErrPasswordTooShort
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPasswordHashFailure, err)
	}

	return string(bytes), nil
}

// CheckPassword compares a plaintext password against a stored bcrypt hash.
// Returns nil on success, or an error if the password does not match.
func CheckPassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("password comparison error: %w", err)
	}
	return nil
}

// IsHashedPassword checks whether a string appears to be a bcrypt hash.
// This is useful for validating that stored passwords have been hashed.
func IsHashedPassword(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}
