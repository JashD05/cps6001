package attack

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Parameter types
// ---------------------------------------------------------------------------

// ParameterType defines the allowed types for attack module parameters.
type ParameterType string

const (
	ParamTypeString ParameterType = "string"
	ParamTypeInt    ParameterType = "int"
	ParamTypeBool   ParameterType = "bool"
	ParamTypeSelect ParameterType = "select"
)

// Parameter describes a single configurable parameter for an attack module.
type Parameter struct {
	Name        string        `json:"name"`
	Type        ParameterType `json:"type"`
	Required    bool          `json:"required"`
	Default     interface{}   `json:"default,omitempty"`
	Description string        `json:"description"`
	Options     []string      `json:"options,omitempty"` // populated when Type == ParamTypeSelect
}

// ---------------------------------------------------------------------------
// ClusterClient interface
// ---------------------------------------------------------------------------

// ClusterClient is the interface through which attack modules interact with a
// Kubernetes cluster. The engine / kubernetes package will provide a concrete
// implementation so that attack modules never import kubernetes client-go
// directly.
type ClusterClient interface {
	// CreatePod creates a pod in the given namespace from the supplied YAML manifest.
	CreatePod(ctx context.Context, namespace string, manifest []byte) (podName string, err error)
	// WaitForPodReady blocks until the named pod is running or the context is cancelled.
	WaitForPodReady(ctx context.Context, namespace, podName string, timeout time.Duration) error
	// ExecInPod runs a command inside a container and returns its combined stdout/stderr output.
	ExecInPod(ctx context.Context, namespace, podName, container string, command []string) (stdout string, err error)
	// GetPodLogs retrieves the log output for the specified pod.
	GetPodLogs(ctx context.Context, namespace, podName string, opts *PodLogOptions) (string, error)
	// DeletePod deletes a pod, optionally waiting for it to be fully removed.
	DeletePod(ctx context.Context, namespace, podName string) error
	// ListPods returns the names of pods matching the given label selector in the namespace.
	ListPods(ctx context.Context, namespace, labelSelector string) ([]string, error)
	// CreateService creates a service from the supplied YAML manifest.
	CreateService(ctx context.Context, namespace string, manifest []byte) (serviceName string, err error)
	// DeleteService deletes a named service.
	DeleteService(ctx context.Context, namespace, serviceName string) error
	// GetNetworkPolicy reads the raw network policy manifest.
	GetNetworkPolicy(ctx context.Context, namespace, name string) ([]byte, error)
	// ListNetworkPolicies returns the names of all network policies in the namespace.
	ListNetworkPolicies(ctx context.Context, namespace string) ([]string, error)
	// ApplyManifest applies an arbitrary Kubernetes YAML manifest (create or update).
	ApplyManifest(ctx context.Context, namespace string, manifest []byte) error
	// DeleteManifest deletes a resource previously applied by manifest.
	DeleteManifest(ctx context.Context, namespace string, manifest []byte) error
}

// PodLogOptions mirrors k8s pod log options without importing client-go.
type PodLogOptions struct {
	Container    string
	Follow       bool
	Previous     bool
	SinceSeconds *int64
	TailLines    *int64
	Timestamps   bool
}

// ---------------------------------------------------------------------------
// AttackConfig – input to Execute / Validate / Cleanup
// ---------------------------------------------------------------------------

// AttackConfig holds all configuration needed to run an attack module.
type AttackConfig struct {
	ExperimentID  string                 `json:"experiment_id"`
	RunID         string                 `json:"run_id"`
	ClusterClient ClusterClient          `json:"-"`
	Namespace     string                 `json:"namespace"`
	Parameters    map[string]interface{} `json:"parameters"`
	Timeout       time.Duration          `json:"timeout"`
	Logger        *zap.Logger            `json:"-"`
}

// ParamString retrieves a string parameter from the config.
func (c AttackConfig) ParamString(key string) (string, bool) {
	v, ok := c.Parameters[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ParamInt retrieves an int parameter from the config.
func (c AttackConfig) ParamInt(key string) (int, bool) {
	v, ok := c.Parameters[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// ParamBool retrieves a bool parameter from the config.
func (c AttackConfig) ParamBool(key string) (bool, bool) {
	v, ok := c.Parameters[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// ---------------------------------------------------------------------------
// AttackResult – output of an attack execution
// ---------------------------------------------------------------------------

// AttackResult captures the outcome of a single attack module execution.
type AttackResult struct {
	Success   bool          `json:"success"`
	Blocked   bool          `json:"blocked"`  // true when the security control worked
	Evidence  string        `json:"evidence"` // raw evidence / logs from the attack
	Logs      string        `json:"logs"`     // full pod log output
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
}

// ---------------------------------------------------------------------------
// AttackModule interface
// ---------------------------------------------------------------------------

// AttackModule is the interface that every attack module must implement.
type AttackModule interface {
	// ID returns the unique module identifier (e.g. "pod-egress-test").
	ID() string
	// Name returns the human-readable display name.
	Name() string
	// Category returns the module category: network, rbac, security, resource.
	Category() string
	// Severity returns the module severity: low, medium, high, critical.
	Severity() string
	// Description returns a human-readable description of what the module does.
	Description() string
	// Parameters returns the list of configurable parameters for this module.
	Parameters() []Parameter
	// Execute runs the attack and returns the result.
	Execute(ctx context.Context, config AttackConfig) (*AttackResult, error)
	// Validate checks the configuration is valid before execution.
	Validate(ctx context.Context, config AttackConfig) error
	// Cleanup removes any resources created by the attack.
	Cleanup(ctx context.Context, config AttackConfig) error
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// Registry stores and retrieves attack modules.
type Registry struct {
	mu      sync.RWMutex
	modules map[string]AttackModule
	ordered []string // preserves insertion order for List()
}

// NewRegistry creates a new Registry and registers all built-in attack modules.
func NewRegistry() *Registry {
	r := &Registry{
		modules: make(map[string]AttackModule),
	}

	// Register all built-in modules.
	r.Register(NewPodEgressTest())
	r.Register(NewPodIngressTest())
	r.Register(NewNetworkPolicyTest())
	r.Register(NewRBACPrivilegeTest())
	r.Register(NewSecretAccessTest())

	return r
}

// Register adds a module to the registry. It panics if a module with the same
// ID is already registered, as this indicates a programming error.
func (r *Registry) Register(module AttackModule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := module.ID()
	if _, exists := r.modules[id]; exists {
		panic(fmt.Sprintf("attack module %q already registered", id))
	}
	r.modules[id] = module
	r.ordered = append(r.ordered, id)
}

// Get retrieves a module by its unique ID.
func (r *Registry) Get(id string) (AttackModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.modules[id]
	if !ok {
		return nil, fmt.Errorf("attack module %q not found", id)
	}
	return m, nil
}

// List returns all registered modules in registration order.
func (r *Registry) List() []AttackModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]AttackModule, 0, len(r.ordered))
	for _, id := range r.ordered {
		result = append(result, r.modules[id])
	}
	return result
}

// ListByCategory returns all modules matching the given category.
func (r *Registry) ListByCategory(category string) []AttackModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []AttackModule
	for _, id := range r.ordered {
		m := r.modules[id]
		if m.Category() == category {
			result = append(result, m)
		}
	}
	return result
}

// ListBySeverity returns all modules matching the given severity.
func (r *Registry) ListBySeverity(severity string) []AttackModule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []AttackModule
	for _, id := range r.ordered {
		m := r.modules[id]
		if m.Severity() == severity {
			result = append(result, m)
		}
	}
	return result
}
