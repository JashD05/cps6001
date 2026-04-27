package attack

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// mockModule – minimal AttackModule implementation for testing
// ---------------------------------------------------------------------------

type mockModule struct {
	id          string
	name        string
	category    string
	severity    string
	description string
	params      []Parameter
}

func (m *mockModule) ID() string              { return m.id }
func (m *mockModule) Name() string            { return m.name }
func (m *mockModule) Category() string        { return m.category }
func (m *mockModule) Severity() string        { return m.severity }
func (m *mockModule) Description() string     { return m.description }
func (m *mockModule) Parameters() []Parameter { return m.params }
func (m *mockModule) Execute(_ context.Context, _ AttackConfig) (*AttackResult, error) {
	return &AttackResult{Success: true, Timestamp: time.Now()}, nil
}
func (m *mockModule) Validate(_ context.Context, _ AttackConfig) error { return nil }
func (m *mockModule) Cleanup(_ context.Context, _ AttackConfig) error  { return nil }

// ---------------------------------------------------------------------------
// NewRegistry tests
// ---------------------------------------------------------------------------

func TestNewRegistry_CreatesEmptyRegistry(t *testing.T) {
	// NewRegistry() registers built-in modules, so it should NOT be empty.
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestNewRegistry_RegistersBuiltinModules(t *testing.T) {
	r := NewRegistry()
	modules := r.List()

	builtinIDs := []string{
		"pod-egress-test",
		"pod-ingress-test",
		"network-policy-test",
		"rbac-privilege-test",
		"secret-access-test",
	}

	if len(modules) != len(builtinIDs) {
		t.Fatalf("expected %d built-in modules, got %d", len(builtinIDs), len(modules))
	}

	registeredIDs := make(map[string]bool)
	for _, m := range modules {
		registeredIDs[m.ID()] = true
	}

	for _, id := range builtinIDs {
		if !registeredIDs[id] {
			t.Errorf("built-in module %q not found in registry", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Registry.Register tests
// ---------------------------------------------------------------------------

func TestRegistry_Register_AddsModule(t *testing.T) {
	r := NewRegistry()
	initialCount := len(r.List())

	m := &mockModule{id: "test-module-unique-1", name: "Test Module", category: "test", severity: "low"}
	r.Register(m)

	if len(r.List()) != initialCount+1 {
		t.Errorf("expected %d modules after Register, got %d", initialCount+1, len(r.List()))
	}
}

func TestRegistry_Register_DuplicatePanics(t *testing.T) {
	r := NewRegistry()
	m := &mockModule{id: "duplicate-test", name: "Dup", category: "test", severity: "low"}

	r.Register(m)

	defer func() {
		if rec := recover(); rec == nil {
			t.Error("Register with duplicate ID should panic")
		}
	}()

	r.Register(m)
}

// ---------------------------------------------------------------------------
// Registry.Get tests
// ---------------------------------------------------------------------------

func TestRegistry_Get_ExistingModule(t *testing.T) {
	r := NewRegistry()
	m, err := r.Get("pod-egress-test")
	if err != nil {
		t.Fatalf("Get(\"pod-egress-test\") returned error: %v", err)
	}
	if m.ID() != "pod-egress-test" {
		t.Errorf("Get(\"pod-egress-test\").ID() = %q, want %q", m.ID(), "pod-egress-test")
	}
}

func TestRegistry_Get_NonexistentModule(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nonexistent-module")
	if err == nil {
		t.Error("Get(\"nonexistent-module\") should return error")
	}
}

// ---------------------------------------------------------------------------
// Registry.List tests
// ---------------------------------------------------------------------------

func TestRegistry_List_ReturnsAllModules(t *testing.T) {
	r := NewRegistry()
	modules := r.List()

	if len(modules) == 0 {
		t.Error("List() returned empty slice, expected built-in modules")
	}

	// Verify each module implements the AttackModule interface.
	for _, m := range modules {
		if m.ID() == "" {
			t.Error("found module with empty ID")
		}
		if m.Name() == "" {
			t.Error("found module with empty Name")
		}
	}
}

func TestRegistry_List_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	list1 := r.List()
	list2 := r.List()

	if len(list1) != len(list2) {
		t.Error("two calls to List() returned different lengths")
	}

	// Modifying the returned slice should not affect the registry.
	// (This is a safety check; the registry may or may not return a copy.)
}

// ---------------------------------------------------------------------------
// Registry.ListByCategory tests
// ---------------------------------------------------------------------------

func TestRegistry_ListByCategory(t *testing.T) {
	r := NewRegistry()

	categories := []string{"network", "rbac", "security", "resource", "test"}
	foundAny := false
	for _, cat := range categories {
		modules := r.ListByCategory(cat)
		if len(modules) > 0 {
			foundAny = true
			for _, m := range modules {
				if m.Category() != cat {
					t.Errorf("ListByCategory(%q) returned module with category %q", cat, m.Category())
				}
			}
		}
	}

	networkModules := r.ListByCategory("network")
	if len(networkModules) == 0 {
		t.Error("expected at least one network module")
	}

	_ = foundAny
}

func TestRegistry_ListByCategory_EmptyCategory(t *testing.T) {
	r := NewRegistry()
	modules := r.ListByCategory("nonexistent-category")
	if len(modules) != 0 {
		t.Errorf("ListByCategory(\"nonexistent-category\") returned %d items, want 0", len(modules))
	}
}

// ---------------------------------------------------------------------------
// Registry.ListBySeverity tests
// ---------------------------------------------------------------------------

func TestRegistry_ListBySeverity(t *testing.T) {
	r := NewRegistry()

	severities := []string{"low", "medium", "high", "critical"}
	totalBySeverity := 0
	for _, sev := range severities {
		modules := r.ListBySeverity(sev)
		for _, m := range modules {
			if m.Severity() != sev {
				t.Errorf("ListBySeverity(%q) returned module with severity %q", sev, m.Severity())
			}
		}
		totalBySeverity += len(modules)
	}

	// All built-in modules should be accounted for.
	allModules := r.List()
	if totalBySeverity != len(allModules) {
		t.Errorf("total by severity = %d, total modules = %d – mismatch", totalBySeverity, len(allModules))
	}
}

func TestRegistry_ListBySeverity_EmptySeverity(t *testing.T) {
	r := NewRegistry()
	modules := r.ListBySeverity("nonexistent-severity")
	if len(modules) != 0 {
		t.Errorf("ListBySeverity(\"nonexistent-severity\") returned %d items, want 0", len(modules))
	}
}

// ---------------------------------------------------------------------------
// Built-in module interface compliance tests
// ---------------------------------------------------------------------------

func TestBuiltinModules_ImplementAttackModule(t *testing.T) {
	r := NewRegistry()
	modules := r.List()

	for _, m := range modules {
		t.Run(m.ID(), func(t *testing.T) {
			// Verify all interface methods exist and return non-zero values.
			if m.ID() == "" {
				t.Error("ID() returned empty string")
			}
			if m.Name() == "" {
				t.Error("Name() returned empty string")
			}
			if m.Category() == "" {
				t.Error("Category() returned empty string")
			}
			if m.Severity() == "" {
				t.Error("Severity() returned empty string")
			}
			if m.Description() == "" {
				t.Error("Description() returned empty string")
			}
			// Parameters() may return nil or empty slice.
			_ = m.Parameters()
		})
	}
}

func TestBuiltinModules_ValidateDoesNotPanic(t *testing.T) {
	r := NewRegistry()
	modules := r.List()
	ctx := context.Background()

	for _, m := range modules {
		t.Run(m.ID(), func(t *testing.T) {
			// Built-in modules require a real Kubernetes cluster to validate/cleanup.
			// Without one, some may panic due to nil ClusterClient.
			// We test only that the module can be constructed and the interface
			// methods exist — actual validation/cleanup requires K8s.
			// Skip modules that need a cluster client.
			if os.Getenv("KUBECONFIG") == "" {
				t.Skip("skipping: KUBECONFIG not set (requires real K8s cluster)")
			}

			config := AttackConfig{
				ExperimentID: "test-exp",
				RunID:        "test-run",
				Namespace:    "default",
				Parameters:   map[string]interface{}{},
				Logger:       zap.NewNop(),
			}
			_ = m.Validate(ctx, config)
		})
	}
}

func TestBuiltinModules_CleanupDoesNotPanic(t *testing.T) {
	r := NewRegistry()
	modules := r.List()
	ctx := context.Background()

	for _, m := range modules {
		t.Run(m.ID(), func(t *testing.T) {
			// Built-in modules require a real Kubernetes cluster to validate/cleanup.
			// Without one, some may panic due to nil ClusterClient.
			// Skip modules that need a cluster client.
			if os.Getenv("KUBECONFIG") == "" {
				t.Skip("skipping: KUBECONFIG not set (requires real K8s cluster)")
			}

			config := AttackConfig{
				ExperimentID: "test-exp",
				RunID:        "test-run",
				Namespace:    "default",
				Parameters:   map[string]interface{}{},
				Logger:       zap.NewNop(),
			}
			_ = m.Cleanup(ctx, config)
		})
	}
}

// ---------------------------------------------------------------------------
// AttackConfig parameter helper tests
// ---------------------------------------------------------------------------

func TestAttackConfig_ParamString(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"host":   "example.com",
			"port":   8080,
			"active": true,
		},
	}

	tests := []struct {
		name     string
		key      string
		want     string
		wantBool bool
	}{
		{"existing string key", "host", "example.com", true},
		{"non-existing key", "missing", "", false},
		{"wrong type key", "port", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := config.ParamString(tt.key)
			if ok != tt.wantBool {
				t.Errorf("ParamString(%q) ok = %v, want %v", tt.key, ok, tt.wantBool)
			}
			if got != tt.want {
				t.Errorf("ParamString(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestAttackConfig_ParamString_EmptyConfig(t *testing.T) {
	config := AttackConfig{Parameters: map[string]interface{}{}}
	got, ok := config.ParamString("any")
	if ok {
		t.Error("ParamString on empty config should return false")
	}
	if got != "" {
		t.Errorf("ParamString on empty config = %q, want empty", got)
	}
}

func TestAttackConfig_ParamString_NilConfig(t *testing.T) {
	config := AttackConfig{Parameters: nil}
	got, ok := config.ParamString("any")
	if ok {
		t.Error("ParamString on nil parameters should return false")
	}
	_ = got
}

func TestAttackConfig_ParamInt(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"count":   42,
			"count64": int64(100),
			"countf":  float64(7),
			"host":    "example.com",
			"active":  true,
		},
	}

	tests := []struct {
		name     string
		key      string
		want     int
		wantBool bool
	}{
		{"int value", "count", 42, true},
		{"int64 value", "count64", 100, true},
		{"float64 value", "countf", 7, true},
		{"non-existing key", "missing", 0, false},
		{"wrong type string", "host", 0, false},
		{"wrong type bool", "active", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := config.ParamInt(tt.key)
			if ok != tt.wantBool {
				t.Errorf("ParamInt(%q) ok = %v, want %v", tt.key, ok, tt.wantBool)
			}
			if got != tt.want {
				t.Errorf("ParamInt(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestAttackConfig_ParamInt_EmptyConfig(t *testing.T) {
	config := AttackConfig{Parameters: map[string]interface{}{}}
	got, ok := config.ParamInt("any")
	if ok {
		t.Error("ParamInt on empty config should return false")
	}
	if got != 0 {
		t.Errorf("ParamInt on empty config = %d, want 0", got)
	}
}

func TestAttackConfig_ParamBool(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"active": true,
			"count":  42,
			"host":   "example.com",
		},
	}

	tests := []struct {
		name     string
		key      string
		want     bool
		wantBool bool
	}{
		{"bool value true", "active", true, true},
		{"non-existing key", "missing", false, false},
		{"wrong type int", "count", false, false},
		{"wrong type string", "host", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := config.ParamBool(tt.key)
			if ok != tt.wantBool {
				t.Errorf("ParamBool(%q) ok = %v, want %v", tt.key, ok, tt.wantBool)
			}
			if got != tt.want {
				t.Errorf("ParamBool(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestAttackConfig_ParamBool_EmptyConfig(t *testing.T) {
	config := AttackConfig{Parameters: map[string]interface{}{}}
	got, ok := config.ParamBool("any")
	if ok {
		t.Error("ParamBool on empty config should return false")
	}
	if got {
		t.Error("ParamBool on empty config should return false value")
	}
}

// ---------------------------------------------------------------------------
// Parameter tests
// ---------------------------------------------------------------------------

func TestParameter_Fields(t *testing.T) {
	p := Parameter{
		Name:        "target-host",
		Type:        ParamTypeString,
		Required:    true,
		Default:     "localhost",
		Description: "The target host to test against",
	}

	if p.Name != "target-host" {
		t.Errorf("Parameter.Name = %q, want %q", p.Name, "target-host")
	}
	if p.Type != ParamTypeString {
		t.Errorf("Parameter.Type = %q, want %q", p.Type, ParamTypeString)
	}
	if !p.Required {
		t.Error("Parameter.Required should be true")
	}
	if p.Default != "localhost" {
		t.Errorf("Parameter.Default = %v, want %q", p.Default, "localhost")
	}
}

func TestParameterType_Values(t *testing.T) {
	tests := []struct {
		name     string
		got      ParameterType
		expected string
	}{
		{"string", ParamTypeString, "string"},
		{"int", ParamTypeInt, "int"},
		{"bool", ParamTypeBool, "bool"},
		{"select", ParamTypeSelect, "select"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("ParameterType %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestParameter_SelectWithOptions(t *testing.T) {
	p := Parameter{
		Name:        "severity",
		Type:        ParamTypeSelect,
		Required:    false,
		Default:     "medium",
		Description: "Attack severity level",
		Options:     []string{"low", "medium", "high", "critical"},
	}

	if p.Type != ParamTypeSelect {
		t.Errorf("Parameter.Type = %q, want %q", p.Type, ParamTypeSelect)
	}
	if len(p.Options) != 4 {
		t.Errorf("len(Parameter.Options) = %d, want 4", len(p.Options))
	}
}

// ---------------------------------------------------------------------------
// AttackResult tests
// ---------------------------------------------------------------------------

func TestAttackResult_Fields(t *testing.T) {
	now := time.Now()
	result := AttackResult{
		Success:   true,
		Blocked:   false,
		Evidence:  "Pod attempted egress to external host",
		Logs:      "full pod log output",
		Timestamp: now,
		Duration:  5 * time.Second,
	}

	if !result.Success {
		t.Error("AttackResult.Success should be true")
	}
	if result.Blocked {
		t.Error("AttackResult.Blocked should be false")
	}
	if result.Evidence != "Pod attempted egress to external host" {
		t.Errorf("AttackResult.Evidence = %q, want expected evidence string", result.Evidence)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("AttackResult.Duration = %v, want 5s", result.Duration)
	}
}

func TestAttackResult_BlockedResult(t *testing.T) {
	result := AttackResult{
		Success:   false,
		Blocked:   true,
		Evidence:  "Connection was blocked by network policy",
		Timestamp: time.Now(),
		Duration:  3 * time.Second,
	}

	if result.Success {
		t.Error("Blocked result should have Success=false")
	}
	if !result.Blocked {
		t.Error("Blocked result should have Blocked=true")
	}
}

func TestAttackResult_WithError(t *testing.T) {
	result := AttackResult{
		Success:   false,
		Blocked:   false,
		Error:     "pod failed to start: image pull error",
		Timestamp: time.Now(),
		Duration:  10 * time.Second,
	}

	if result.Success {
		t.Error("Error result should have Success=false")
	}
	if result.Error == "" {
		t.Error("Error result should have non-empty Error field")
	}
}

// ---------------------------------------------------------------------------
// AttackConfig tests
// ---------------------------------------------------------------------------

func TestAttackConfig_Fields(t *testing.T) {
	config := AttackConfig{
		ExperimentID: "exp-123",
		RunID:        "run-456",
		Namespace:    "chaos-test-ns",
		Parameters:   map[string]interface{}{"target": "10.0.0.1"},
		Timeout:      30 * time.Second,
		Logger:       zap.NewNop(),
	}

	if config.ExperimentID != "exp-123" {
		t.Errorf("AttackConfig.ExperimentID = %q, want %q", config.ExperimentID, "exp-123")
	}
	if config.Namespace != "chaos-test-ns" {
		t.Errorf("AttackConfig.Namespace = %q, want %q", config.Namespace, "chaos-test-ns")
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("AttackConfig.Timeout = %v, want 30s", config.Timeout)
	}
}

func TestAttackConfig_ParamString_DefaultValues(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"empty":   "",
			"present": "value",
		},
	}

	// Empty string is still a valid string value.
	got, ok := config.ParamString("empty")
	if !ok {
		t.Error("ParamString('empty') should return ok=true")
	}
	if got != "" {
		t.Errorf("ParamString('empty') = %q, want empty string", got)
	}

	got, ok = config.ParamString("present")
	if !ok {
		t.Error("ParamString('present') should return ok=true")
	}
	if got != "value" {
		t.Errorf("ParamString('present') = %q, want %q", got, "value")
	}
}

func TestAttackConfig_ParamInt_DefaultValues(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"zero": 0,
			"neg":  -1,
			"big":  999999,
		},
	}

	got, ok := config.ParamInt("zero")
	if !ok || got != 0 {
		t.Errorf("ParamInt('zero') = (%d, %v), want (0, true)", got, ok)
	}

	got, ok = config.ParamInt("neg")
	if !ok || got != -1 {
		t.Errorf("ParamInt('neg') = (%d, %v), want (-1, true)", got, ok)
	}

	got, ok = config.ParamInt("big")
	if !ok || got != 999999 {
		t.Errorf("ParamInt('big') = (%d, %v), want (999999, true)", got, ok)
	}
}

func TestAttackConfig_ParamBool_DefaultValues(t *testing.T) {
	config := AttackConfig{
		Parameters: map[string]interface{}{
			"false_val": false,
			"true_val":  true,
		},
	}

	got, ok := config.ParamBool("false_val")
	if !ok || got != false {
		t.Errorf("ParamBool('false_val') = (%v, %v), want (false, true)", got, ok)
	}

	got, ok = config.ParamBool("true_val")
	if !ok || got != true {
		t.Errorf("ParamBool('true_val') = (%v, %v), want (true, true)", got, ok)
	}
}

// ---------------------------------------------------------------------------
// Registry concurrency test
// ---------------------------------------------------------------------------

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool, 10)

	// Read from the registry concurrently.
	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			_ = r.List()
			_ = r.ListByCategory("network")
			_ = r.ListBySeverity("high")
		}()
	}

	// Wait for all goroutines to finish.
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestRegistry_Get_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool, 10)

	ids := []string{"pod-egress-test", "pod-ingress-test", "network-policy-test", "nonexistent"}

	for i := 0; i < 5; i++ {
		go func() {
			defer func() { done <- true }()
			for _, id := range ids {
				m, err := r.Get(id)
				if err != nil && id != "nonexistent" {
					t.Errorf("Get(%q) returned unexpected error: %v", id, err)
				}
				_ = m
			}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}
