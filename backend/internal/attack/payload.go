package attack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// PayloadGenerator – generates Kubernetes manifests and commands from modules
// ---------------------------------------------------------------------------

// PayloadGenerator renders Kubernetes manifests and attack commands from attack
// module templates and runtime configuration. It handles parameter validation,
// default application, and Go template execution.
type PayloadGenerator struct {
	// templates stores raw manifest templates keyed by module ID.
	// Built-in templates are registered at construction time; callers may
	// register custom templates for user-defined modules.
	templates map[string]string
	logger    *zap.Logger
}

// NewPayloadGenerator creates a PayloadGenerator pre-loaded with templates
// for all built-in attack modules.
func NewPayloadGenerator(logger *zap.Logger) *PayloadGenerator {
	pg := &PayloadGenerator{
		templates: make(map[string]string),
		logger:    logger.Named("payload_generator"),
	}

	// Register built-in module templates.
	pg.RegisterTemplate("pod-egress-test", egressManifestTmpl)
	pg.RegisterTemplate("pod-ingress-test", ingressTargetPodTmpl)
	pg.RegisterTemplate("network-policy-test", netpolTestPodTmpl)
	pg.RegisterTemplate("rbac-privilege-test", rbacAttackerPodTmpl)
	pg.RegisterTemplate("secret-access-test", secretAPITestPodTmpl)

	return pg
}

// RegisterTemplate registers a manifest template for the given module ID.
// If a template is already registered for the ID it is overwritten.
func (pg *PayloadGenerator) RegisterTemplate(moduleID string, tmpl string) {
	pg.templates[moduleID] = tmpl
	pg.logger.Debug("registered manifest template",
		zap.String("module_id", moduleID),
	)
}

// ---------------------------------------------------------------------------
// GenerateManifest
// ---------------------------------------------------------------------------

// GenerateManifest renders the Kubernetes manifest for the given module using
// the provided attack configuration. It applies parameter defaults, validates
// the result, and executes the module's Go template with standard template
// functions.
func (pg *PayloadGenerator) GenerateManifest(module AttackModule, config AttackConfig) ([]byte, error) {
	// 1. Apply defaults for missing parameters.
	params := ApplyDefaults(module, config.Parameters)

	// 2. Validate parameters.
	if err := ValidateParameters(module, params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// 3. Look up the registered template.
	tmplStr, ok := pg.templates[module.ID()]
	if !ok {
		return nil, fmt.Errorf("no manifest template registered for module %q", module.ID())
	}

	// 4. Build template data from config and parameters.
	data := buildTemplateData(module, config, params)

	// 5. Parse and execute the template.
	tmpl, err := template.New(module.ID()).Funcs(defaultTemplateFuncs()).Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template for module %q: %w", module.ID(), err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render manifest for module %q: %w", module.ID(), err)
	}

	return buf.Bytes(), nil
}

// GenerateManifestWithTemplate renders a manifest using an explicitly provided
// template string rather than the one registered for the module. This is
// useful for custom/user-defined templates stored in the database.
func (pg *PayloadGenerator) GenerateManifestWithTemplate(
	module AttackModule,
	config AttackConfig,
	tmplStr string,
) ([]byte, error) {
	// 1. Apply defaults.
	params := ApplyDefaults(module, config.Parameters)

	// 2. Validate parameters.
	if err := ValidateParameters(module, params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// 3. Build template data.
	data := buildTemplateData(module, config, params)

	// 4. Parse and execute.
	tmpl, err := template.New(module.ID()).Funcs(defaultTemplateFuncs()).Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse custom template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render custom manifest: %w", err)
	}

	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// GenerateCommand
// ---------------------------------------------------------------------------

// GenerateCommand produces a human-readable representation of the attack
// command that would be executed inside the attacker pod. This is useful for
// preview and audit purposes.
func (pg *PayloadGenerator) GenerateCommand(module AttackModule, config AttackConfig) (string, error) {
	// Apply defaults.
	params := ApplyDefaults(module, config.Parameters)

	// Validate parameters.
	if err := ValidateParameters(module, params); err != nil {
		return "", fmt.Errorf("parameter validation failed: %w", err)
	}

	// Build a command summary from the module's parameters.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Attack Module: %s (%s)\n", module.Name(), module.ID()))
	b.WriteString(fmt.Sprintf("# Category: %s | Severity: %s\n", module.Category(), module.Severity()))
	b.WriteString(fmt.Sprintf("# Namespace: %s\n", config.Namespace))
	b.WriteString(fmt.Sprintf("# Experiment: %s | Run: %s\n", config.ExperimentID, config.RunID))
	b.WriteString("#\n# Parameters:\n")

	for _, p := range module.Parameters() {
		val, ok := params[p.Name]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("#   %s = %v (%s)\n", p.Name, val, p.Description))
	}

	b.WriteString("#\n# Generated command:\n")

	// Module-specific command generation.
	switch module.ID() {
	case "pod-egress-test":
		b.WriteString(generateEgressCommand(params, config))
	case "pod-ingress-test":
		b.WriteString(generateIngressCommand(params, config))
	case "network-policy-test":
		b.WriteString(generateNetPolCommand(params, config))
	case "rbac-privilege-test":
		b.WriteString(generateRBACCommand(params, config))
	case "secret-access-test":
		b.WriteString(generateSecretCommand(params, config))
	default:
		b.WriteString(fmt.Sprintf("# (command generation not implemented for module %q)\n", module.ID()))
	}

	return b.String(), nil
}

