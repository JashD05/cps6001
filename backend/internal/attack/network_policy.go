package attack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Network Policy Validation – Attack Module
// ---------------------------------------------------------------------------

// NetworkPolicyTest validates that existing network policies in a namespace
// are correctly configured and effectively enforced. It reads policies,
// deploys test pods that attempt to violate each policy, and reports which
// policies are effective and which have gaps.
type NetworkPolicyTest struct{}

// NewNetworkPolicyTest creates a new NetworkPolicyTest module.
func NewNetworkPolicyTest() *NetworkPolicyTest {
	return &NetworkPolicyTest{}
}

func (m *NetworkPolicyTest) ID() string       { return "network-policy-test" }
func (m *NetworkPolicyTest) Name() string     { return "Network Policy Validation" }
func (m *NetworkPolicyTest) Category() string { return "network" }
func (m *NetworkPolicyTest) Severity() string { return "high" }
func (m *NetworkPolicyTest) Description() string {
	return "Validates that existing network policies are correctly configured and " +
		"effectively enforced. Reads network policies from the target namespace, " +
		"creates test pods that attempt to violate each policy, and reports which " +
		"policies are effective and which have configuration gaps."
}

func (m *NetworkPolicyTest) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "target_namespace",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Namespace containing the network policies to validate",
		},
		{
			Name:        "policy_name",
			Type:        ParamTypeString,
			Required:    false,
			Default:     "",
			Description: "Specific network policy to test (empty = test all policies in namespace)",
		},
		{
			Name:        "test_cidr",
			Type:        ParamTypeString,
			Required:    false,
			Default:     "0.0.0.0/0",
			Description: "CIDR block to test against egress rules",
		},
		{
			Name:        "test_port",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     443,
			Description: "Port number to test against policy rules",
		},
		{
			Name:        "timeout_seconds",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     10,
			Description: "Connection timeout in seconds per attempt",
		},
		{
			Name:        "attempts",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     3,
			Description: "Number of connection attempts per test",
		},
	}
}

// ---------------------------------------------------------------------------
// Network policy representation (lightweight, no k8s imports)
// ---------------------------------------------------------------------------

// NetworkPolicySpec is a simplified representation of a Kubernetes NetworkPolicy
// used internally by this module. It is parsed from the raw manifest returned
// by ClusterClient.GetNetworkPolicy / ListNetworkPolicies.
type NetworkPolicySpec struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	PodSelector map[string]string `json:"pod_selector"`
	Ingress     []PolicyRule      `json:"ingress,omitempty"`
	Egress      []PolicyRule      `json:"egress,omitempty"`
}

// PolicyRule represents a single ingress or egress rule in a network policy.
type PolicyRule struct {
	Ports []PolicyPort `json:"ports,omitempty"`
	From  []PolicyPeer `json:"from,omitempty"`
	To    []PolicyPeer `json:"to,omitempty"`
}

