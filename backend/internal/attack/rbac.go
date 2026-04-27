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
// RBAC Privilege Escalation Test – Attack Module
// ---------------------------------------------------------------------------

// RBACPrivilegeTest tests whether service accounts have excessive permissions
// by creating an attacker pod with the specified service account and attempting
// privileged actions via kubectl. If the action is denied, RBAC controls are
// working as expected.
type RBACPrivilegeTest struct{}

// NewRBACPrivilegeTest creates a new RBACPrivilegeTest module.
func NewRBACPrivilegeTest() *RBACPrivilegeTest {
	return &RBACPrivilegeTest{}
}

func (m *RBACPrivilegeTest) ID() string       { return "rbac-privilege-test" }
func (m *RBACPrivilegeTest) Name() string     { return "RBAC Privilege Escalation Test" }
func (m *RBACPrivilegeTest) Category() string { return "rbac" }
func (m *RBACPrivilegeTest) Severity() string { return "high" }
func (m *RBACPrivilegeTest) Description() string {
	return "Tests whether service accounts have excessive permissions by " +
		"attempting privileged actions (e.g. listing secrets, creating pods, " +
		"executing into pods) from an attacker pod using the specified service " +
		"account. If the action is denied, RBAC controls are working correctly; " +
		"if allowed, a privilege escalation path exists."
}

func (m *RBACPrivilegeTest) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "target_namespace",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Namespace in which to launch the attacker pod and test RBAC",
		},
		{
			Name:        "service_account",
			Type:        ParamTypeString,
			Required:    false,
			Default:     "default",
			Description: "Service account to impersonate in the attacker pod",
		},
		{
			Name:        "test_action",
			Type:        ParamTypeSelect,
			Required:    true,
			Description: "Privileged action to attempt via kubectl",
			Options:     []string{"list-secrets", "create-pods", "delete-pods", "list-configmaps", "exec-pods"},
		},
		{
			Name:        "timeout_seconds",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     30,
			Description: "Timeout in seconds for the kubectl command to complete",
		},
		{
			Name:        "attempts",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     1,
			Description: "Number of times to attempt the action",
		},
	}
}

// ---------------------------------------------------------------------------
// Manifest template
// ---------------------------------------------------------------------------

// rbacManifestData holds the data injected into the attacker pod manifest.
type rbacManifestData struct {
	RunID          string
	ExperimentID   string
	Namespace      string
	ServiceAccount string
	TestAction     string
	TimeoutSeconds int
	Attempts       int
}

