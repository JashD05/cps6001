package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Base contains common columns shared across all models.
type Base struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:now()"`
}

// Organization stores multi-tenant organization data for isolation.
type Organization struct {
	ID        uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string          `json:"name" gorm:"type:varchar(255);not null;uniqueIndex"`
	Slug      string          `json:"slug" gorm:"type:varchar(100);not null;uniqueIndex"`
	Status    string          `json:"status" gorm:"type:varchar(20);not null;default:'active'"`
	Settings  json.RawMessage `json:"settings" gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time       `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt time.Time       `json:"updated_at" gorm:"not null;default:now()"`
}

// TableName specifies the table name for Organization.
func (Organization) TableName() string { return "organizations" }

// Role defines RBAC roles and permissions.
type Role struct {
	ID          uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string          `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Description string          `json:"description" gorm:"type:text"`
	Permissions json.RawMessage `json:"permissions" gorm:"type:jsonb;not null"`
	IsSystem    bool            `json:"is_system" gorm:"not null;default:false"`
	CreatedAt   time.Time       `json:"created_at" gorm:"not null;default:now()"`

	Users []User `json:"users,omitempty" gorm:"foreignKey:RoleID"`
}

// TableName specifies the table name for Role.
func (Role) TableName() string { return "roles" }

// User stores user account information and authentication credentials.
type User struct {
	ID             uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email          string     `json:"email" gorm:"type:varchar(255);not null;uniqueIndex"`
	PasswordHash   string     `json:"-" gorm:"type:varchar(255);not null;column:password_hash"`
	Name           string     `json:"name" gorm:"type:varchar(255);not null"`
	OrganizationID uuid.UUID  `json:"organization_id" gorm:"type:uuid;not null;index"`
	RoleID         uuid.UUID  `json:"role_id" gorm:"type:uuid;not null;index"`
	IsActive       bool       `json:"is_active" gorm:"not null;default:true;index"`
	LastLoginAt    *time.Time `json:"last_login_at" gorm:"type:timestamptz"`
	CreatedAt      time.Time  `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"not null;default:now()"`

	Organization Organization `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	Role         Role         `json:"role,omitempty" gorm:"foreignKey:RoleID"`
}

// TableName specifies the table name for User.
func (User) TableName() string { return "users" }

