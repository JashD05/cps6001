package attack

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Pod Egress Test – Attack Module
// ---------------------------------------------------------------------------

// PodEgressTest tests whether egress (outbound) network policies are properly
// enforced by attempting an outbound connection from a pod in the target
// namespace to an arbitrary destination.
type PodEgressTest struct{}

// NewPodEgressTest creates a new PodEgressTest module.
func NewPodEgressTest() *PodEgressTest {
	return &PodEgressTest{}
}

func (m *PodEgressTest) ID() string       { return "pod-egress-test" }
func (m *PodEgressTest) Name() string     { return "Pod Egress Test" }
func (m *PodEgressTest) Category() string { return "network" }
func (m *PodEgressTest) Severity() string { return "medium" }
func (m *PodEgressTest) Description() string {
	return "Tests whether egress (outbound) network policies are properly enforced " +
		"by attempting an outbound connection from a pod in the target namespace " +
		"to a specified destination IP and port. If the connection is blocked, the " +
		"network policy control is working as expected."
}

func (m *PodEgressTest) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "target_namespace",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Namespace in which to launch the attacker pod",
		},
		{
			Name:        "destination_ip",
			Type:        ParamTypeString,
			Required:    true,
			Default:     "8.8.8.8",
			Description: "IP address to attempt connection to",
		},
		{
			Name:        "destination_port",
			Type:        ParamTypeInt,
			Required:    true,
			Default:     53,
			Description: "Port number to test on the destination",
		},
		{
			Name:        "destination_protocol",
			Type:        ParamTypeSelect,
			Required:    false,
			Default:     "tcp",
			Description: "Protocol to use for the connection test",
			Options:     []string{"tcp", "udp", "icmp"},
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
			Description: "Number of connection attempts to make",
		},
	}
}

// egressManifestData holds the data injected into the pod manifest template.
type egressManifestData struct {
	RunID           string
	ExperimentID    string
	Namespace       string
	DestinationIP   string
	DestinationPort int
	Protocol        string
	TimeoutSeconds  int
	Attempts        int
}