// ---------------------------------------------------------------------------
// ValidateParameters
// ---------------------------------------------------------------------------

// ValidateParameters checks that the provided parameter map satisfies the
// module's parameter schema. It verifies that all required parameters are
// present, that types match, and that select-type parameters contain valid
// options.
func ValidateParameters(module AttackModule, params map[string]interface{}) error {
	moduleParams := module.Parameters()

	// Build a lookup map of the provided params.
	provided := make(map[string]bool, len(params))
	for k := range params {
		provided[k] = true
	}

	for _, mp := range moduleParams {
		val, exists := params[mp.Name]

		// Check required parameters.
		if mp.Required && !exists {
			return fmt.Errorf("required parameter %q is missing", mp.Name)
		}

		// Skip further validation if the parameter wasn't provided and isn't required.
		if !exists {
			continue
		}

		// Type validation.
		if err := validateParameterType(mp, val); err != nil {
			return fmt.Errorf("parameter %q: %w", mp.Name, err)
		}

		// Options validation for select type.
		if mp.Type == ParamTypeSelect && len(mp.Options) > 0 {
			strVal, ok := val.(string)
			if !ok {
				return fmt.Errorf("parameter %q: select-type parameter must be a string", mp.Name)
			}
			valid := false
			for _, opt := range mp.Options {
				if strVal == opt {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("parameter %q: value %q is not one of the allowed options: %v",
					mp.Name, strVal, mp.Options)
			}
		}
	}

	return nil
}

// validateParameterType checks that the provided value matches the expected
// parameter type.
func validateParameterType(mp Parameter, val interface{}) error {
	switch mp.Type {
	case ParamTypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}
	case ParamTypeInt:
		if !isIntLike(val) {
			return fmt.Errorf("expected int, got %T", val)
		}
	case ParamTypeBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", val)
		}
	case ParamTypeSelect:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string (select), got %T", val)
		}
	default:
		return fmt.Errorf("unknown parameter type %q", mp.Type)
	}
	return nil
}