// APIKey stores API keys for programmatic access.
type APIKey struct {
	ID          uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;not null;index"`
	Name        string          `json:"name" gorm:"type:varchar(255);not null"`
	KeyHash     string          `json:"-" gorm:"type:varchar(255);not null;uniqueIndex;column:key_hash"`
	KeyPrefix   string          `json:"key_prefix" gorm:"type:varchar(8);not null;column:key_prefix"`
	Permissions json.RawMessage `json:"permissions,omitempty" gorm:"type:jsonb"`
	ExpiresAt   *time.Time      `json:"expires_at" gorm:"type:timestamptz;index"`
	LastUsedAt  *time.Time      `json:"last_used_at" gorm:"type:timestamptz"`
	CreatedAt   time.Time       `json:"created_at" gorm:"not null;default:now()"`
	RevokedAt   *time.Time      `json:"revoked_at" gorm:"type:timestamptz"`

	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for APIKey.
func (APIKey) TableName() string { return "api_keys" }

// KubernetesCluster stores connection details for target Kubernetes clusters.
type KubernetesCluster struct {
	ID                uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID    uuid.UUID  `json:"organization_id" gorm:"type:uuid;not null;index"`
	Name              string     `json:"name" gorm:"type:varchar(255);not null;index"`
	Description       string     `json:"description" gorm:"type:text"`
	APIEndpoint       string     `json:"api_endpoint" gorm:"type:varchar(500);not null;column:api_endpoint"`
	CACertificate     string     `json:"-" gorm:"type:text;not null;column:ca_certificate"`
	ClientCertificate string     `json:"-" gorm:"type:text;not null;column:client_certificate"`
	ClientKey         string     `json:"-" gorm:"type:text;not null;column:client_key"`
	DefaultNamespace  string     `json:"default_namespace" gorm:"type:varchar(255);not null;default:'chaos-sec';column:default_namespace"`
	Status            string     `json:"status" gorm:"type:varchar(20);not null;default:'pending';index"`
	LastConnectedAt   *time.Time `json:"last_connected_at" gorm:"type:timestamptz;column:last_connected_at"`
	KubernetesVersion *string    `json:"kubernetes_version" gorm:"type:varchar(50);column:kubernetes_version"`
	NodeCount         *int       `json:"node_count" gorm:"column:node_count"`
	CreatedAt         time.Time  `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"not null;default:now()"`

	Organization Organization `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
}

// TableName specifies the table name for KubernetesCluster.
func (KubernetesCluster) TableName() string { return "kubernetes_clusters" }

// AttackTemplate defines predefined attack patterns for experiments.
type AttackTemplate struct {
	ID               uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name             string          `json:"name" gorm:"type:varchar(255);not null"`
	Slug             string          `json:"slug" gorm:"type:varchar(255);not null;uniqueIndex"`
	Category         string          `json:"category" gorm:"type:varchar(100);not null;index"`
	Severity         string          `json:"severity" gorm:"type:varchar(20);not null;index"`
	Description      string          `json:"description" gorm:"type:text;not null"`
	MitreAttackID    *string         `json:"mitre_attack_id" gorm:"type:varchar(50);index;column:mitre_attack_id"`
	K8sManifest      json.RawMessage `json:"k8s_manifest" gorm:"type:jsonb;not null;column:k8s_manifest"`
	Parameters       json.RawMessage `json:"parameters" gorm:"type:jsonb;not null"`
	Prerequisites    json.RawMessage `json:"prerequisites" gorm:"type:jsonb;default:'[]'"`
	ExpectedBehavior string          `json:"expected_behavior" gorm:"type:text;not null;column:expected_behavior"`
	Mitigation       string          `json:"mitigation" gorm:"type:text"`
	IsActive         bool            `json:"is_active" gorm:"not null;default:true;index;column:is_active"`
	IsSystem         bool            `json:"is_system" gorm:"not null;default:false;column:is_system"`
	CreatedAt        time.Time       `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt        time.Time       `json:"updated_at" gorm:"not null;default:now()"`
}

// TableName specifies the table name for AttackTemplate.
func (AttackTemplate) TableName() string { return "attack_templates" }

// Experiment defines reusable experiment configurations.
type Experiment struct {
	ID                 uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID     uuid.UUID       `json:"organization_id" gorm:"type:uuid;not null;index"`
	Name               string          `json:"name" gorm:"type:varchar(255);not null"`
	Description        string          `json:"description" gorm:"type:text"`
	Status             string          `json:"status" gorm:"type:varchar(20);not null;default:'draft';index"`
	CreatedBy          uuid.UUID       `json:"created_by" gorm:"type:uuid;not null;index;column:created_by"`
	ScheduleCron       *string         `json:"schedule_cron" gorm:"type:varchar(100);column:schedule_cron"`
	AutoCleanup        bool            `json:"auto_cleanup" gorm:"not null;default:true;column:auto_cleanup"`
	NotificationConfig json.RawMessage `json:"notification_config" gorm:"type:jsonb;default:'{}';column:notification_config"`
	CreatedAt          time.Time       `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt          time.Time       `json:"updated_at" gorm:"not null;default:now()"`

	Organization        Organization         `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	Creator             User                 `json:"creator,omitempty" gorm:"foreignKey:CreatedBy"`
	ExperimentTemplates []ExperimentTemplate `json:"experiment_templates,omitempty" gorm:"foreignKey:ExperimentID"`
	Runs                []ExperimentRun      `json:"runs,omitempty" gorm:"foreignKey:ExperimentID"`
}

// TableName specifies the table name for Experiment.
func (Experiment) TableName() string { return "experiments" }

// ExperimentTemplate links experiments to attack templates with specific configurations.
type ExperimentTemplate struct {
	ID               uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ExperimentID     uuid.UUID       `json:"experiment_id" gorm:"type:uuid;not null;index;column:experiment_id"`
	AttackTemplateID uuid.UUID       `json:"attack_template_id" gorm:"type:uuid;not null;index;column:attack_template_id"`
	OrderIndex       int             `json:"order_index" gorm:"not null;default:0;column:order_index"`
	Configuration    json.RawMessage `json:"configuration" gorm:"type:jsonb;not null"`
	TargetNamespaces []string        `json:"target_namespaces" gorm:"type:text[];column:target_namespaces"`
	TargetLabels     json.RawMessage `json:"target_labels" gorm:"type:jsonb;column:target_labels"`
	DurationSeconds  int             `json:"duration_seconds" gorm:"not null;default:300;column:duration_seconds"`
	CleanupPolicy    string          `json:"cleanup_policy" gorm:"type:varchar(50);not null;default:'immediate';column:cleanup_policy"`
	SIEMValidation   json.RawMessage `json:"siem_validation" gorm:"type:jsonb;column:siem_validation"`
	Enabled          bool            `json:"enabled" gorm:"not null;default:true"`
	CreatedAt        time.Time       `json:"created_at" gorm:"not null;default:now()"`

	Experiment     Experiment     `json:"experiment,omitempty" gorm:"foreignKey:ExperimentID"`
	AttackTemplate AttackTemplate `json:"attack_template,omitempty" gorm:"foreignKey:AttackTemplateID"`
}

// TableName specifies the table name for ExperimentTemplate.
func (ExperimentTemplate) TableName() string { return "experiment_templates" }

// ExperimentRun tracks individual execution instances of experiments.
type ExperimentRun struct {
	ID            uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ExperimentID  uuid.UUID       `json:"experiment_id" gorm:"type:uuid;not null;index;column:experiment_id"`
	ClusterID     uuid.UUID       `json:"cluster_id" gorm:"type:uuid;not null;index;column:cluster_id"`
	RunNumber     int             `json:"run_number" gorm:"not null"`
	Status        string          `json:"status" gorm:"type:varchar(30);not null;default:'pending';index"`
	TriggeredBy   *uuid.UUID      `json:"triggered_by" gorm:"type:uuid;column:triggered_by"`
	TriggerType   string          `json:"trigger_type" gorm:"type:varchar(30);not null;column:trigger_type"`
	StartedAt     *time.Time      `json:"started_at" gorm:"type:timestamptz;index;column:started_at"`
	CompletedAt   *time.Time      `json:"completed_at" gorm:"type:timestamptz;column:completed_at"`
	DurationMs    *int64          `json:"duration_ms" gorm:"column:duration_ms"`
	ResultSummary json.RawMessage `json:"result_summary" gorm:"type:jsonb;column:result_summary"`
	ErrorMessage  *string         `json:"error_message" gorm:"type:text;column:error_message"`
	CleanupStatus string          `json:"cleanup_status" gorm:"type:varchar(30);default:'pending';column:cleanup_status"`
	CreatedAt     time.Time       `json:"created_at" gorm:"not null;default:now();index"`

	Experiment      Experiment        `json:"experiment,omitempty" gorm:"foreignKey:ExperimentID"`
	Cluster         KubernetesCluster `json:"cluster,omitempty" gorm:"foreignKey:ClusterID"`
	Trigger         *User             `json:"trigger,omitempty" gorm:"foreignKey:TriggeredBy"`
	AttackPods      []AttackPod       `json:"attack_pods,omitempty" gorm:"foreignKey:RunID"`
	SIEMValidations []SIEMValidation  `json:"siem_validations,omitempty" gorm:"foreignKey:RunID"`
	TestResults     []TestResult      `json:"test_results,omitempty" gorm:"foreignKey:RunID"`
}

// TableName specifies the table name for ExperimentRun.
func (ExperimentRun) TableName() string { return "experiment_runs" }

// AttackPod tracks individual attacker pods spawned during experiment runs.
type AttackPod struct {
	ID           uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RunID        uuid.UUID  `json:"run_id" gorm:"type:uuid;not null;index;column:run_id"`
	TemplateID   uuid.UUID  `json:"template_id" gorm:"type:uuid;not null;index;column:template_id"`
	PodName      string     `json:"pod_name" gorm:"type:varchar(255);not null;column:pod_name"`
	Namespace    string     `json:"namespace" gorm:"type:varchar(255);not null;index"`
	NodeName     *string    `json:"node_name" gorm:"type:varchar(255);column:node_name"`
	IPAddress    *string    `json:"ip_address" gorm:"type:varchar(45);column:ip_address"`
	Status       string     `json:"status" gorm:"type:varchar(30);not null;default:'pending';index"`
	Phase        *string    `json:"phase" gorm:"type:varchar(30)"`
	StartedAt    *time.Time `json:"started_at" gorm:"type:timestamptz;column:started_at"`
	TerminatedAt *time.Time `json:"terminated_at" gorm:"type:timestamptz;column:terminated_at"`
	ExitCode     *int       `json:"exit_code" gorm:"column:exit_code"`
	LogsSummary  *string    `json:"logs_summary" gorm:"type:text;column:logs_summary"`
	CreatedAt    time.Time  `json:"created_at" gorm:"not null;default:now()"`

	Run      ExperimentRun  `json:"run,omitempty" gorm:"foreignKey:RunID"`
	Template AttackTemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`
}

// TableName specifies the table name for AttackPod.
func (AttackPod) TableName() string { return "attack_pods" }

// SIEMValidation tracks SIEM alert validation for each experiment run.
type SIEMValidation struct {
	ID                    uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RunID                 uuid.UUID       `json:"run_id" gorm:"type:uuid;not null;index;column:run_id"`
	AttackPodID           *uuid.UUID      `json:"attack_pod_id" gorm:"type:uuid;index;column:attack_pod_id"`
	ExpectedAlertType     string          `json:"expected_alert_type" gorm:"type:varchar(255);not null;column:expected_alert_type"`
	ExpectedAlertSeverity *string         `json:"expected_alert_severity" gorm:"type:varchar(20);column:expected_alert_severity"`
	AlertReceived         bool            `json:"alert_received" gorm:"not null;default:false;index;column:alert_received"`
	ReceivedAt            *time.Time      `json:"received_at" gorm:"type:timestamptz;column:received_at"`
	SIEMResponse          json.RawMessage `json:"siem_response" gorm:"type:jsonb;column:siem_response"`
	AlertID               *string         `json:"alert_id" gorm:"type:varchar(255);column:alert_id"`
	Matched               *bool           `json:"matched" gorm:"column:matched"`
	MatchDetails          json.RawMessage `json:"match_details" gorm:"type:jsonb;column:match_details"`
	ValidationStatus      string          `json:"validation_status" gorm:"type:varchar(30);not null;default:'pending';index;column:validation_status"`
	CheckedAt             *time.Time      `json:"checked_at" gorm:"type:timestamptz;column:checked_at"`
	CreatedAt             time.Time       `json:"created_at" gorm:"not null;default:now()"`

	Run       ExperimentRun `json:"run,omitempty" gorm:"foreignKey:RunID"`
	AttackPod *AttackPod    `json:"attack_pod,omitempty" gorm:"foreignKey:AttackPodID"`
}

// TableName specifies the table name for SIEMValidation.
func (SIEMValidation) TableName() string { return "siem_validations" }

// TestResult stores individual test/check results within an experiment run.
type TestResult struct {
	ID           uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RunID        uuid.UUID       `json:"run_id" gorm:"type:uuid;not null;index;column:run_id"`
	AttackPodID  *uuid.UUID      `json:"attack_pod_id" gorm:"type:uuid;index;column:attack_pod_id"`
	CheckName    string          `json:"check_name" gorm:"type:varchar(255);not null;column:check_name"`
	CheckType    string          `json:"check_type" gorm:"type:varchar(100);not null;index;column:check_type"`
	Status       string          `json:"status" gorm:"type:varchar(30);not null;index"`
	Expected     *string         `json:"expected" gorm:"type:text"`
	Actual       *string         `json:"actual" gorm:"type:text"`
	Details      json.RawMessage `json:"details" gorm:"type:jsonb"`
	ErrorMessage *string         `json:"error_message" gorm:"type:text;column:error_message"`
	Timestamp    time.Time       `json:"timestamp" gorm:"not null;default:now();index"`

	Run       ExperimentRun `json:"run,omitempty" gorm:"foreignKey:RunID"`
	AttackPod *AttackPod    `json:"attack_pod,omitempty" gorm:"foreignKey:AttackPodID"`
}

// TableName specifies the table name for TestResult.
func (TestResult) TableName() string { return "test_results" }

// AuditLog is an immutable log of all user actions and system events.
type AuditLog struct {
	ID             uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID       `json:"organization_id" gorm:"type:uuid;not null;index;column:organization_id"`
	UserID         *uuid.UUID      `json:"user_id" gorm:"type:uuid;index;column:user_id"`
	APIKeyID       *uuid.UUID      `json:"api_key_id" gorm:"type:uuid;column:api_key_id"`
	Action         string          `json:"action" gorm:"type:varchar(100);not null;index"`
	ResourceType   string          `json:"resource_type" gorm:"type:varchar(100);not null;index;column:resource_type"`
	ResourceID     *uuid.UUID      `json:"resource_id" gorm:"type:uuid;column:resource_id"`
	ResourceName   *string         `json:"resource_name" gorm:"type:varchar(255);column:resource_name"`
	Details        json.RawMessage `json:"details" gorm:"type:jsonb"`
	IPAddress      *string         `json:"ip_address" gorm:"type:varchar(45);column:ip_address"`
	UserAgent      *string         `json:"user_agent" gorm:"type:varchar(500);column:user_agent"`
	Status         string          `json:"status" gorm:"type:varchar(30);not null;index"`
	Timestamp      time.Time       `json:"timestamp" gorm:"not null;default:now();index"`

	Organization Organization `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	User         *User        `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for AuditLog.
func (AuditLog) TableName() string { return "audit_logs" }

// Notification stores notification channel configurations.
type Notification struct {
	ID             uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID       `json:"organization_id" gorm:"type:uuid;not null;index;column:organization_id"`
	Name           string          `json:"name" gorm:"type:varchar(255);not null"`
	Type           string          `json:"type" gorm:"type:varchar(50);not null;index"`
	Configuration  json.RawMessage `json:"configuration" gorm:"type:jsonb;not null"`
	IsActive       bool            `json:"is_active" gorm:"not null;default:true;index;column:is_active"`
	CreatedAt      time.Time       `json:"created_at" gorm:"not null;default:now()"`

	Organization Organization        `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	Events       []NotificationEvent `json:"events,omitempty" gorm:"foreignKey:NotificationID"`
}

// TableName specifies the table name for Notification.
func (Notification) TableName() string { return "notifications" }

// NotificationEvent tracks notification delivery attempts and results.
type NotificationEvent struct {
	ID             uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	NotificationID uuid.UUID  `json:"notification_id" gorm:"type:uuid;not null;index;column:notification_id"`
	EventType      string     `json:"event_type" gorm:"type:varchar(100);not null;index;column:event_type"`
	RelatedRunID   *uuid.UUID `json:"related_run_id" gorm:"type:uuid;column:related_run_id"`
	Subject        *string    `json:"subject" gorm:"type:varchar(500)"`
	Content        *string    `json:"content" gorm:"type:text"`
	Status         string     `json:"status" gorm:"type:varchar(30);not null;index"`
	SentAt         *time.Time `json:"sent_at" gorm:"type:timestamptz;column:sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at" gorm:"type:timestamptz;column:delivered_at"`
	ErrorMessage   *string    `json:"error_message" gorm:"type:text;column:error_message"`
	RetryCount     int        `json:"retry_count" gorm:"not null;default:0;column:retry_count"`
	CreatedAt      time.Time  `json:"created_at" gorm:"not null;default:now();index"`

	Notification Notification   `json:"notification,omitempty" gorm:"foreignKey:NotificationID"`
	RelatedRun   *ExperimentRun `json:"related_run,omitempty" gorm:"foreignKey:RelatedRunID"`
}

// TableName specifies the table name for NotificationEvent.
func (NotificationEvent) TableName() string { return "notification_events" }

// ============================================================================
// Request/Response DTOs
// ============================================================================

// LoginRequest represents a login request payload.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// RegisterRequest represents a user registration payload.
type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email"`
	Password       string `json:"password" binding:"required,min=8"`
	Name           string `json:"name" binding:"required,min=2,max=255"`
	OrganizationID string `json:"organization_id" binding:"required,uuid"`
	RoleID         string `json:"role_id" binding:"required,uuid"`
}