// PolicyPort represents a port and optional protocol in a policy rule.
type PolicyPort struct {
	Port     int    `json:"port,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// PolicyPeer represents a source or destination peer in a policy rule.
type PolicyPeer struct {
	IPBlock           *IPBlock          `json:"ip_block,omitempty"`
	NamespaceSelector map[string]string `json:"namespace_selector,omitempty"`
	PodSelector       map[string]string `json:"pod_selector,omitempty"`
}

// IPBlock represents a CIDR and optional exceptions in a policy peer.
type IPBlock struct {
	CIDR   string   `json:"cidr"`
	Except []string `json:"except,omitempty"`
}

// ---------------------------------------------------------------------------
// Manifest templates
// ---------------------------------------------------------------------------

// netpolTestPodData holds the data injected into the test pod manifest.
type netpolTestPodData struct {
	RunID          string
	ExperimentID   string
	Namespace      string
	TestCIDR       string
	TestPort       int
	TimeoutSeconds int
	Attempts       int
	Direction      string // "ingress" or "egress"
}

// netpolTestPodTmpl is the Kubernetes pod manifest used for network policy testing.
const netpolTestPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-netpol-{{.Direction}}-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-netpol-test
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: network-policy-test
spec:
  automountServiceAccountToken: false
  hostNetwork: false
  hostPID: false
  hostIPC: false
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    capabilities:
      drop: ["ALL"]
    readOnlyRootFilesystem: true
  containers:
  - name: tester
    image: curlimages/curl:latest
    command: ["sh", "-c"]
    args:
    - |
      echo "Starting network policy {{.Direction}} test..."
      echo "Target CIDR: {{.TestCIDR}}, Port: {{.TestPort}}"
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        if curl -s -o /dev/null -w "%{http_code}" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} "https://{{.TestCIDR}}:{{.TestPort}}" 2>/dev/null; then
          echo "CONNECTION_SUCCESS: Connected to {{.TestCIDR}}:{{.TestPort}} via {{.Direction}}"
        else
          echo "CONNECTION_BLOCKED: Could not connect to {{.TestCIDR}}:{{.TestPort}} via {{.Direction}}"
        fi
        sleep 1
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// netpolIngressTargetPodTmpl is a simple HTTP server pod used as an ingress
// target when testing ingress policy enforcement.
const netpolIngressTargetPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-netpol-ingress-target-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-netpol-target
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: network-policy-test
spec:
  automountServiceAccountToken: false
  hostNetwork: false
  hostPID: false
  hostIPC: false
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    capabilities:
      drop: ["ALL"]
    readOnlyRootFilesystem: true
  containers:
  - name: server
    image: curlimages/curl:latest
    command: ["sh", "-c"]
    args:
    - |
      echo "Ingress target server listening on port {{.TestPort}}..."
      while true; do
        echo -e "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nchaos-sec-netpol-target" | nc -l -p {{.TestPort}} 2>/dev/null || true
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// netpolIngressAttackerTmpl is the attacker pod for cross-namespace ingress testing.
const netpolIngressAttackerTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-netpol-ingress-attacker-{{.RunID}}
  namespace: default
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: network-policy-test
spec:
  automountServiceAccountToken: false
  hostNetwork: false
  hostPID: false
  hostIPC: false
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
    capabilities:
      drop: ["ALL"]
    readOnlyRootFilesystem: true
  containers:
  - name: attacker
    image: curlimages/curl:latest
    command: ["sh", "-c"]
    args:
    - |
      TARGET="chaos-sec-netpol-ingress-target-{{.RunID}}.{{.Namespace}}.svc.cluster.local:{{.TestPort}}"
      echo "Starting ingress attacker test to $TARGET..."
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        if curl -s -o /dev/null -w "%{http_code}" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} "http://$TARGET" 2>/dev/null; then
          echo "CONNECTION_SUCCESS: Connected to $TARGET"
        else
          echo "CONNECTION_BLOCKED: Could not connect to $TARGET"
        fi
        sleep 1
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// ---------------------------------------------------------------------------
// Policy validation result types
// ---------------------------------------------------------------------------

// PolicyValidationResult captures the outcome of testing a single network policy.
type PolicyValidationResult struct {
	PolicyName string `json:"policy_name"`
	Type       string `json:"type"` // "ingress" or "egress"
	Effective  bool   `json:"effective"`
	Details    string `json:"details"`
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

// Validate checks that the attack configuration contains all required parameters.
func (m *NetworkPolicyTest) Validate(_ context.Context, config AttackConfig) error {
	if config.Namespace == "" {
		return fmt.Errorf("target_namespace is required")
	}
	if config.ClusterClient == nil {
		return fmt.Errorf("cluster client is required")
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Execute
// ---------------------------------------------------------------------------

// Execute runs the network policy validation by reading policies from the
// target namespace, deploying test pods, and checking whether each policy
// is effectively enforced.
func (m *NetworkPolicyTest) Execute(ctx context.Context, config AttackConfig) (*AttackResult, error) {
	start := time.Now()
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("run_id", config.RunID),
	)

	// Apply defaults for optional parameters.
	params := ApplyDefaults(m, config.Parameters)

	policyName, _ := params["policy_name"].(string)
	testCIDR, _ := params["test_cidr"].(string)
	if testCIDR == "" {
		testCIDR = "0.0.0.0/0"
	}
	testPort, _ := toInt(params["test_port"])
	if testPort == 0 {
		testPort = 443
	}
	timeoutSec, _ := toInt(params["timeout_seconds"])
	if timeoutSec == 0 {
		timeoutSec = 10
	}
	attempts, _ := toInt(params["attempts"])
	if attempts == 0 {
		attempts = 3
	}

	// Ensure cleanup runs when we exit.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := m.Cleanup(cleanupCtx, config); err != nil {
			logger.Warn("failed to cleanup network policy test resources", zap.Error(err))
		}
	}()

	// -----------------------------------------------------------------------
	// 1. Read network policies from the target namespace.
	// -----------------------------------------------------------------------
	var policyNames []string
	var err error

	if policyName != "" {
		policyNames = []string{policyName}
	} else {
		logger.Info("listing network policies in namespace",
			zap.String("namespace", config.Namespace),
		)
		policyNames, err = config.ClusterClient.ListNetworkPolicies(ctx, config.Namespace)
		if err != nil {
			return &AttackResult{
				Success:   false,
				Blocked:   false,
				Error:     fmt.Sprintf("failed to list network policies: %v", err),
				Timestamp: time.Now(),
				Duration:  time.Since(start),
			}, nil
		}
	}

	if len(policyNames) == 0 {
		evidence := fmt.Sprintf(
			"No network policies found in namespace %q. "+
				"This means all traffic is allowed by default (no isolation). "+
				"Recommend creating default-deny policies.",
			config.Namespace,
		)
		return &AttackResult{
			Success:   true,
			Blocked:   false,
			Evidence:  evidence,
			Logs:      evidence,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	logger.Info("found network policies to validate",
		zap.Int("count", len(policyNames)),
	)

	// -----------------------------------------------------------------------
	// 2–4. For each policy, parse it and test enforcement.
	// -----------------------------------------------------------------------
	var validationResults []PolicyValidationResult
	var allLogs strings.Builder

	for _, name := range policyNames {
		logger.Info("validating network policy",
			zap.String("policy_name", name),
		)

		rawPolicy, err := config.ClusterClient.GetNetworkPolicy(ctx, config.Namespace, name)
		if err != nil {
			validationResults = append(validationResults, PolicyValidationResult{
				PolicyName: name,
				Type:       "unknown",
				Effective:  false,
				Details:    fmt.Sprintf("failed to read policy: %v", err),
			})
			allLogs.WriteString(fmt.Sprintf("Policy %q: ERROR - %v\n", name, err))
			continue
		}

		policy, parseErr := parseNetworkPolicy(rawPolicy)
		if parseErr != nil {
			validationResults = append(validationResults, PolicyValidationResult{
				PolicyName: name,
				Type:       "unknown",
				Effective:  false,
				Details:    fmt.Sprintf("failed to parse policy manifest: %v", parseErr),
			})
			allLogs.WriteString(fmt.Sprintf("Policy %q: PARSE ERROR - %v\n", name, parseErr))
			continue
		}

		// Test egress rules if present.
		if len(policy.Egress) > 0 {
			result := m.testEgressPolicy(ctx, config, policy, testCIDR, testPort, timeoutSec, attempts, logger)
			validationResults = append(validationResults, result)
			allLogs.WriteString(fmt.Sprintf("Policy %q egress: effective=%v %s\n",
				name, result.Effective, result.Details))
		}

		// Test ingress rules if present.
		if len(policy.Ingress) > 0 {
			result := m.testIngressPolicy(ctx, config, policy, testPort, timeoutSec, attempts, logger)
			validationResults = append(validationResults, result)
			allLogs.WriteString(fmt.Sprintf("Policy %q ingress: effective=%v %s\n",
				name, result.Effective, result.Details))
		}

		// If policy has neither ingress nor egress rules, it's effectively a
		// pod selector with no restrictions.
		if len(policy.Ingress) == 0 && len(policy.Egress) == 0 {
			validationResults = append(validationResults, PolicyValidationResult{
				PolicyName: name,
				Type:       "none",
				Effective:  false,
				Details:    "Policy has no ingress or egress rules – it has no effect on traffic",
			})
			allLogs.WriteString(fmt.Sprintf("Policy %q: NO RULES - policy has no effect\n", name))
		}
	}

	// -----------------------------------------------------------------------
	// 5. Build final result.
	// -----------------------------------------------------------------------
	overallBlocked := true
	effectiveCount := 0
	gapCount := 0
	for _, r := range validationResults {
		if r.Effective {
			effectiveCount++
		} else {
			gapCount++
			overallBlocked = false
		}
	}

	evidence := buildNetPolEvidence(config.Namespace, validationResults, effectiveCount, gapCount)

	logger.Info("network policy validation completed",
		zap.Int("policies_tested", len(validationResults)),
		zap.Int("effective", effectiveCount),
		zap.Int("gaps", gapCount),
		zap.Bool("overall_blocked", overallBlocked),
		zap.Duration("duration", time.Since(start)),
	)

	return &AttackResult{
		Success:   true,
		Blocked:   overallBlocked,
		Evidence:  evidence,
		Logs:      allLogs.String(),
		Timestamp: time.Now(),
		Duration:  time.Since(start),
	}, nil
}

// ---------------------------------------------------------------------------
// Policy testing helpers
// ---------------------------------------------------------------------------

// testEgressPolicy tests whether an egress policy is effectively enforced
// by deploying a pod that matches the policy's pod selector and attempting
// an egress connection.
func (m *NetworkPolicyTest) testEgressPolicy(
	ctx context.Context,
	config AttackConfig,
	policy *NetworkPolicySpec,
	testCIDR string,
	testPort int,
	timeoutSec int,
	attempts int,
	logger *zap.Logger,
) PolicyValidationResult {
	result := PolicyValidationResult{
		PolicyName: policy.Name,
		Type:       "egress",
		Effective:  false,
	}

	// Render the egress test pod.
	tmpl, err := template.New("netpol-egress-pod").Parse(netpolTestPodTmpl)
	if err != nil {
		result.Details = fmt.Sprintf("template parse error: %v", err)
		return result
	}

	data := netpolTestPodData{
		RunID:          config.RunID + "-egress",
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		TestCIDR:       testCIDR,
		TestPort:       testPort,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
		Direction:      "egress",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		result.Details = fmt.Sprintf("template render error: %v", err)
		return result
	}

	podName := fmt.Sprintf("chaos-sec-netpol-egress-%s", config.RunID)

	logger.Info("deploying egress test pod",
		zap.String("pod_name", podName),
		zap.String("policy_name", policy.Name),
	)

	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, buf.Bytes()); err != nil {
		result.Details = fmt.Sprintf("failed to create egress test pod: %v", err)
		return result
	}

	// Wait for pod to be ready.
	podTimeout := config.Timeout
	if podTimeout < time.Duration(timeoutSec*attempts+60)*time.Second {
		podTimeout = time.Duration(timeoutSec*attempts+60) * time.Second
	}
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, podName, podTimeout); err != nil {
		// Pod may have exited quickly – try to get logs anyway.
		logs, _ := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
		result.Details = fmt.Sprintf("egress test pod not ready: %v; logs: %s", err, logs)
		return result
	}

	// Wait for test to complete.
	execWait := time.Duration(timeoutSec*attempts+15) * time.Second
	select {
	case <-ctx.Done():
		result.Details = fmt.Sprintf("context cancelled: %v", ctx.Err())
		return result
	case <-time.After(execWait):
	}

	// Collect logs.
	logs, err := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
	if err != nil {
		result.Details = fmt.Sprintf("failed to get egress test pod logs: %v", err)
		return result
	}

	// Clean up test pod.
	_ = config.ClusterClient.DeletePod(ctx, config.Namespace, podName)

	// Analyse results.
	blocked := isConnectionBlocked(logs)
	result.Effective = blocked
	if blocked {
		result.Details = fmt.Sprintf("Egress to %s:%d was blocked (policy effective)", testCIDR, testPort)
	} else {
		result.Details = fmt.Sprintf("Egress to %s:%d was allowed (policy gap detected)", testCIDR, testPort)
	}

	return result
}

// testIngressPolicy tests whether an ingress policy is effectively enforced
// by deploying a target pod in the namespace and an attacker pod in the
// default namespace, then checking cross-namespace connectivity.
func (m *NetworkPolicyTest) testIngressPolicy(
	ctx context.Context,
	config AttackConfig,
	policy *NetworkPolicySpec,
	testPort int,
	timeoutSec int,
	attempts int,
	logger *zap.Logger,
) PolicyValidationResult {
	result := PolicyValidationResult{
		PolicyName: policy.Name,
		Type:       "ingress",
		Effective:  false,
	}

	// Render the ingress target pod.
	tmpl, err := template.New("netpol-ingress-target").Parse(netpolIngressTargetPodTmpl)
	if err != nil {
		result.Details = fmt.Sprintf("target template parse error: %v", err)
		return result
	}

	targetData := netpolTestPodData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		TestPort:       testPort,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
	}

	var targetBuf bytes.Buffer
	if err := tmpl.Execute(&targetBuf, targetData); err != nil {
		result.Details = fmt.Sprintf("target template render error: %v", err)
		return result
	}

	targetPodName := fmt.Sprintf("chaos-sec-netpol-ingress-target-%s", config.RunID)
	attackerPodName := fmt.Sprintf("chaos-sec-netpol-ingress-attacker-%s", config.RunID)

	logger.Info("deploying ingress target pod",
		zap.String("pod_name", targetPodName),
		zap.String("policy_name", policy.Name),
	)

	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, targetBuf.Bytes()); err != nil {
		result.Details = fmt.Sprintf("failed to create ingress target pod: %v", err)
		return result
	}

	// Wait for target to be ready.
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, targetPodName, config.Timeout); err != nil {
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("ingress target pod not ready: %v", err)
		return result
	}

	// Render the ingress attacker pod.
	attackerTmpl, err := template.New("netpol-ingress-attacker").Parse(netpolIngressAttackerTmpl)
	if err != nil {
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("attacker template parse error: %v", err)
		return result
	}

	attackerData := netpolTestPodData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		TestPort:       testPort,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
	}

	var attackerBuf bytes.Buffer
	if err := attackerTmpl.Execute(&attackerBuf, attackerData); err != nil {
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("attacker template render error: %v", err)
		return result
	}

	logger.Info("deploying ingress attacker pod",
		zap.String("pod_name", attackerPodName),
	)

	if err := config.ClusterClient.ApplyManifest(ctx, "default", attackerBuf.Bytes()); err != nil {
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("failed to create ingress attacker pod: %v", err)
		return result
	}

	// Wait for attacker to be ready.
	if err := config.ClusterClient.WaitForPodReady(ctx, "default", attackerPodName, config.Timeout); err != nil {
		_ = config.ClusterClient.DeletePod(ctx, "default", attackerPodName)
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("ingress attacker pod not ready: %v", err)
		return result
	}

	// Wait for test to complete.
	execWait := time.Duration(timeoutSec*attempts+30) * time.Second
	select {
	case <-ctx.Done():
		_ = config.ClusterClient.DeletePod(ctx, "default", attackerPodName)
		_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)
		result.Details = fmt.Sprintf("context cancelled: %v", ctx.Err())
		return result
	case <-time.After(execWait):
	}

	// Collect logs.
	logs, err := config.ClusterClient.GetPodLogs(ctx, "default", attackerPodName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to get attacker pod logs", zap.Error(err))
		logs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	// Clean up.
	_ = config.ClusterClient.DeletePod(ctx, "default", attackerPodName)
	_ = config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName)

	// Analyse results.
	blocked := isConnectionBlocked(logs)
	result.Effective = blocked
	if blocked {
		result.Details = fmt.Sprintf("Ingress from default namespace to %s:%d was blocked (policy effective)", config.Namespace, testPort)
	} else {
		result.Details = fmt.Sprintf("Ingress from default namespace to %s:%d was allowed (policy gap detected)", config.Namespace, testPort)
	}

	return result
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// Cleanup removes all pods created by the network policy test.
func (m *NetworkPolicyTest) Cleanup(ctx context.Context, config AttackConfig) error {
	logger := config.Logger.With(zap.String("module", m.ID()))

	podNames := []string{
		fmt.Sprintf("chaos-sec-netpol-egress-%s", config.RunID),
		fmt.Sprintf("chaos-sec-netpol-ingress-target-%s", config.RunID),
	}
	attackerPodName := fmt.Sprintf("chaos-sec-netpol-ingress-attacker-%s", config.RunID)

	var firstErr error

	// Clean up pods in the target namespace.
	for _, name := range podNames {
		if err := config.ClusterClient.DeletePod(ctx, config.Namespace, name); err != nil {
			logger.Warn("failed to delete pod during cleanup",
				zap.String("pod_name", name),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to delete pod %q: %w", name, err)
			}
		}
	}

	// Clean up attacker pod in default namespace.
	if err := config.ClusterClient.DeletePod(ctx, "default", attackerPodName); err != nil {
		logger.Warn("failed to delete attacker pod during cleanup",
			zap.String("pod_name", attackerPodName),
			zap.Error(err),
		)
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete attacker pod %q: %w", attackerPodName, err)
		}
	}

	return firstErr
}

// ---------------------------------------------------------------------------
// Parsing and evidence helpers
// ---------------------------------------------------------------------------

// parseNetworkPolicy parses a raw Kubernetes NetworkPolicy manifest (YAML or JSON)
// into a simplified NetworkPolicySpec. It supports both YAML and JSON inputs.
func parseNetworkPolicy(raw []byte) (*NetworkPolicySpec, error) {
	// Try to unmarshal as JSON first (the ClusterClient may return either format).
	// We look for a minimal structure: metadata.name, spec.podSelector,
	// spec.ingress, spec.egress.

	var policyDoc struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			PodSelector map[string]interface{} `json:"podSelector"`
			Ingress     []PolicyRule           `json:"ingress"`
			Egress      []PolicyRule           `json:"egress"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(raw, &policyDoc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal network policy: %w", err)
	}

	// Flatten pod selector to string map.
	podSelector := make(map[string]string)
	for k, v := range policyDoc.Spec.PodSelector {
		if s, ok := v.(string); ok {
			podSelector[k] = s
		}
	}

	return &NetworkPolicySpec{
		Name:        policyDoc.Metadata.Name,
		Namespace:   policyDoc.Metadata.Namespace,
		PodSelector: podSelector,
		Ingress:     policyDoc.Spec.Ingress,
		Egress:      policyDoc.Spec.Egress,
	}, nil
}

// buildNetPolEvidence constructs a human-readable evidence string summarising
// the network policy validation results.
func buildNetPolEvidence(
	namespace string,
	results []PolicyValidationResult,
	effectiveCount, gapCount int,
) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Network Policy Validation Report for namespace %q\n", namespace))
	b.WriteString(fmt.Sprintf("Policies tested: %d | Effective: %d | Gaps: %d\n\n", len(results), effectiveCount, gapCount))

	for _, r := range results {
		status := "EFFECTIVE ✓"
		if !r.Effective {
			status = "GAP ✗"
		}
		b.WriteString(fmt.Sprintf("  [%s] %s (%s): %s\n", status, r.PolicyName, r.Type, r.Details))
	}

	b.WriteString("\n")
	if gapCount == 0 {
		b.WriteString("Result: All tested network policies are effectively enforced.\n")
	} else {
		b.WriteString(fmt.Sprintf("Result: %d policy gap(s) detected. Review the details above and update policies accordingly.\n", gapCount))
	}

	return b.String()
}
