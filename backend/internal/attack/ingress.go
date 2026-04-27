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
// Pod Ingress Test – Attack Module
// ---------------------------------------------------------------------------

// PodIngressTest tests whether ingress (inbound) network policies block
// unauthorized connections by deploying a target service and then attempting
// to reach it from an attacker pod in a different (source) namespace.
type PodIngressTest struct{}

// NewPodIngressTest creates a new PodIngressTest module.
func NewPodIngressTest() *PodIngressTest {
	return &PodIngressTest{}
}

func (m *PodIngressTest) ID() string       { return "pod-ingress-test" }
func (m *PodIngressTest) Name() string     { return "Pod Ingress Test" }
func (m *PodIngressTest) Category() string { return "network" }
func (m *PodIngressTest) Severity() string { return "medium" }
func (m *PodIngressTest) Description() string {
	return "Tests whether ingress (inbound) network policies block unauthorized " +
		"connections by deploying a target service in the target namespace and " +
		"attempting to reach it from an attacker pod in a source namespace. " +
		"If the connection is blocked, the ingress network policy control is " +
		"working as expected."
}

func (m *PodIngressTest) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "target_namespace",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Namespace where the target service will be deployed",
		},
		{
			Name:        "target_service",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Name for the target service to deploy and test against",
		},
		{
			Name:        "target_port",
			Type:        ParamTypeInt,
			Required:    true,
			Default:     80,
			Description: "Port on which the target service listens",
		},
		{
			Name:        "source_namespace",
			Type:        ParamTypeString,
			Required:    false,
			Default:     "default",
			Description: "Namespace from which to launch the attacker pod",
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

// ---------------------------------------------------------------------------
// Manifest templates
// ---------------------------------------------------------------------------

// ingressTargetPodData holds the data injected into the target pod manifest.
type ingressTargetPodData struct {
	RunID        string
	ExperimentID string
	Namespace    string
	ServiceName  string
	Port         int
}

// ingressTargetPodTmpl is the Kubernetes pod manifest for the target server.
const ingressTargetPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-ingress-target-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-ingress-target
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: pod-ingress-test
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
      echo "Ingress target server listening on port {{.Port}}..."
      while true; do
        echo -e "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nchaos-sec-ingress-target" | nc -l -p {{.Port}} 2>/dev/null || true
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// ingressServiceTmpl is the Kubernetes Service manifest that exposes the target.
const ingressServiceTmpl = `apiVersion: v1
kind: Service
metadata:
  name: {{.ServiceName}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-ingress-target
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: pod-ingress-test
spec:
  selector:
    app: chaos-sec-ingress-target
    chaos-sec.io/run-id: "{{.RunID}}"
  ports:
  - port: {{.Port}}
    targetPort: {{.Port}}
    protocol: TCP
  type: ClusterIP
`

// ingressAttackerPodData holds the data for the attacker pod manifest.
type ingressAttackerPodData struct {
	RunID           string
	ExperimentID    string
	SourceNamespace string
	TargetService   string
	TargetNamespace string
	Port            int
	TimeoutSeconds  int
	Attempts        int
}

// ingressAttackerPodTmpl is the Kubernetes pod manifest for the attacker pod.
const ingressAttackerPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-ingress-attacker-{{.RunID}}
  namespace: {{.SourceNamespace}}
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: pod-ingress-test
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
      TARGET="{{.TargetService}}.{{.TargetNamespace}}.svc.cluster.local:{{.Port}}"
      echo "Starting ingress test to $TARGET..."
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        if curl -s -o /dev/null -w "%{http_code}" --connect-timeout {{.TimeoutSeconds}} --max-time {{.TimeoutSeconds}} "http://$TARGET"; then
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
// Validate
// ---------------------------------------------------------------------------

// Validate checks that the attack configuration contains all required parameters.
func (m *PodIngressTest) Validate(_ context.Context, config AttackConfig) error {
	if config.Namespace == "" {
		return fmt.Errorf("target_namespace is required")
	}
	if _, ok := config.ParamString("target_service"); !ok {
		return fmt.Errorf("target_service parameter is required")
	}
	if _, ok := config.ParamInt("target_port"); !ok {
		return fmt.Errorf("target_port parameter is required")
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

// Execute runs the ingress test by deploying a target pod + service, deploying
// an attacker pod in the source namespace, and checking whether the attacker
// can reach the target across namespaces.
func (m *PodIngressTest) Execute(ctx context.Context, config AttackConfig) (*AttackResult, error) {
	start := time.Now()
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("run_id", config.RunID),
	)

	// Apply defaults for optional parameters.
	params := ApplyDefaults(m, config.Parameters)

	targetService, _ := params["target_service"].(string)
	targetPort, _ := toInt(params["target_port"])
	if targetPort == 0 {
		targetPort = 80
	}
	sourceNamespace, _ := params["source_namespace"].(string)
	if sourceNamespace == "" {
		sourceNamespace = "default"
	}
	timeoutSec, _ := toInt(params["timeout_seconds"])
	if timeoutSec == 0 {
		timeoutSec = 10
	}
	attempts, _ := toInt(params["attempts"])
	if attempts == 0 {
		attempts = 3
	}

	targetPodName := fmt.Sprintf("chaos-sec-ingress-target-%s", config.RunID)
	attackerPodName := fmt.Sprintf("chaos-sec-ingress-attacker-%s", config.RunID)

	// Ensure cleanup runs when we exit.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cleanupConfig := config

		logger.Info("cleaning up ingress test resources")

		// Delete attacker pod.
		if err := config.ClusterClient.DeletePod(cleanupCtx, sourceNamespace, attackerPodName); err != nil {
			logger.Warn("failed to delete attacker pod during cleanup", zap.Error(err))
		}
		// Delete target pod.
		if err := config.ClusterClient.DeletePod(cleanupCtx, config.Namespace, targetPodName); err != nil {
			logger.Warn("failed to delete target pod during cleanup", zap.Error(err))
		}
		// Delete service.
		_ = cleanupConfig
		if err := config.ClusterClient.DeleteService(cleanupCtx, config.Namespace, targetService); err != nil {
			logger.Warn("failed to delete service during cleanup", zap.Error(err))
		}

		logger.Info("ingress test cleanup complete")
	}()

	// -----------------------------------------------------------------------
	// 1. Deploy target pod in target namespace.
	// -----------------------------------------------------------------------
	targetTmpl, err := template.New("ingress-target-pod").Parse(ingressTargetPodTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target pod template: %w", err)
	}

	targetData := ingressTargetPodData{
		RunID:        config.RunID,
		ExperimentID: config.ExperimentID,
		Namespace:    config.Namespace,
		ServiceName:  targetService,
		Port:         targetPort,
	}

	var targetBuf bytes.Buffer
	if err := targetTmpl.Execute(&targetBuf, targetData); err != nil {
		return nil, fmt.Errorf("failed to render target pod manifest: %w", err)
	}

	logger.Info("creating ingress target pod",
		zap.String("namespace", config.Namespace),
		zap.String("pod_name", targetPodName),
	)
	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, targetBuf.Bytes()); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("failed to create target pod: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// -----------------------------------------------------------------------
	// 2. Deploy target service.
	// -----------------------------------------------------------------------
	svcTmpl, err := template.New("ingress-service").Parse(ingressServiceTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service template: %w", err)
	}

	svcData := ingressTargetPodData{
		RunID:        config.RunID,
		ExperimentID: config.ExperimentID,
		Namespace:    config.Namespace,
		ServiceName:  targetService,
		Port:         targetPort,
	}

	var svcBuf bytes.Buffer
	if err := svcTmpl.Execute(&svcBuf, svcData); err != nil {
		return nil, fmt.Errorf("failed to render service manifest: %w", err)
	}

	logger.Info("creating ingress target service",
		zap.String("namespace", config.Namespace),
		zap.String("service_name", targetService),
	)
	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, svcBuf.Bytes()); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("failed to create target service: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// Wait for the target pod to become ready before launching the attacker.
	podTimeout := config.Timeout
	logger.Info("waiting for ingress target pod to be ready",
		zap.Duration("timeout", podTimeout),
	)
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, targetPodName, podTimeout); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Evidence:  "Target pod failed to become ready",
			Error:     fmt.Sprintf("target pod never became ready: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// -----------------------------------------------------------------------
	// 3. Deploy attacker pod in source namespace.
	// -----------------------------------------------------------------------
	attackerTmpl, err := template.New("ingress-attacker-pod").Parse(ingressAttackerPodTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse attacker pod template: %w", err)
	}

	attackerData := ingressAttackerPodData{
		RunID:           config.RunID,
		ExperimentID:    config.ExperimentID,
		SourceNamespace: sourceNamespace,
		TargetService:   targetService,
		TargetNamespace: config.Namespace,
		Port:            targetPort,
		TimeoutSeconds:  timeoutSec,
		Attempts:        attempts,
	}

	var attackerBuf bytes.Buffer
	if err := attackerTmpl.Execute(&attackerBuf, attackerData); err != nil {
		return nil, fmt.Errorf("failed to render attacker pod manifest: %w", err)
	}

	logger.Info("creating ingress attacker pod",
		zap.String("namespace", sourceNamespace),
		zap.String("pod_name", attackerPodName),
	)
	if err := config.ClusterClient.ApplyManifest(ctx, sourceNamespace, attackerBuf.Bytes()); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("failed to create attacker pod: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// Wait for the attacker pod to be ready.
	logger.Info("waiting for ingress attacker pod to be ready",
		zap.Duration("timeout", podTimeout),
	)
	if err := config.ClusterClient.WaitForPodReady(ctx, sourceNamespace, attackerPodName, podTimeout); err != nil {
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Evidence:  "Attacker pod failed to become ready",
			Error:     fmt.Sprintf("attacker pod never became ready: %v", err),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	}

	// -----------------------------------------------------------------------
	// 4. Wait for the attack script to complete, then collect logs.
	// -----------------------------------------------------------------------
	execWait := time.Duration(timeoutSec*attempts+30) * time.Second
	logger.Info("waiting for ingress test to complete", zap.Duration("wait", execWait))

	select {
	case <-ctx.Done():
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("context cancelled while waiting for ingress test: %v", ctx.Err()),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	case <-time.After(execWait):
		// Expected path – the pod script should have completed by now.
	}

	// -----------------------------------------------------------------------
	// 5. Collect logs and determine result.
	// -----------------------------------------------------------------------
	attackerLogs, err := config.ClusterClient.GetPodLogs(ctx, sourceNamespace, attackerPodName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to retrieve attacker pod logs", zap.Error(err))
		attackerLogs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	blocked := isConnectionBlocked(attackerLogs)
	evidence := buildIngressEvidence(
		attackerLogs, config.Namespace, sourceNamespace,
		targetService, targetPort, blocked,
	)

	logger.Info("ingress test completed",
		zap.Bool("blocked", blocked),
		zap.Duration("duration", time.Since(start)),
	)

	return &AttackResult{
		Success:   true,
		Blocked:   blocked,
		Evidence:  evidence,
		Logs:      attackerLogs,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
	}, nil
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// Cleanup removes all resources created by the ingress test. The Execute method
// already uses a deferred cleanup, but this method allows external callers
// (e.g. the engine) to trigger cleanup explicitly.
func (m *PodIngressTest) Cleanup(ctx context.Context, config AttackConfig) error {
	params := ApplyDefaults(m, config.Parameters)

	targetService, _ := params["target_service"].(string)
	sourceNamespace, _ := params["source_namespace"].(string)
	if sourceNamespace == "" {
		sourceNamespace = "default"
	}

	targetPodName := fmt.Sprintf("chaos-sec-ingress-target-%s", config.RunID)
	attackerPodName := fmt.Sprintf("chaos-sec-ingress-attacker-%s", config.RunID)

	logger := config.Logger.With(zap.String("module", m.ID()))

	var firstErr error

	// Delete attacker pod.
	if err := config.ClusterClient.DeletePod(ctx, sourceNamespace, attackerPodName); err != nil {
		logger.Warn("failed to delete attacker pod", zap.Error(err))
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete attacker pod %q: %w", attackerPodName, err)
		}
	}

	// Delete target pod.
	if err := config.ClusterClient.DeletePod(ctx, config.Namespace, targetPodName); err != nil {
		logger.Warn("failed to delete target pod", zap.Error(err))
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete target pod %q: %w", targetPodName, err)
		}
	}

	// Delete service.
	if err := config.ClusterClient.DeleteService(ctx, config.Namespace, targetService); err != nil {
		logger.Warn("failed to delete service", zap.Error(err))
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete service %q: %w", targetService, err)
		}
	}

	return firstErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildIngressEvidence constructs a human-readable evidence string summarising
// the ingress test result.
func buildIngressEvidence(logs, targetNS, sourceNS, serviceName string, port int, blocked bool) string {
	status := "BLOCKED (control effective)"
	if !blocked {
		status = "ALLOWED (control gap detected)"
	}
	return fmt.Sprintf(
		"Ingress test from %s to %s.%s.svc.cluster.local:%d: %s\n\nAttacker pod logs:\n%s",
		sourceNS, serviceName, targetNS, port, status, logs,
	)
}