// TokenResponse represents the JWT token response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until expiry
	TokenType    string `json:"token_type"`
}

// RefreshRequest represents a token refresh request.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// UserResponse is the user data returned in API responses (no password hash).
type UserResponse struct {
	ID             uuid.UUID  `json:"id"`
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	RoleID         uuid.UUID  `json:"role_id"`
	IsActive       bool       `json:"is_active"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	Organization *OrganizationResponse `json:"organization,omitempty"`
	Role         *RoleResponse         `json:"role,omitempty"`
}

// OrganizationResponse is the organization data returned in API responses.
type OrganizationResponse struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Status    string          `json:"status"`
	Settings  json.RawMessage `json:"settings"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// RoleResponse is the role data returned in API responses.
type RoleResponse struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Permissions json.RawMessage `json:"permissions"`
	IsSystem    bool            `json:"is_system"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CreateExperimentRequest represents a create experiment payload.
type CreateExperimentRequest struct {
	Name               string                    `json:"name" binding:"required,min=2,max=255"`
	Description        string                    `json:"description"`
	ScheduleCron       *string                   `json:"schedule_cron"`
	AutoCleanup        *bool                     `json:"auto_cleanup"`
	NotificationConfig json.RawMessage           `json:"notification_config"`
	Templates          []ExperimentTemplateInput `json:"templates" binding:"required,min=1,dive"`
}

// ExperimentTemplateInput represents a template configuration in experiment creation.
type ExperimentTemplateInput struct {
	AttackTemplateID string          `json:"attack_template_id" binding:"required,uuid"`
	OrderIndex       int             `json:"order_index"`
	Configuration    json.RawMessage `json:"configuration" binding:"required"`
	TargetNamespaces []string        `json:"target_namespaces"`
	TargetLabels     json.RawMessage `json:"target_labels"`
	DurationSeconds  int             `json:"duration_seconds"`
	CleanupPolicy    string          `json:"cleanup_policy"`
	SIEMValidation   json.RawMessage `json:"siem_validation"`
	Enabled          *bool           `json:"enabled"`
}

// ExecuteExperimentRequest represents an experiment execution request.
type ExecuteExperimentRequest struct {
	ClusterID string `json:"cluster_id"`
}

// ListExperimentsQuery represents query parameters for listing experiments.
type ListExperimentsQuery struct {
	Page      int    `form:"page,default=1" binding:"min=1"`
	PageSize  int    `form:"page_size,default=20" binding:"min=1,max=100"`
	Status    string `form:"status"`
	SortBy    string `form:"sort_by,default=created_at"`
	SortOrder string `form:"sort_order,default=desc"`
	Search    string `form:"search"`
	ClusterID string `form:"cluster_id"`
}

// PaginatedResponse is a generic paginated response wrapper.
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// HealthCheckResponse represents the health check endpoint response.
type HealthCheckResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// APIResponse represents a standard success response wrapper.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Message string      `json:"message,omitempty"`
}

// DashboardSummary represents aggregated dashboard data.
// DashboardSummary is the comprehensive dashboard summary response
// matching the frontend's DashboardSummary type with camelCase JSON tags.
type DashboardSummary struct {
	SecurityPostureScore     float64                  `json:"securityPostureScore"`
	PostureTrend             PostureTrendData         `json:"postureTrend"`
	ExperimentSummary        ExperimentSummaryData    `json:"experimentSummary"`
	RecentExperiments        []RecentExperimentItem   `json:"recentExperiments"`
	ClusterHealth            []ClusterHealthItem      `json:"clusterHealth"`
	ThreatCoverage           ThreatCoverageData       `json:"threatCoverage"`
	ThreatCoverageByCategory []ThreatCoverageCategory `json:"threatCoverageByCategory"`
	ExperimentTrend          []ActivityTimelinePoint  `json:"experimentTrend"`
	TopAttackTypes           []AttackTypePoint        `json:"topAttackTypes"`
	ValidationSuccessRate    []TrendDataPoint         `json:"validationSuccessRate"`
}

// PostureTrendData represents the trend direction and magnitude for security posture.
type PostureTrendData struct {
	Direction  string  `json:"direction"`
	Percentage float64 `json:"percentage"`
	Period     string  `json:"period"`
}

// ExperimentSummaryData holds experiment count breakdowns by status.
type ExperimentSummaryData struct {
	Total     int64 `json:"total"`
	Running   int64 `json:"running"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Pending   int64 `json:"pending"`
}