// isIntLike returns true if the value can be treated as an integer.
func isIntLike(val interface{}) bool {
	switch val.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float64:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// ApplyDefaults
// ---------------------------------------------------------------------------

// ApplyDefaults returns a new parameter map with default values applied for
// any parameters that are missing or nil. It does NOT modify the input map.
func ApplyDefaults(module AttackModule, params map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(params))

	// Copy existing parameters.
	for k, v := range params {
		if v != nil {
			result[k] = v
		}
	}

	// Apply defaults for missing parameters.
	for _, mp := range module.Parameters() {
		if _, exists := result[mp.Name]; !exists && mp.Default != nil {
			result[mp.Name] = mp.Default
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Template helpers
// ---------------------------------------------------------------------------

// manifestTemplateData is the unified data structure passed to all manifest
// templates. Individual modules may use subsets of these fields.
type manifestTemplateData struct {
	// Core identifiers.
	RunID          string `json:"run_id"`
	ExperimentID   string `json:"experiment_id"`
	Namespace      string `json:"namespace"`
	ModuleID       string `json:"module_id"`
	ModuleName     string `json:"module_name"`
	Category       string `json:"category"`
	Severity       string `json:"severity"`
	Timeout        string `json:"timeout"`
	TimeoutSeconds int    `json:"timeout_seconds"`

	// Egress parameters.
	DestinationIP   string `json:"destination_ip,omitempty"`
	DestinationPort int    `json:"destination_port,omitempty"`
	Protocol        string `json:"destination_protocol,omitempty"`
	Attempts        int    `json:"attempts,omitempty"`

	// Ingress parameters.
	TargetService   string `json:"target_service,omitempty"`
	TargetPort      int    `json:"target_port,omitempty"`
	SourceNamespace string `json:"source_namespace,omitempty"`

	// Network policy parameters.
	TestCIDR  string `json:"test_cidr,omitempty"`
	TestPort  int    `json:"test_port,omitempty"`
	Direction string `json:"direction,omitempty"`

	// RBAC parameters.
	ServiceAccount string `json:"service_account,omitempty"`
	TestAction     string `json:"test_action,omitempty"`

	// Secret access parameters.
	SecretName string `json:"secret_name,omitempty"`
	MountPath  bool   `json:"mount_path,omitempty"`

	// Generic parameters map for extensibility.
	Parameters map[string]interface{} `json:"parameters"`
}

// buildTemplateData constructs the template data from the module and config.
func buildTemplateData(module AttackModule, config AttackConfig, params map[string]interface{}) manifestTemplateData {
	data := manifestTemplateData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		ModuleID:       module.ID(),
		ModuleName:     module.Name(),
		Category:       module.Category(),
		Severity:       module.Severity(),
		Timeout:        config.Timeout.String(),
		TimeoutSeconds: int(config.Timeout.Seconds()),
		Parameters:     params,
	}

	// Populate module-specific fields from params.
	if v, ok := params["destination_ip"].(string); ok {
		data.DestinationIP = v
	}
	if v, ok := toInt(params["destination_port"]); ok {
		data.DestinationPort = v
	}
	if v, ok := params["destination_protocol"].(string); ok {
		data.Protocol = v
	}
	if v, ok := toInt(params["attempts"]); ok {
		data.Attempts = v
	}
	if v, ok := params["target_service"].(string); ok {
		data.TargetService = v
	}
	if v, ok := toInt(params["target_port"]); ok {
		data.TargetPort = v
	}
	if v, ok := params["source_namespace"].(string); ok {
		data.SourceNamespace = v
	}
	if v, ok := params["test_cidr"].(string); ok {
		data.TestCIDR = v
	}
	if v, ok := toInt(params["test_port"]); ok {
		data.TestPort = v
	}
	if v, ok := params["direction"].(string); ok {
		data.Direction = v
	}
	if v, ok := params["service_account"].(string); ok {
		data.ServiceAccount = v
	}
	if v, ok := params["test_action"].(string); ok {
		data.TestAction = v
	}
	if v, ok := params["secret_name"].(string); ok {
		data.SecretName = v
	}
	if v, ok := params["mount_path"].(bool); ok {
		data.MountPath = v
	}

	// Ensure non-zero defaults where needed.
	if data.Attempts == 0 {
		data.Attempts = 3
	}
	if data.Protocol == "" {
		data.Protocol = "tcp"
	}
	if data.SourceNamespace == "" {
		data.SourceNamespace = "default"
	}
	if data.ServiceAccount == "" {
		data.ServiceAccount = "default"
	}

	return data
}

// defaultTemplateFuncs returns the standard template function map available
// to all manifest templates.
func defaultTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// toYAML is a no-op placeholder – modules render YAML directly via
		// their embedded templates. Including it avoids "function not defined"
		// errors for templates that reference it.
		"toYAML": func(v interface{}) string {
			b, err := json.Marshal(v)
			if err != nil {
				return fmt.Sprintf("%v", v)
			}
			return string(b)
		},
		// toJSON serialises a value to a JSON string.
		"toJSON": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		// quote wraps a string in double quotes.
		"quote": func(s string) string {
			return fmt.Sprintf("%q", s)
		},
		// default returns the first non-empty string argument.
		"default": func(def string, val string) string {
			if val == "" {
				return def
			}
			return val
		},
		// upper converts to uppercase.
		"upper": strings.ToUpper,
		// lower converts to lowercase.
		"lower": strings.ToLower,
		// trimSpace trims leading/trailing whitespace.
		"trimSpace": strings.TrimSpace,
	}
}

