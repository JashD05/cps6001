package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	watchapi "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/retry"
)

// AttackPodConfig holds the configuration for creating an attacker pod.
type AttackPodConfig struct {
	ExperimentID   string            `json:"experiment_id"`
	RunID          string            `json:"run_id"`
	TemplateID     string            `json:"template_id"`
	Namespace      string            `json:"namespace"`
	Image          string            `json:"image"`
	Command        []string          `json:"command"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	ResourceLimits *ResourceLimits   `json:"resource_limits,omitempty"`
}

// ResourceLimits specifies the resource constraints for a pod.
type ResourceLimits struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
}

// PodStatusInfo holds detailed status information about a pod.
type PodStatusInfo struct {
	Phase             string                   `json:"phase"`
	Conditions        []corev1.PodCondition    `json:"conditions,omitempty"`
	ContainerStatuses []corev1.ContainerStatus `json:"container_statuses,omitempty"`
	IP                string                   `json:"ip,omitempty"`
	NodeName          string                   `json:"node_name,omitempty"`
	StartTime         *metav1.Time             `json:"start_time,omitempty"`
	Message           string                   `json:"message,omitempty"`
	Reason            string                   `json:"reason,omitempty"`
}

// defaultResourceLimits returns the default resource limits for attacker pods.
func defaultResourceLimits() *ResourceLimits {
	return &ResourceLimits{
		CPURequest:    "100m",
		CPULimit:      "500m",
		MemoryRequest: "128Mi",
		MemoryLimit:   "512Mi",
	}
}

// PodController manages the lifecycle of attacker pods in a Kubernetes cluster.
// It provides methods for creating, monitoring, executing commands in,
// and cleaning up pods used for security experiments.
type PodController struct {
	client     kubernetes.Interface
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger
}

// NewPodController creates a new PodController for the given cluster client.
func NewPodController(client *ClusterClient) (*PodController, error) {
	if client == nil {
		return nil, fmt.Errorf("cluster client must not be nil")
	}

	cs := client.Clientset()
	if cs == nil {
		return nil, fmt.Errorf("cluster clientset must not be nil")
	}

	rc := client.RESTConfig()
	if rc == nil {
		return nil, fmt.Errorf("cluster rest config must not be nil")
	}

	return &PodController{
		client:     cs,
		restConfig: rc,
		clusterID:  client.ClusterID(),
		logger:     client.logger.Named("pod_controller"),
	}, nil
}

// NewPodControllerFromClientset creates a PodController directly from a clientset and rest config.
// This is useful for testing or when a ClusterClient is not available.
func NewPodControllerFromClientset(clientset kubernetes.Interface, restConfig *rest.Config, clusterID string, logger *zap.Logger) *PodController {
	return &PodController{
		client:     clientset,
		restConfig: restConfig,
		clusterID:  clusterID,
		logger:     logger.Named("pod_controller"),
	}
}

// CreateAttackerPod creates a new attacker pod with strict security constraints.
// The pod is configured with a restrictive security context, resource limits,
// and labels for tracking and cleanup. The pod name is auto-generated from
// the experiment ID.
func (p *PodController) CreateAttackerPod(ctx context.Context, config AttackPodConfig) (*corev1.Pod, error) {
	if config.Namespace == "" {
		config.Namespace = "chaos-sec"
	}

	if config.Image == "" {
		return nil, fmt.Errorf("attack pod image must not be empty")
	}

	if config.ExperimentID == "" {
		return nil, fmt.Errorf("experiment ID must not be empty")
	}

	// Apply default resource limits if not specified.
	if config.ResourceLimits == nil {
		config.ResourceLimits = defaultResourceLimits()
	}

	// Fill in default limit values for any empty fields.
	rl := config.ResourceLimits
	if rl.CPURequest == "" {
		rl.CPURequest = "100m"
	}
	if rl.CPULimit == "" {
		rl.CPULimit = "500m"
	}
	if rl.MemoryRequest == "" {
		rl.MemoryRequest = "128Mi"
	}
	if rl.MemoryLimit == "" {
		rl.MemoryLimit = "512Mi"
	}

	// Generate a unique pod name using the experiment ID and a short timestamp.
	podName := fmt.Sprintf("chaos-attacker-%s-%d", truncateID(config.ExperimentID, 8), time.Now().UnixNano()%100000)

	// Build environment variables.
	envVars := make([]corev1.EnvVar, 0, len(config.EnvVars))
	for key, value := range config.EnvVars {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Ensure CHAOS_SEC environment markers are set for identification.
	envVars = append(envVars, corev1.EnvVar{
		Name:  "CHAOS_SEC_EXPERIMENT",
		Value: config.ExperimentID,
	})
	envVars = append(envVars, corev1.EnvVar{
		Name:  "CHAOS_SEC_RUN",
		Value: config.RunID,
	})

	// Build the pod spec with maximum security restrictions.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: config.Namespace,
			Labels: map[string]string{
				"app":                          "chaos-sec-attacker",
				"app.kubernetes.io/managed-by": "chaos-sec",
				"chaos-sec/experiment-id":      config.ExperimentID,
				"chaos-sec/run-id":             config.RunID,
				"chaos-sec/template-id":        config.TemplateID,
			},
			Annotations: map[string]string{
				"chaos-sec/created-by": "pod-controller",
				"chaos-sec/created-at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: corev1.PodSpec{
			// Security: Disable host namespace sharing.
			HostNetwork: false,
			HostPID:     false,
			HostIPC:     false,

			// Security: Do not automount the service account token.
			AutomountServiceAccountToken: newBool(false),

			// Use the default service account with minimal permissions.
			ServiceAccountName: "default",

			// Restart policy: never restart attacker pods.
			RestartPolicy: corev1.RestartPolicyNever,

			// Containers for the attacker pod.
			Containers: []corev1.Container{
				{
					Name:    "attacker",
					Image:   config.Image,
					Command: config.Command,

					// Resource requests and limits.
					Resources: corev1.ResourceRequirements{
						Requests: mustParseResourceList(rl.CPURequest, rl.MemoryRequest),
						Limits:   mustParseResourceList(rl.CPULimit, rl.MemoryLimit),
					},

					// Environment variables.
					Env: envVars,

					// Security context for the container.
					SecurityContext: &corev1.SecurityContext{
						// Must run as non-root.
						RunAsNonRoot: newBool(true),
						RunAsUser:    int64Ptr(1000),
						RunAsGroup:   int64Ptr(1000),

						// Drop ALL Linux capabilities.
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},

						// Read-only root filesystem.
						ReadOnlyRootFilesystem: newBool(true),

						// Prevent privilege escalation.
						AllowPrivilegeEscalation: newBool(false),

						// Set a seccomp profile.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},

					// Mount an emptyDir volume for writable temp space.
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "tmp",
							MountPath: "/tmp",
						},
						{
							Name:      "home",
							MountPath: "/home/attacker",
						},
					},
				},
			},

			// Volumes: emptyDir for temp writable directories (since root FS is read-only).
			Volumes: []corev1.Volume{
				{
					Name: "tmp",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "home",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},

			// Pod-level security context.
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: newBool(true),
				RunAsUser:    int64Ptr(1000),
				RunAsGroup:   int64Ptr(1000),
				FSGroup:      int64Ptr(1000),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
		},
	}

	p.logger.Info("creating attacker pod",
		zap.String("pod_name", podName),
		zap.String("namespace", config.Namespace),
		zap.String("experiment_id", config.ExperimentID),
		zap.String("run_id", config.RunID),
		zap.String("image", config.Image),
	)

	createdPod, err := p.client.CoreV1().Pods(config.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		p.logger.Error("failed to create attacker pod",
			zap.String("pod_name", podName),
			zap.String("namespace", config.Namespace),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to create attacker pod %s in namespace %s: %w", podName, config.Namespace, err)
	}

	p.logger.Info("attacker pod created successfully",
		zap.String("pod_name", createdPod.Name),
		zap.String("namespace", createdPod.Namespace),
	)

	return createdPod, nil
}

// WaitForPodReady blocks until the specified pod reaches the "Running" phase
// or the context/timeout expires. It polls the pod status at regular intervals.
func (p *PodController) WaitForPodReady(ctx context.Context, podName, namespace string, timeout time.Duration) error {
	p.logger.Info("waiting for pod to be ready",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
		zap.Duration("timeout", timeout),
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := p.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			// If the pod is not found yet, keep waiting.
			return false, nil
		}

		switch pod.Status.Phase {
		case corev1.PodRunning:
			p.logger.Info("pod is running",
				zap.String("pod_name", podName),
				zap.String("namespace", namespace),
			)
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			return false, fmt.Errorf("pod %s/%s reached terminal phase: %s", namespace, podName, pod.Status.Phase)
		default:
			return false, nil
		}
	})
}

// WaitForPodCompletion blocks until the specified pod reaches a terminal phase
// (Succeeded or Failed) or the context/timeout expires.
func (p *PodController) WaitForPodCompletion(ctx context.Context, podName, namespace string, timeout time.Duration) (*corev1.Pod, error) {
	p.logger.Info("waiting for pod to complete",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
		zap.Duration("timeout", timeout),
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var finalPod *corev1.Pod

	err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		pod, err := p.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}

		finalPod = pod

		switch pod.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			return true, nil
		default:
			return false, nil
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed waiting for pod %s/%s to complete: %w", namespace, podName, err)
	}

	return finalPod, nil
}

// ExecuteInPod executes a command inside a running container within a pod.
// It uses the Kubernetes remotecommand API to stream the command's stdout/stderr
// back to the caller. The command runs as the container's configured user.
func (p *PodController) ExecuteInPod(ctx context.Context, podName, namespace, container, command string) (string, error) {
	p.logger.Info("executing command in pod",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
		zap.String("container", container),
		zap.String("command", command),
	)

	req := p.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/sh", "-c", command},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(p.restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create SPDY executor for pod %s/%s: %w", namespace, podName, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	output := stdout.String()
	if stderr.Len() > 0 {
		p.logger.Debug("command stderr",
			zap.String("pod_name", podName),
			zap.String("stderr", stderr.String()),
		)
		output += "\n" + stderr.String()
	}

	if err != nil {
		p.logger.Error("command execution failed",
			zap.String("pod_name", podName),
			zap.String("namespace", namespace),
			zap.String("command", command),
			zap.Error(err),
		)
		return output, fmt.Errorf("command execution failed in pod %s/%s: %w", namespace, podName, err)
	}

	p.logger.Info("command executed successfully",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	return output, nil
}

// GetPodLogs retrieves the logs for a pod. If opts is nil, default log options
// are used (tail limit of 100 lines from the "attacker" container).
func (p *PodController) GetPodLogs(ctx context.Context, podName, namespace string, opts *corev1.PodLogOptions) (string, error) {
	if opts == nil {
		opts = &corev1.PodLogOptions{
			Container: "attacker",
			TailLines: int64Ptr(100),
		}
	}

	p.logger.Debug("retrieving pod logs",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	req := p.client.CoreV1().Pods(namespace).GetLogs(podName, opts)
	logStream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to stream logs for pod %s/%s: %w", namespace, podName, err)
	}
	defer logStream.Close()

	logBytes, err := io.ReadAll(logStream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs for pod %s/%s: %w", namespace, podName, err)
	}

	return string(logBytes), nil
}

// DeletePod gracefully deletes a pod. It first attempts to delete with
// a grace period, then falls back to a forceful deletion if the context
// is cancelled.
func (p *PodController) DeletePod(ctx context.Context, podName, namespace string) error {
	p.logger.Info("deleting pod",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	gracePeriodSeconds := int64(30)
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	err := p.client.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOpts)
	if err != nil {
		p.logger.Error("failed to delete pod",
			zap.String("pod_name", podName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete pod %s/%s: %w", namespace, podName, err)
	}

	p.logger.Info("pod deleted successfully",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	return nil
}

// ForceDeletePod forcefully deletes a pod by setting the grace period to zero.
// Use this only for cleanup when graceful deletion has failed.
func (p *PodController) ForceDeletePod(ctx context.Context, podName, namespace string) error {
	p.logger.Warn("force deleting pod",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	gracePeriodSeconds := int64(0)
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	err := p.client.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOpts)
	if err != nil {
		p.logger.Error("failed to force delete pod",
			zap.String("pod_name", podName),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return fmt.Errorf("failed to force delete pod %s/%s: %w", namespace, podName, err)
	}

	p.logger.Info("pod force deleted",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	return nil
}

// DeletePodsWithLabel deletes all pods matching the given label selector in the
// specified namespace. This is used for experiment cleanup to remove all
// attacker pods associated with an experiment or run.
func (p *PodController) DeletePodsWithLabel(ctx context.Context, namespace, labelSelector string) error {
	p.logger.Info("deleting pods with label selector",
		zap.String("namespace", namespace),
		zap.String("label_selector", labelSelector),
	)

	selector, err := labels.Parse(labelSelector)
	if err != nil {
		return fmt.Errorf("invalid label selector %q: %w", labelSelector, err)
	}

	gracePeriodSeconds := int64(30)
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	listOpts := metav1.ListOptions{
		LabelSelector: selector.String(),
	}

	err = p.client.CoreV1().Pods(namespace).DeleteCollection(ctx, deleteOpts, listOpts)
	if err != nil {
		p.logger.Error("failed to delete pods with label",
			zap.String("namespace", namespace),
			zap.String("label_selector", labelSelector),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete pods with label %q in namespace %s: %w", labelSelector, namespace, err)
	}

	p.logger.Info("pods deleted with label selector",
		zap.String("namespace", namespace),
		zap.String("label_selector", labelSelector),
	)

	return nil
}

// WatchPod returns a watch.Interface that streams pod events for the specified pod.
// The caller is responsible for closing the watch when done.
func (p *PodController) WatchPod(ctx context.Context, podName, namespace string) (watchapi.Interface, error) {
	p.logger.Debug("setting up pod watch",
		zap.String("pod_name", podName),
		zap.String("namespace", namespace),
	)

	fieldSelector := fields.OneTermEqualSelector("metadata.name", podName)

	watcher, err := p.client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to watch pod %s/%s: %w", namespace, podName, err)
	}

	return watcher, nil
}

// GetPodStatus returns detailed status information about the specified pod.
// It includes phase, conditions, container statuses, IP, and node name.
func (p *PodController) GetPodStatus(ctx context.Context, podName, namespace string) (*PodStatusInfo, error) {
	pod, err := p.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, podName, err)
	}

	statusInfo := &PodStatusInfo{
		Phase:             string(pod.Status.Phase),
		Conditions:        pod.Status.Conditions,
		ContainerStatuses: pod.Status.ContainerStatuses,
		IP:                pod.Status.PodIP,
		StartTime:         pod.Status.StartTime,
		Message:           pod.Status.Message,
		Reason:            pod.Status.Reason,
	}

	if pod.Spec.NodeName != "" {
		statusInfo.NodeName = pod.Spec.NodeName
	}

	return statusInfo, nil
}

// ListPodsByExperiment lists all pods belonging to a specific experiment.
// It uses the chaos-sec/experiment-id label to filter pods.
func (p *PodController) ListPodsByExperiment(ctx context.Context, namespace, experimentID string) ([]corev1.Pod, error) {
	labelSelector := fmt.Sprintf("chaos-sec/experiment-id=%s", experimentID)

	podList, err := p.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for experiment %s in namespace %s: %w", experimentID, namespace, err)
	}

	return podList.Items, nil
}

// CleanupExperimentPods removes all pods created for a specific experiment run.
// It first attempts graceful deletion, then optionally force-deletes any
// remaining pods after a timeout.
func (p *PodController) CleanupExperimentPods(ctx context.Context, namespace, experimentID, runID string) error {
	p.logger.Info("cleaning up experiment pods",
		zap.String("namespace", namespace),
		zap.String("experiment_id", experimentID),
		zap.String("run_id", runID),
	)

	var labelSelector string
	if runID != "" {
		labelSelector = fmt.Sprintf("chaos-sec/experiment-id=%s,chaos-sec/run-id=%s", experimentID, runID)
	} else {
		labelSelector = fmt.Sprintf("chaos-sec/experiment-id=%s", experimentID)
	}

	// Attempt graceful deletion.
	if err := p.DeletePodsWithLabel(ctx, namespace, labelSelector); err != nil {
		return fmt.Errorf("failed to cleanup experiment pods: %w", err)
	}

	// Verify deletion with retries.
	return retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return true // retry all errors
	}, func() error {
		selector, _ := labels.Parse(labelSelector)
		podList, err := p.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector.String(),
		})
		if err != nil {
			return err
		}

		if len(podList.Items) > 0 {
			return fmt.Errorf("%d pods still remaining after cleanup", len(podList.Items))
		}

		p.logger.Info("experiment pods cleanup verified",
			zap.String("namespace", namespace),
			zap.String("experiment_id", experimentID),
		)
		return nil
	})
}

// WaitForPodByWatcher watches a pod until it reaches the Running phase using
// a Kubernetes watch instead of polling. This is more efficient than WaitForPodReady
// for long-running waits.
func (p *PodController) WaitForPodByWatcher(ctx context.Context, podName, namespace string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fieldSelector := fields.OneTermEqualSelector("metadata.name", podName).String()

	watcher, err := p.client.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to watch pod %s/%s: %w", namespace, podName, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for pod %s/%s to be ready", namespace, podName)
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed for pod %s/%s", namespace, podName)
			}

			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			switch pod.Status.Phase {
			case corev1.PodRunning:
				return nil
			case corev1.PodFailed, corev1.PodSucceeded:
				return fmt.Errorf("pod %s/%s reached terminal phase: %s", namespace, podName, pod.Status.Phase)
			default:
				// Continue watching for phase changes.
			}
		}
	}
}

// --- Helper functions ---

// truncateID returns the first n characters of an ID string for use in pod names.
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// newBool returns a pointer to the given bool value.
func newBool(b bool) *bool {
	return &b
}

// int64Ptr returns a pointer to the given int64 value.
func int64Ptr(i int64) *int64 {
	return &i
}

// mustParseResourceList creates a corev1.ResourceList from CPU and memory strings.
// Panics if the strings cannot be parsed (should not happen with validated defaults).
func mustParseResourceList(cpu, memory string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    mustParseQuantity(cpu),
		corev1.ResourceMemory: mustParseQuantity(memory),
	}
}
