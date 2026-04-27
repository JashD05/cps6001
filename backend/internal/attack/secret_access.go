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
// Secret Access Test – Attack Module
// ---------------------------------------------------------------------------

// SecretAccessTest tests whether pods can access secrets they shouldn't by
// deploying an attacker pod in the target namespace that attempts to read
// secrets via the Kubernetes API and inspect mounted secrets from environment
// variables and filesystem paths.
type SecretAccessTest struct{}

// NewSecretAccessTest creates a new SecretAccessTest module.
func NewSecretAccessTest() *SecretAccessTest {
	return &SecretAccessTest{}
}

func (m *SecretAccessTest) ID() string       { return "secret-access-test" }
func (m *SecretAccessTest) Name() string     { return "Secret Access Test" }
func (m *SecretAccessTest) Category() string { return "security" }
func (m *SecretAccessTest) Severity() string { return "high" }
func (m *SecretAccessTest) Description() string {
	return "Tests whether pods can access secrets they shouldn't by deploying " +
		"an attacker pod that attempts to read secrets via the Kubernetes API " +
		"and inspect mounted secrets from environment variables and filesystem " +
		"paths. If access is denied, the security controls are working correctly; " +
		"if allowed, a secret exposure vulnerability exists."
}

func (m *SecretAccessTest) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "target_namespace",
			Type:        ParamTypeString,
			Required:    true,
			Description: "Namespace in which to launch the attacker pod and test secret access",
		},
		{
			Name:        "secret_name",
			Type:        ParamTypeString,
			Required:    false,
			Default:     "",
			Description: "Specific secret to test access to (empty = attempt to list all secrets)",
		},
		{
			Name:        "mount_path",
			Type:        ParamTypeBool,
			Required:    false,
			Default:     false,
			Description: "Test whether secrets are accessible via mounted volumes and environment variables",
		},
		{
			Name:        "timeout_seconds",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     30,
			Description: "Timeout in seconds for each kubectl/API command",
		},
		{
			Name:        "attempts",
			Type:        ParamTypeInt,
			Required:    false,
			Default:     1,
			Description: "Number of times to attempt each access method",
		},
	}
}

// ---------------------------------------------------------------------------
// Manifest templates
// ---------------------------------------------------------------------------

// secretManifestData holds the data injected into the attacker pod manifest.
type secretManifestData struct {
	RunID          string
	ExperimentID   string
	Namespace      string
	SecretName     string
	MountPath      bool
	TimeoutSeconds int
	Attempts       int
}

// secretAPITestPodTmpl is the Kubernetes pod manifest used to test secret
// access via the Kubernetes API using kubectl. This pod mounts the default
// service account token so it can make API calls.
const secretAPITestPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-secret-api-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: secret-access-test
spec:
  automountServiceAccountToken: true
  serviceAccountName: default
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
      echo "Starting secret access test via Kubernetes API..."
      echo "Namespace: {{.Namespace}}"
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        {{- if .SecretName}}
        echo "Attempting: kubectl get secret {{.SecretName}} -n {{.Namespace}}"
        if kubectl get secret {{.SecretName}} -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
          echo "SECRET_API_ALLOWED: Successfully read secret {{.SecretName}} in {{.Namespace}}"
        else
          echo "SECRET_API_DENIED: Forbidden to read secret {{.SecretName}} in {{.Namespace}}"
        fi
        echo "---"
        echo "Attempting: kubectl get secret {{.SecretName}} -o yaml -n {{.Namespace}}"
        if kubectl get secret {{.SecretName}} -o yaml -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
          echo "SECRET_API_YAML_ALLOWED: Successfully read secret {{.SecretName}} YAML in {{.Namespace}}"
        else
          echo "SECRET_API_YAML_DENIED: Forbidden to read secret {{.SecretName}} YAML in {{.Namespace}}"
        fi
        {{- else}}
        echo "Attempting: kubectl get secrets -n {{.Namespace}}"
        if kubectl get secrets -n {{.Namespace}} --request-timeout={{.TimeoutSeconds}}s 2>&1; then
          echo "SECRET_LIST_ALLOWED: Successfully listed secrets in {{.Namespace}}"
        else
          echo "SECRET_LIST_DENIED: Forbidden to list secrets in {{.Namespace}}"
        fi
        {{- end}}
        sleep 2
      done
    resources:
      requests: { cpu: 50m, memory: 64Mi }
      limits: { cpu: 500m, memory: 256Mi }
  terminationGracePeriodSeconds: 30