// ---------------------------------------------------------------------------
// Command generators (per-module)
// ---------------------------------------------------------------------------

// generateEgressCommand builds a human-readable command for the egress test.
func generateEgressCommand(params map[string]interface{}, config AttackConfig) string {
	destIP, _ := params["destination_ip"].(string)
	destPort, _ := toInt(params["destination_port"])
	protocol, _ := params["destination_protocol"].(string)
	timeoutSec, _ := toInt(params["timeout_seconds"])
	attempts, _ := toInt(params["attempts"])

	return fmt.Sprintf(
		"curl -s -o /dev/null -w '%%{http_code}' "+
			"--connect-timeout %d --max-time %d -%s %s:%d  (x%d attempts)",
		timeoutSec, timeoutSec, protocol, destIP, destPort, attempts,
	)
}

// generateIngressCommand builds a human-readable command for the ingress test.
func generateIngressCommand(params map[string]interface{}, config AttackConfig) string {
	targetService, _ := params["target_service"].(string)
	targetPort, _ := toInt(params["target_port"])
	sourceNamespace, _ := params["source_namespace"].(string)
	timeoutSec, _ := toInt(params["timeout_seconds"])
	attempts, _ := toInt(params["attempts"])

	target := fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		targetService, config.Namespace, targetPort)

	return fmt.Sprintf(
		"curl -s -o /dev/null -w '%%{http_code}' "+
			"--connect-timeout %d --max-time %d http://%s  (from %s, x%d attempts)",
		timeoutSec, timeoutSec, target, sourceNamespace, attempts,
	)
}

// generateNetPolCommand builds a human-readable command for the network policy test.
func generateNetPolCommand(params map[string]interface{}, config AttackConfig) string {
	testCIDR, _ := params["test_cidr"].(string)
	testPort, _ := toInt(params["test_port"])
	timeoutSec, _ := toInt(params["timeout_seconds"])

	return fmt.Sprintf(
		"curl -s -o /dev/null -w '%%{http_code}' "+
			"--connect-timeout %d --max-time %d https://%s:%d",
		timeoutSec, timeoutSec, testCIDR, testPort,
	)
}

// generateRBACCommand builds a human-readable command for the RBAC test.
func generateRBACCommand(params map[string]interface{}, config AttackConfig) string {
	serviceAccount, _ := params["service_account"].(string)
	testAction, _ := params["test_action"].(string)
	timeoutSec, _ := toInt(params["timeout_seconds"])

	var kubectlCmd string
	switch testAction {
	case "list-secrets":
		kubectlCmd = fmt.Sprintf("kubectl get secrets -n %s --request-timeout=%ds", config.Namespace, timeoutSec)
	case "create-pods":
		kubectlCmd = fmt.Sprintf("kubectl run test-rbac-pod --image=busybox -n %s --restart=Never --request-timeout=%ds", config.Namespace, timeoutSec)
	case "delete-pods":
		kubectlCmd = fmt.Sprintf("kubectl delete pods --all -n %s --dry-run=client --request-timeout=%ds", config.Namespace, timeoutSec)
	case "list-configmaps":
		kubectlCmd = fmt.Sprintf("kubectl get configmaps -n %s --request-timeout=%ds", config.Namespace, timeoutSec)
	case "exec-pods":
		kubectlCmd = fmt.Sprintf("kubectl auth can-i create pods/exec -n %s --request-timeout=%ds", config.Namespace, timeoutSec)
	default:
		kubectlCmd = fmt.Sprintf("# unknown action: %s", testAction)
	}

	return fmt.Sprintf("# Service Account: %s\n%s", serviceAccount, kubectlCmd)
}