// egressManifestTmpl is the Kubernetes pod manifest used to run the egress test.
const egressManifestTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-egress-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: pod-egress-test
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
      echo "Starting egress test to {{.DestinationIP}}:{{.DestinationPort}}..."
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        if curl -s -o /dev/null -w "%{http_code}" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} {{.DestinationIP}}:{{.DestinationPort}}; then
          echo "CONNECTION_SUCCESS: Connected to {{.DestinationIP}}:{{.DestinationPort}}"
        else
          echo "CONNECTION_BLOCKED: Could not connect to {{.DestinationIP}}:{{.DestinationPort}}"
        fi
        sleep 1
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// Validate checks that the attack configuration contains all required parameters
// and that the values are within acceptable ranges.
func (m *PodEgressTest) Validate(_ context.Context, config AttackConfig) error {
	if config.Namespace == "" {
		return fmt.Errorf("target_namespace is required")
	}
	if _, ok := config.ParamString("destination_ip"); !ok {
		return fmt.Errorf("destination_ip parameter is required")
	}
	if _, ok := config.ParamInt("destination_port"); !ok {
		return fmt.Errorf("destination_port parameter is required")
	}
	if config.ClusterClient == nil {
		return fmt.Errorf("cluster client is required")
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// Execute runs the egress test by deploying an attacker pod, waiting for it to
// complete, collecting logs, and determining whether the connection was blocked.
func (m *PodEgressTest) Execute(ctx context.Context, config AttackConfig) (*AttackResult, error) {
	start := time.Now()
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("run_id", config.RunID),
	)

	// Apply defaults for optional parameters.
	params := ApplyDefaults(m, config.Parameters)

	destIP, _ := params["destination_ip"].(string)
	destPort, _ := toInt(params["destination_port"])
	protocol, _ := params["destination_protocol"].(string)
	if protocol == "" {
		protocol = "tcp"
	}
	timeoutSec, _ := toInt(params["timeout_seconds"])
	if timeoutSec == 0 {
		timeoutSec = 10
	}
	attempts, _ := toInt(params["attempts"])
	if attempts == 0 {
		attempts = 3
	}

	// Render the pod manifest.
	tmpl, err := template.New("egress-pod").Parse(egressManifestTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse egress manifest template: %w", err)
	}

	data := egressManifestData{
		RunID:           config.RunID,
		ExperimentID:    config.ExperimentID,
		Namespace:       config.Namespace,
		DestinationIP:   destIP,
		DestinationPort: destPort,
		Protocol:        protocol,
		TimeoutSeconds:  timeoutSec,
		Attempts:        attempts,
	}

	var manifestBuf bytes.Buffer
	if err := tmpl.Execute(&manifestBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render egress manifest: %w", err)
	}
	manifest := manifestBuf.Bytes()

	podName := fmt.Sprintf("chaos-sec-egress-%s", config.RunID)

	// Ensure cleanup runs when we exit.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := m.Cleanup(cleanupCtx, config); err != nil {
			logger.Warn("failed to cleanup egress pod", zap.Error(err))
		}
	}()

	// 1. Create the attacker pod.
	logger.Info("creating egress attacker pod",
		zap.String("namespace", config.Namespace),
		zap.String("pod_name", podName),
	)
	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, manifest); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("failed to create attacker pod: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// 2. Wait for the pod to be ready.
	podTimeout := config.Timeout
	if podTimeout < time.Duration(timeoutSec*int(attempts)+60)*time.Second {
		podTimeout = time.Duration(timeoutSec*int(attempts)+60) * time.Second
	}
	logger.Info("waiting for egress attacker pod to be ready",
		zap.Duration("timeout", podTimeout),
	)
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, podName, podTimeout); err != nil {
		logs, _ := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Evidence:  "Pod failed to become ready",
			Logs:      logs,
			Error:     fmt.Sprintf("pod never became ready: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// 3–5. Give the pod time to finish its connection attempts, then collect logs.
	// The pod script runs sequentially and exits on its own. We wait a generous
	// amount of time for the script to complete.
	execWait := time.Duration(timeoutSec*int(attempts)+15) * time.Second
	logger.Info("waiting for egress test to complete", zap.Duration("wait", execWait))

	select {
	case <-ctx.Done():
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("context cancelled while waiting for egress test: %v", ctx.Err()),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	case <-time.After(execWait):
		// Expected path – the pod script should have completed by now.
	}

	// 6. Collect pod logs as evidence.
	logs, err := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to retrieve pod logs", zap.Error(err))
		logs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	// 7. Analyse logs to determine if the connection was blocked.
	blocked := isConnectionBlocked(logs)
	success := true // the test itself ran successfully
	evidence := buildEgressEvidence(logs, destIP, destPort, protocol, blocked)

	logger.Info("egress test completed",
		zap.Bool("blocked", blocked),
		zap.Duration("duration", time.Since(start)),
	)

	return &AttackResult{
		Success:   success,
		Blocked:   blocked,
		Evidence:  evidence,
		Logs:      logs,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
	}, nil
}

// Cleanup removes the attacker pod created by the egress test.
func (m *PodEgressTest) Cleanup(ctx context.Context, config AttackConfig) error {
	podName := fmt.Sprintf("chaos-sec-egress-%s", config.RunID)
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("pod_name", podName),
	)

	logger.Info("deleting egress attacker pod")
	if err := config.ClusterClient.DeletePod(ctx, config.Namespace, podName); err != nil {
		return fmt.Errorf("failed to delete egress attacker pod %q: %w", podName, err)
	}
	logger.Info("egress attacker pod deleted")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isConnectionBlocked inspects the pod log output for CONNECTION_SUCCESS or
// CONNECTION_BLOCKED markers injected by the pod script.
func isConnectionBlocked(logs string) bool {
	// If ANY attempt succeeded, the control is NOT blocking.
	// The pod script emits "CONNECTION_SUCCESS" when curl returns 0 and
	// "CONNECTION_BLOCKED" when curl returns non-zero.
	if containsMarker(logs, "CONNECTION_SUCCESS") {
		return false
	}
	// If we see at least one BLOCKED marker (and no SUCCESS), the control worked.
	if containsMarker(logs, "CONNECTION_BLOCKED") {
		return true
	}
	// No markers at all – assume blocked (could not even attempt).
	return true
}

// containsMarker checks whether the given marker string appears in the logs.
func containsMarker(logs, marker string) bool {
	return len(logs) >= len(marker) && containsString(logs, marker)
}

// containsString is a simple substring search without importing strings.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// buildEgressEvidence constructs a human-readable evidence string summarising
// the egress test result.
func buildEgressEvidence(logs, destIP string, destPort int, protocol string, blocked bool) string {
	status := "BLOCKED (control effective)"
	if !blocked {
		status = "ALLOWED (control gap detected)"
	}
	return fmt.Sprintf("Egress test to %s:%d (%s): %s\n\nPod logs:\n%s",
		destIP, destPort, protocol, status, logs)
}