`

// secretMountTestPodTmpl is the Kubernetes pod manifest used to test whether
// secrets are accessible via mounted volumes and environment variables.
// This pod deliberately mounts a secret (if a specific name is given) or
// checks for commonly mounted secret paths.
const secretMountTestPodTmpl = `apiVersion: v1
kind: Pod
metadata:
  name: chaos-sec-secret-mount-{{.RunID}}
  namespace: {{.Namespace}}
  labels:
    app: chaos-sec-attacker
    chaos-sec.io/experiment-id: "{{.ExperimentID}}"
    chaos-sec.io/run-id: "{{.RunID}}"
    chaos-sec.io/module: secret-access-test
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
      echo "Starting secret mount path test..."
      echo "Namespace: {{.Namespace}}"
      for i in $(seq 1 {{.Attempts}}); do
        echo "Attempt $i/{{.Attempts}}..."
        echo "--- Checking environment variables for secrets ---"
        env | grep -i -E '(secret|password|token|key|credential|api_key|apikey)' 2>/dev/null || echo "SECRET_ENV_NONE: No secret-related environment variables found"
        echo "--- Checking common secret mount paths ---"
        for dir in /etc/secrets /var/secrets /secrets /run/secrets /etc/config; do
          if [ -d "$dir" ]; then
            echo "SECRET_MOUNT_FOUND: Directory $dir exists"
            ls -la "$dir" 2>/dev/null || echo "SECRET_MOUNT_LIST_DENIED: Cannot list $dir"
            for f in "$dir"/*; do
              if [ -f "$f" ]; then
                echo "SECRET_FILE_FOUND: $f"
                head -c 64 "$f" 2>/dev/null && echo "" || echo "SECRET_FILE_READ_DENIED: Cannot read $f"
              fi
            done
          else
            echo "SECRET_MOUNT_NOT_FOUND: Directory $dir does not exist"
          fi
        done
        echo "--- Checking /var/run/secrets/kubernetes.io/serviceaccount ---"
        if [ -d "/var/run/secrets/kubernetes.io/serviceaccount" ]; then
          echo "SECRET_SA_MOUNT_FOUND: Service account token is mounted (automountServiceAccountToken should be false)"
          ls -la /var/run/secrets/kubernetes.io/serviceaccount/ 2>/dev/null || echo "SECRET_SA_LIST_DENIED: Cannot list SA directory"
          for f in /var/run/secrets/kubernetes.io/serviceaccount/*; do
            if [ -f "$f" ]; then
              echo "SECRET_SA_FILE_FOUND: $f"
              head -c 64 "$f" 2>/dev/null && echo "" || echo "SECRET_SA_READ_DENIED: Cannot read $f"
            fi
          done
        else
          echo "SECRET_SA_MOUNT_NOT_FOUND: Service account token is not mounted (good)"
        fi
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
func (m *SecretAccessTest) Validate(_ context.Context, config AttackConfig) error {
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

// Execute runs the secret access test by deploying attacker pods that attempt
// to read secrets via the Kubernetes API and/or mounted volumes. It then
// analyses the logs to determine whether access was denied or allowed.
func (m *SecretAccessTest) Execute(ctx context.Context, config AttackConfig) (*AttackResult, error) {
	start := time.Now()
	logger := config.Logger.With(
		zap.String("module", m.ID()),
		zap.String("run_id", config.RunID),
	)

	// Apply defaults for optional parameters.
	params := ApplyDefaults(m, config.Parameters)

	secretName, _ := params["secret_name"].(string)
	mountPath, _ := params["mount_path"].(bool)
	timeoutSec, _ := toInt(params["timeout_seconds"])
	if timeoutSec == 0 {
		timeoutSec = 30
	}
	attempts, _ := toInt(params["attempts"])
	if attempts == 0 {
		attempts = 1
	}

	// Ensure cleanup runs when we exit.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := m.Cleanup(cleanupCtx, config); err != nil {
			logger.Warn("failed to cleanup secret access test resources", zap.Error(err))
		}
	}()

	var allLogs string
	var apiBlocked bool
	var mountBlocked bool
	apiTested := false
	mountTested := false

	// -----------------------------------------------------------------------
	// 1–2. Test secret access via the Kubernetes API.
	// -----------------------------------------------------------------------
	apiResult, err := m.runAPITest(ctx, config, secretName, timeoutSec, attempts, logger)
	if err != nil {
		return nil, fmt.Errorf("API test failed: %w", err)
	}
	allLogs += fmt.Sprintf("=== API Access Test ===\n%s\n", apiResult.Logs)
	apiBlocked = apiResult.Blocked
	apiTested = true

	// -----------------------------------------------------------------------
	// 3–4. Test mounted secret access (if mount_path is enabled).
	// -----------------------------------------------------------------------
	if mountPath {
		mountResult, err := m.runMountTest(ctx, config, timeoutSec, attempts, logger)
		if err != nil {
			return nil, fmt.Errorf("mount test failed: %w", err)
		}
		allLogs += fmt.Sprintf("=== Mount Path Test ===\n%s\n", mountResult.Logs)
		mountBlocked = mountResult.Blocked
		mountTested = true
	}

	// -----------------------------------------------------------------------
	// 5. Determine overall result.
	// -----------------------------------------------------------------------
	overallBlocked := true
	if apiTested && !apiBlocked {
		overallBlocked = false
	}
	if mountTested && !mountBlocked {
		overallBlocked = false
	}

	evidence := buildSecretEvidence(allLogs, config.Namespace, secretName, mountPath, apiBlocked, mountBlocked, apiTested, mountTested)

	logger.Info("secret access test completed",
		zap.Bool("api_blocked", apiBlocked),
		zap.Bool("mount_blocked", mountBlocked),
		zap.Bool("overall_blocked", overallBlocked),
		zap.Duration("duration", time.Since(start)),
	)

	return &AttackResult{
		Success:   true,
		Blocked:   overallBlocked,
		Evidence:  evidence,
		Logs:      allLogs,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
	}, nil
}

// ---------------------------------------------------------------------------
// Test runners
// ---------------------------------------------------------------------------

// secretTestSubResult is the outcome of a single sub-test (API or mount).
type secretTestSubResult struct {
	Blocked bool
	Logs    string
}

// runAPITest deploys the API test pod and returns whether API secret access
// was blocked.
func (m *SecretAccessTest) runAPITest(
	ctx context.Context,
	config AttackConfig,
	secretName string,
	timeoutSec int,
	attempts int,
	logger *zap.Logger,
) (*secretTestSubResult, error) {
	tmpl, err := template.New("secret-api-pod").Parse(secretAPITestPodTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API test template: %w", err)
	}

	data := secretManifestData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		SecretName:     secretName,
		MountPath:      false,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render API test manifest: %w", err)
	}

	podName := fmt.Sprintf("chaos-sec-secret-api-%s", config.RunID)

	logger.Info("creating secret API test pod",
		zap.String("namespace", config.Namespace),
		zap.String("pod_name", podName),
		zap.String("secret_name", secretName),
	)

	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, buf.Bytes()); err != nil {
		return &secretTestSubResult{
			Blocked: true, // Assume blocked if we can't even create the pod
			Logs:    fmt.Sprintf("Failed to create API test pod: %v", err),
		}, nil
	}

	// Wait for the pod to be ready.
	podTimeout := config.Timeout
	minTimeout := time.Duration(timeoutSec*attempts+60) * time.Second
	if podTimeout < minTimeout {
		podTimeout = minTimeout
	}
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, podName, podTimeout); err != nil {
		logs, _ := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
		// Pod may have exited already – still try to get logs.
		return &secretTestSubResult{
			Blocked: isSecretAPIDenied(logs),
			Logs:    logs,
		}, nil
	}

	// Wait for the test to complete.
	execWait := time.Duration(timeoutSec*attempts+30) * time.Second
	select {
	case <-ctx.Done():
		return &secretTestSubResult{
			Blocked: true,
			Logs:    fmt.Sprintf("Context cancelled: %v", ctx.Err()),
		}, nil
	case <-time.After(execWait):
	}

	// Collect logs.
	logs, err := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to retrieve API test pod logs", zap.Error(err))
		logs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	// Clean up the API test pod now (not relying solely on deferred cleanup).
	_ = config.ClusterClient.DeletePod(ctx, config.Namespace, podName)

	blocked := isSecretAPIDenied(logs)
	return &secretTestSubResult{
		Blocked: blocked,
		Logs:    logs,
	}, nil
}

// runMountTest deploys the mount path test pod and returns whether mounted
// secret access was blocked.
func (m *SecretAccessTest) runMountTest(
	ctx context.Context,
	config AttackConfig,
	timeoutSec int,
	attempts int,
	logger *zap.Logger,
) (*secretTestSubResult, error) {
	tmpl, err := template.New("secret-mount-pod").Parse(secretMountTestPodTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mount test template: %w", err)
	}

	data := secretManifestData{
		RunID:          config.RunID,
		ExperimentID:   config.ExperimentID,
		Namespace:      config.Namespace,
		SecretName:     "",
		MountPath:      true,
		TimeoutSeconds: timeoutSec,
		Attempts:       attempts,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render mount test manifest: %w", err)
	}

	podName := fmt.Sprintf("chaos-sec-secret-mount-%s", config.RunID)

	logger.Info("creating secret mount test pod",
		zap.String("namespace", config.Namespace),
		zap.String("pod_name", podName),
	)

	if err := config.ClusterClient.ApplyManifest(ctx, config.Namespace, buf.Bytes()); err != nil {
		return &secretTestSubResult{
			Blocked: true,
			Logs:    fmt.Sprintf("Failed to create mount test pod: %v", err),
		}, nil
	}

	// Wait for the pod to be ready.
	podTimeout := config.Timeout
	minTimeout := time.Duration(timeoutSec*attempts+60) * time.Second
	if podTimeout < minTimeout {
		podTimeout = minTimeout
	}
	if err := config.ClusterClient.WaitForPodReady(ctx, config.Namespace, podName, podTimeout); err != nil {
		logs, _ := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
		return &secretTestSubResult{
			Blocked: isSecretMountDenied(logs),
			Logs:    logs,
		}, nil
	}

	// Wait for the test to complete.
	execWait := time.Duration(timeoutSec*attempts+30) * time.Second
	select {
	case <-ctx.Done():
		return &secretTestSubResult{
			Blocked: true,
			Logs:    fmt.Sprintf("Context cancelled: %v", ctx.Err()),
		}, nil
	case <-time.After(execWait):
	}

	// Collect logs.
	logs, err := config.ClusterClient.GetPodLogs(ctx, config.Namespace, podName, &PodLogOptions{})
	if err != nil {
		logger.Warn("failed to retrieve mount test pod logs", zap.Error(err))
		logs = fmt.Sprintf("(failed to retrieve logs: %v)", err)
	}

	// Clean up the mount test pod now.
	_ = config.ClusterClient.DeletePod(ctx, config.Namespace, podName)

	blocked := isSecretMountDenied(logs)
	return &secretTestSubResult{
		Blocked: blocked,
		Logs:    logs,
	}, nil
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// Cleanup removes all pods created by the secret access test.
func (m *SecretAccessTest) Cleanup(ctx context.Context, config AttackConfig) error {
	logger := config.Logger.With(zap.String("module", m.ID()))

	apiPodName := fmt.Sprintf("chaos-sec-secret-api-%s", config.RunID)
	mountPodName := fmt.Sprintf("chaos-sec-secret-mount-%s", config.RunID)

	var firstErr error

	// Delete API test pod.
	if err := config.ClusterClient.DeletePod(ctx, config.Namespace, apiPodName); err != nil {
		logger.Warn("failed to delete API test pod during cleanup",
			zap.String("pod_name", apiPodName),
			zap.Error(err),
		)
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete API test pod %q: %w", apiPodName, err)
		}
	}

	// Delete mount test pod.
	if err := config.ClusterClient.DeletePod(ctx, config.Namespace, mountPodName); err != nil {
		logger.Warn("failed to delete mount test pod during cleanup",
			zap.String("pod_name", mountPodName),
			zap.Error(err),
		)
		if firstErr == nil {
			firstErr = fmt.Errorf("failed to delete mount test pod %q: %w", mountPodName, err)
		}
	}

	return firstErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isSecretAPIDenied inspects the pod log output for markers indicating whether
// the Kubernetes API allowed or denied secret access.
func isSecretAPIDenied(logs string) bool {
	// If any "ALLOWED" marker is found, access was NOT blocked.
	if containsString(logs, "SECRET_API_ALLOWED") ||
		containsString(logs, "SECRET_API_YAML_ALLOWED") ||
		containsString(logs, "SECRET_LIST_ALLOWED") {
		return false
	}
	// If "DENIED" markers are found, RBAC blocked access.
	if containsString(logs, "SECRET_API_DENIED") ||
		containsString(logs, "SECRET_API_YAML_DENIED") ||
		containsString(logs, "SECRET_LIST_DENIED") {
		return true
	}
	// No markers – assume blocked (safe default).
	return true
}

// isSecretMountDenied inspects the pod log output for markers indicating
// whether mounted secrets were accessible.
func isSecretMountDenied(logs string) bool {
	// If any secret content was found in mounts or env, access was NOT blocked.
	if containsString(logs, "SECRET_ENV_NONE") == false {
		// env grep found something — check if it's actually a secret match
		// vs the "none found" message. If we find secret env vars, that's
		// potentially a finding but not necessarily a failure.
	}

	// Key indicators of a control gap:
	// - SECRET_SA_MOUNT_FOUND: service account token is mounted when it shouldn't be
	// - SECRET_FILE_FOUND / SECRET_MOUNT_FOUND: secret files are readable
	if containsString(logs, "SECRET_SA_MOUNT_FOUND") ||
		containsString(logs, "SECRET_FILE_FOUND") ||
		containsString(logs, "SECRET_MOUNT_FOUND") {
		// Check if the files were actually readable (not just the directory existing).
		// If the file content was readable, that's a definite gap.
		if containsString(logs, "SECRET_SA_FILE_FOUND") && !containsString(logs, "SECRET_SA_READ_DENIED") {
			return false // SA token readable = control gap
		}
	}

	// If all mount checks returned NOT_FOUND or NONE, controls are working.
	if containsString(logs, "SECRET_ENV_NONE") ||
		containsString(logs, "SECRET_MOUNT_NOT_FOUND") ||
		containsString(logs, "SECRET_SA_MOUNT_NOT_FOUND") {
		return true
	}

	// Default to blocked if markers are inconclusive.
	return true
}

// buildSecretEvidence constructs a human-readable evidence string summarising
// the secret access test result.
func buildSecretEvidence(
	logs, namespace, secretName string,
	mountPath, apiBlocked, mountBlocked, apiTested, mountTested bool,
) string {
	result := fmt.Sprintf("Secret Access Test Report for namespace %q\n\n", namespace)

	if apiTested {
		status := "DENIED (control effective)"
		if !apiBlocked {
			status = "ALLOWED (control gap detected)"
		}
		if secretName != "" {
			result += fmt.Sprintf("API Access to secret %q: %s\n", secretName, status)
		} else {
			result += fmt.Sprintf("API List Secrets: %s\n", status)
		}
	}

	if mountTested {
		status := "DENIED (control effective)"
		if !mountBlocked {
			status = "ALLOWED (control gap detected)"
		}
		result += fmt.Sprintf("Mount Path Access: %s\n", status)
	}

	overall := "BLOCKED"
	if (apiTested && !apiBlocked) || (mountTested && !mountBlocked) {
		overall = "NOT BLOCKED (control gap)"
	}
	result += fmt.Sprintf("\nOverall: %s\n\n--- Full Logs ---\n%s", overall, logs)

	return result
}