// generateSecretCommand builds a human-readable command for the secret access test.
func generateSecretCommand(params map[string]interface{}, config AttackConfig) string {
	secretName, _ := params["secret_name"].(string)
	mountPath, _ := params["mount_path"].(bool)
	timeoutSec, _ := toInt(params["timeout_seconds"])

	var b strings.Builder

	if secretName != "" {
		b.WriteString(fmt.Sprintf("kubectl get secret %s -n %s --request-timeout=%ds\n",
			secretName, config.Namespace, timeoutSec))
		b.WriteString(fmt.Sprintf("kubectl get secret %s -o yaml -n %s --request-timeout=%ds\n",
			secretName, config.Namespace, timeoutSec))
	} else {
		b.WriteString(fmt.Sprintf("kubectl get secrets -n %s --request-timeout=%ds\n",
			config.Namespace, timeoutSec))
	}

	if mountPath {
		b.WriteString("# Mount path inspection:\n")
		b.WriteString("env | grep -i -E '(secret|password|token|key|credential)'\n")
		b.WriteString("ls -la /etc/secrets /var/secrets /secrets /run/secrets /etc/config\n")
		b.WriteString("ls -la /var/run/secrets/kubernetes.io/serviceaccount/\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Template data extraction helpers
// ---------------------------------------------------------------------------

// ParameterSchema represents a parameter's schema for JSON serialisation.
type ParameterSchema struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description"`
	Options     []string    `json:"options,omitempty"`
}

// ModuleSchema represents the full JSON schema for an attack module.
type ModuleSchema struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	Parameters  []ParameterSchema `json:"parameters"`
}

// GetModuleSchema returns the JSON-serialisable schema for an attack module.
func GetModuleSchema(module AttackModule) ModuleSchema {
	params := make([]ParameterSchema, len(module.Parameters()))
	for i, p := range module.Parameters() {
		params[i] = ParameterSchema{
			Name:        p.Name,
			Type:        string(p.Type),
			Required:    p.Required,
			Default:     p.Default,
			Description: p.Description,
			Options:     p.Options,
		}
	}
	return ModuleSchema{
		ID:          module.ID(),
		Name:        module.Name(),
		Category:    module.Category(),
		Severity:    module.Severity(),
		Description: module.Description(),
		Parameters:  params,
	}
}

// ParametersToJSONSchema converts a module's parameter definitions into a
// JSON Schema document suitable for validation and UI rendering.
func ParametersToJSONSchema(module AttackModule) map[string]interface{} {
	properties := make(map[string]interface{})
	required := []string{}

	for _, p := range module.Parameters() {
		prop := map[string]interface{}{
			"description": p.Description,
		}

		switch p.Type {
		case ParamTypeString:
			prop["type"] = "string"
		case ParamTypeInt:
			prop["type"] = "integer"
		case ParamTypeBool:
			prop["type"] = "boolean"
		case ParamTypeSelect:
			prop["type"] = "string"
			prop["enum"] = p.Options
		}

		if p.Default != nil {
			prop["default"] = p.Default
		}

		properties[p.Name] = prop

		if p.Required {
			required = append(required, p.Name)
		}
	}

	return map[string]interface{}{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// ---------------------------------------------------------------------------
// Parameter coercion helpers (shared with modules)
// ---------------------------------------------------------------------------

// toInt coerces an interface{} to int, handling float64 (from JSON) and int64.
// This is defined here as the canonical location; other modules in the package
// reference it directly.
func toInt(v interface{}) (int, bool) {
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

// coerceParameter converts a parameter value to the expected Go type based on
// the parameter definition. This is useful when parameters come from JSON
// deserialisation (where integers become float64).
func coerceParameter(p Parameter, val interface{}) (interface{}, error) {
	switch p.Type {
	case ParamTypeString, ParamTypeSelect:
		if s, ok := val.(string); ok {
			return s, nil
		}
		return fmt.Sprintf("%v", val), nil

	case ParamTypeInt:
		n, ok := toInt(val)
		if !ok {
			return nil, fmt.Errorf("cannot convert %T to int", val)
		}
		return n, nil

	case ParamTypeBool:
		if b, ok := val.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("cannot convert %T to bool", val)

	default:
		return val, nil
	}
}

// CoerceParameters coerces all parameter values in the map to their expected
// Go types based on the module's parameter definitions. This is useful when
// parameters have been deserialised from JSON (e.g. from the database or API).
func CoerceParameters(module AttackModule, params map[string]interface{}) map[string]interface{} {
	paramDefs := make(map[string]Parameter, len(module.Parameters()))
	for _, p := range module.Parameters() {
		paramDefs[p.Name] = p
	}

	result := make(map[string]interface{}, len(params))
	for k, v := range params {
		if pd, ok := paramDefs[k]; ok {
			coerced, err := coerceParameter(pd, v)
			if err != nil {
				// Keep original value if coercion fails.
				result[k] = v
				continue
			}
			result[k] = coerced
		} else {
			result[k] = v
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Utility functions
// ---------------------------------------------------------------------------

// FormatDuration is a template helper that formats a time.Duration in a
// human-readable form.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