// RecentExperimentItem is a lightweight experiment summary for the dashboard.
type RecentExperimentItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Status       string     `json:"status"`
	TemplateName string     `json:"templateName"`
	ClusterName  string     `json:"clusterName"`
	CreatedBy    string     `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
	StartedAt    *time.Time `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
}

// ClusterHealthItem represents health metrics for a single cluster.
type ClusterHealthItem struct {
	ClusterID   string  `json:"clusterId"`
	Status      string  `json:"status"`
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
	PodCount    int64   `json:"podCount"`
	NodeCount   int64   `json:"nodeCount"`
	ErrorRate   float64 `json:"errorRate"`
	LastChecked string  `json:"lastChecked"`
}

// ThreatCoverageData represents aggregate threat coverage metrics.
type ThreatCoverageData struct {
	TotalControls int64   `json:"totalControls"`
	Validated     int64   `json:"validated"`
	Passed        int64   `json:"passed"`
	Failed        int64   `json:"failed"`
	Untested      int64   `json:"untested"`
	Coverage      float64 `json:"coverage"`
}

// ThreatCoverageCategory represents per-category threat coverage for bar charts.
type ThreatCoverageCategory struct {
	Name      string `json:"name"`
	Validated int64  `json:"validated"`
	Untested  int64  `json:"untested"`
}