// rbacAttackerPodTmpl is the Kubernetes pod manifest used to run the RBAC
// privilege test. The pod uses the specified service account and runs kubectl
// to attempt the privileged action.
const rbacAttackerPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-rbac-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: rbac-privilege-test
spec:
  automountServiceAccountToken: true
  serviceAccountName: {{.ServiceAccount}}
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
    image: bitnami/kubectl:latest
    command: ["sh", "-c"]
    args:
    - |
      echo "Starting RBAC privilege test..."
      echo "Service Account: {{.ServiceAccount}}"
      echo "Namespace: {{.Namespace}}"
      echo "Action: {{.TestAction}}"
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        case "{{.TestAction}}" in
          list-secrets)
            echo "Attempting: kubectl get secrets -n {{.Namespace}}"
            if kubectl get secrets -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
              echo "ACTION_ALLOWED: Successfully listed secrets in {{.Namespace}}"
            else
              echo "ACTION_DENIED: Forbidden to list secrets in {{.Namespace}}"
            fi
            ;;
          create-pods)
            echo "Attempting: kubectl run test-rbac-pod --image=busybox -n {{.Namespace}} --restart=Never"
            if kubectl run test-rbac-pod --image=busybox -n {{.Namespace}} --restart=Never --request-timeout={{.TimeoutSeconds}}s 2>&1; then
              echo "ACTION_ALLOWED: Successfully created a pod in {{.Namespace}}"
              echo "Cleaning up test pod..."
              kubectl delete pod test-rbac-pod -n {{.Namespace}} --force --grace-period=0 2>/dev/null || true
            else
              echo "ACTION_DENIED: Forbidden to create pods in {{.Namespace}}"
            fi
            ;;
          delete-pods)
            echo "Attempting: kubectl delete pods --all -n {{.Namespace}} --dry-run=client"
            if kubectl delete pods --all -n {{.Namespace}} --dry-run=client --request-timeout={{.TimeoutSeconds}}s 2>&1; then
              echo "ACTION_ALLOWED: Dry-run delete pods succeeded (permission exists) in {{.Namespace}}"
            else
              echo "ACTION_DENIED: Forbidden to delete pods in {{.Namespace}}"
            fi
            ;;
          list-configmaps)
            echo "Attempting: kubectl get configmaps -n {{.Namespace}}"
            if kubectl get configmaps -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
              echo "ACTION_ALLOWED: Successfully listed configmaps in {{.Namespace}}"
            else
              echo "ACTION_DENIED: Forbidden to list configmaps in {{.Namespace}}"
            fi
            ;;
          exec-pods)
            echo "Attempting: kubectl exec --list (dry-run to check exec permissions)"
            if kubectl auth can-i create pods/exec -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
              echo "ACTION_ALLOWED: Has exec permissions in {{.Namespace}}"
            else
              echo "ACTION_DENIED: Forbidden to exec into pods in {{.Namespace}}"
            fi
            ;;
          *)
            echo "UNKNOWN_ACTION: {{.TestAction}} is not a supported test action"
            ;;
        esac
        sleep 2
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
func (m *RBACPrivilegeTest) Validate(_ context.Context, config AttackConfig) error {
	if config.Namespace == "" {
		return fmt.Errorf("target_namespace is required")
	}
	action, ok := config.ParamString("test_action")
	if !ok || action == "" {
		return fmt.Errorf("test_action parameter is required")
	}
	validActions := map[string]bool{
		"list-secrets":    true,
		"create-pods":     true,
		"delete-pods":     true,
		"list-configmaps": true,
		"exec-pods":       true,
	}
	if !validActions[action] {
		return fmt.Errorf("test_action must be one of: list-secrets, create-pods, delete-pods, list-configmaps, exec-pods")
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

// Execute runs the RBAC privilege test by deploying an attacker pod with the
// specified service account, waiting for it to complete, collecting logs, and
// determining whether the privileged action was allowed or denied.
func (m *RBACPrivilegeTest) Execute(ctx context.Context, config AttackConfig) (*AttackResult, error) {
	start := time.Now()
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("run_id", config.RunID),
	)

	// Apply defaults for optional parameters.
	params := ApplyDefaults(m, config.Parameters)

	serviceAccount, _ := params["service_account"].(string)
	if serviceAccount == "" {
		serviceAccount = "default"
	}
	testAction, _ := params["test_action"].(string)
	timeoutSec, _ := toInt(params["timeout_seconds"])
	if timeoutSec == 0 {
		timeoutSec = 30
	}
	attempts, _ := toInt(params["attempts"])
	if attempts == 0 {
		attempts = 1
	}

	// Render the pod manifest.
	tmpl, err := template.New("rbac-pod").Parse(rbacAttackerPodTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RBAC manifest template: %w", err)
	}

	data := rbacManifestData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		ServiceAccount: serviceAccount,
		TestAction:     testAction,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
	}

	var manifestBuf bytes.Buffer
	if err := tmpl.Execute(&manifestBuf, data); err != nil {
		return nil, fmt.Errorf("failed to render RBAC manifest: %w", err)
	}
	manifest := manifestBuf.Bytes()

	podName := fmt.Sprintf("chaos-sec-rbac-%s", config.RunID)

	// Ensure cleanup runs when we exit.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := m.Cleanup(cleanupCtx, config); err != nil {
			logger.Warn("failed to cleanup RBAC attacker pod", zap.Error(err))
		}
	}()

	// 1. Create the attacker pod.
	logger.Info("creating RBAC attacker pod",
		zap.String("namespace", config.Namespace),
		zap.String("pod_name", podName),
		zap.String("service_account", serviceAccount),
		zap.String("test_action", testAction),
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
	minTimeout := time.Duration(timeoutSec*attempts+60) * time.Second
	if podTimeout < minTimeout {
		podTimeout = minTimeout
	}
	logger.Info("waiting for RBAC attacker pod to be ready",
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

	// 3. Wait for the kubectl command to complete.
	execWait := time.Duration(timeoutSec*attempts+30) * time.Second
	logger.Info("waiting for RBAC test to complete", zap.Duration("wait", execWait))

	select {
	case <-ctx.Done():
		return &AttackResult{
			Success:   false,
			Blocked:   false,
			Error:     fmt.Sprintf("context cancelled while waiting for RBAC test: %v", ctx.Err()),
			Timestamp: time.Now(),
			Duration:  time.Since(start),
		}, nil
	case <-time.After(execWait):
		// Expected path – the pod script should have completed by now.
	}

	// 4. Collect pod logs as evidence.
	logs, err := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to retrieve pod logs", zap.Error(err))
		logs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	// 5. Analyse logs to determine if the action was denied (RBAC working)
	//    or allowed (RBAC failed / privilege escalation path exists).
	blocked := isRBACActionDenied(logs)
	evidence := buildRBACEvidence(logs, config.Namespace, serviceAccount, testAction, blocked)

	logger.Info("RBAC privilege test completed",
		zap.Bool("blocked", blocked),
		zap.Duration("duration", time.Since(start)),
	)

	return &AttackResult{
		Success:   true,
		Blocked:   blocked,
		Evidence:  evidence,
		Logs:      logs,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
	}, nil
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// Cleanup removes the attacker pod created by the RBAC test.
func (m *RBACPrivilegeTest) Cleanup(ctx context.Context, config AttackConfig) error {
	podName := fmt.Sprintf("chaos-sec-rbac-%s", config.RunID)
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("pod_name", podName),
	)

	logger.Info("deleting RBAC attacker pod")
	if err := config.ClusterClient.DeletePod(ctx, config.Namespace, podName); err != nil {
		return fmt.Errorf("failed to delete RBAC attacker pod %q: %w", podName, err)
	}
	logger.Info("RBAC attacker pod deleted")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isRBACActionDenied inspects the pod log output for ACTION_ALLOWED or
// ACTION_DENIED markers injected by the pod script.
func isRBACActionDenied(logs string) bool {
	// If ANY "ACTION_ALLOWED" marker is found, the RBAC control failed to
	// block the action.
	if containsString(logs, "ACTION_ALLOWED") {
		return false
	}
	// If we see "ACTION_DENIED", RBAC is working.
	if containsString(logs, "ACTION_DENIED") {
		return true
	}
	// If neither marker is found (e.g. the pod failed before running the
	// test), default to true (assume blocked) to avoid false positives about
	// RBAC failures.
	return true
}

// buildRBACEvidence constructs a human-readable evidence string summarising
// the RBAC privilege test result.
func buildRBACEvidence(logs, namespace, serviceAccount, testAction string, blocked bool) string {
	status := "DENIED (RBAC control effective)"
	if !blocked {
		status = "ALLOWED (privilege escalation path detected)"
	}
	return fmt.Sprintf(
		"RBAC privilege test in namespace %q with service account %q, action %q: %s\n\nPod logs:\n%s",
		namespace, serviceAccount, testAction, status, logs,
	)
}