// ActivityTimelinePoint represents a single data point in the activity timeline.
type ActivityTimelinePoint struct {
	Date   string `json:"date"`
	Total  int64  `json:"total"`
	Passed int64  `json:"passed"`
	Failed int64  `json:"failed"`
}

// TrendDataPoint represents a single point in a time-series trend.
type TrendDataPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
	Label     string  `json:"label,omitempty"`
}

// AttackTypePoint represents a named value in the top attack types chart.
type AttackTypePoint struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
	Color string `json:"color,omitempty"`
}

// SecurityPostureResponse is the response for the /dashboard/security-posture endpoint.
type SecurityPostureResponse struct {
	Score   float64                       `json:"score"`
	Trend   float64                       `json:"trend"`
	History []SecurityPostureHistoryPoint `json:"history"`
}

// SecurityPostureHistoryPoint represents a monthly security posture score.
type SecurityPostureHistoryPoint struct {
	Date  string  `json:"date"`
	Score float64 `json:"score"`
}

// DashboardMetricsResponse is the response for the /dashboard/metrics endpoint.
type DashboardMetricsResponse struct {
	ExperimentsPerDay float64 `json:"experimentsPerDay"`
	AvgDuration       float64 `json:"avgDuration"`
	SuccessRate       float64 `json:"successRate"`
	ActiveUsers       int64   `json:"activeUsers"`
}

// ExperimentRunSummary is kept for backward compatibility; it was previously
// used in DashboardSummary and may still be referenced elsewhere.
type ExperimentRunSummary struct {
	ID           uuid.UUID  `json:"id"`
	ExperimentID uuid.UUID  `json:"experiment_id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	DurationMs   *int64     `json:"duration_ms"`
}

// Report represents a stored experiment report.
type Report struct {
	ID             uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID      `json:"organization_id" gorm:"type:uuid;not null;index"`
	Title          string         `json:"title" gorm:"type:varchar(255);not null"`
	Type           string         `json:"type" gorm:"type:varchar(50);not null;default:'experiment'"`
	Format         string         `json:"format" gorm:"type:varchar(20);not null;default:'pdf'"`
	Description    string         `json:"description" gorm:"type:text"`
	ExperimentIDs  pq.StringArray `json:"experiment_ids" gorm:"type:text[]"`
	DateRangeFrom  *time.Time     `json:"date_range_from" gorm:"column:date_range_from"`
	DateRangeTo    *time.Time     `json:"date_range_to" gorm:"column:date_range_to"`
	Status         string         `json:"status" gorm:"type:varchar(20);not null;default:'pending'"`
	ErrorMessage   *string        `json:"error_message" gorm:"type:text"`
	DownloadURL    *string        `json:"download_url" gorm:"type:varchar(500)"`
	FileSize       *int64         `json:"file_size" gorm:"type:bigint"`
	Content        []byte         `json:"-" gorm:"type:bytea"`
	GeneratedBy    uuid.UUID      `json:"generated_by" gorm:"type:uuid;not null"`
	CreatedAt      time.Time      `json:"created_at" gorm:"not null;default:now()"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"not null;default:now()"`
}

// TableName specifies the table name for Report.
func (Report) TableName() string { return "reports" }

// ReportResponse represents an experiment report response with metadata.
type ReportResponse struct {
	Report     *Report           `json:"report,omitempty"`
	Experiment Experiment        `json:"experiment"`
	Runs       []ExperimentRun   `json:"runs"`
	Summary    *RunResultSummary `json:"summary,omitempty"`
}

// RunResultSummary represents aggregated result summary from an experiment run.
type RunResultSummary struct {
	TotalPodsSpawned   int       `json:"total_pods_spawned"`
	SuccessfulAttacks  int       `json:"successful_attacks"`
	BlockedAttacks     int       `json:"blocked_attacks"`
	SIEMAlertsExpected int       `json:"siem_alerts_expected"`
	SIEMAlertsReceived int       `json:"siem_alerts_received"`
	DetectionRate      float64   `json:"detection_rate"`
	OverallStatus      string    `json:"overall_status"`
	Findings           []Finding `json:"findings,omitempty"`
}

// Finding represents a single finding in an experiment report.
type Finding struct {
	Severity       string `json:"severity"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
}
