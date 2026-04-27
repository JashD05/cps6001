package kubernetes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NamespaceStatus holds the current status and resource usage of a namespace.
type NamespaceStatus struct {
	Name      string              `json:"name"`
	Phase     string              `json:"phase"`
	PodCount  int                 `json:"pod_count"`
	Resources *NamespaceResources `json:"resources,omitempty"`
	Labels    map[string]string   `json:"labels,omitempty"`
	CreatedAt *metav1.Time        `json:"created_at,omitempty"`
}

// NamespaceResources holds the resource usage within a namespace.
type NamespaceResources struct {
	UsedCPU     string `json:"used_cpu,omitempty"`
	UsedMemory  string `json:"used_memory,omitempty"`
	PodCount    int    `json:"pod_count"`
	CPUQuota    string `json:"cpu_quota,omitempty"`
	MemoryQuota string `json:"memory_quota,omitempty"`
	PodQuota    int    `json:"pod_quota,omitempty"`
}

// NamespaceManager manages Kubernetes namespaces for experiment isolation.
// It provides methods for creating isolated namespaces with security
// constraints, resource quotas, and limit ranges, as well as cleaning
// up namespaces when experiments complete.
type NamespaceManager struct {
	client     kubernetes.Interface
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger
}

// NewNamespaceManager creates a new NamespaceManager for the given cluster client.
func NewNamespaceManager(client *ClusterClient) (*NamespaceManager, error) {
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

	return &NamespaceManager{
		client:     cs,
		restConfig: rc,
		clusterID:  client.ClusterID(),
		logger:     client.logger.Named("namespace_manager"),
	}, nil
}

// NewNamespaceManagerFromClientset creates a NamespaceManager directly from a
// clientset and rest config. This is useful for testing or when a ClusterClient
// is not available.
func NewNamespaceManagerFromClientset(clientset kubernetes.Interface, restConfig *rest.Config, clusterID string, logger *zap.Logger) *NamespaceManager {
	return &NamespaceManager{
		client:     clientset,
		restConfig: restConfig,
		clusterID:  clusterID,
		logger:     logger.Named("namespace_manager"),
	}
}

// CreateExperimentNamespace creates an isolated Kubernetes namespace for a security
// experiment. The namespace is configured with:
//   - A name in the format: chaos-sec-exp-{short-id}
//   - Labels for managed-by=chaos-sec and the experiment ID
//   - Pod Security Standards labels enforcing the restricted profile
//   - A ResourceQuota limiting pods, CPU, and memory
//   - A LimitRange setting default and maximum resource limits per pod
//
// The namespace and all its resources are deleted when the experiment is
// complete via DeleteNamespace.
func (n *NamespaceManager) CreateExperimentNamespace(ctx context.Context, baseName, experimentID string) (string, error) {
	if experimentID == "" {
		return "", fmt.Errorf("experiment ID must not be empty")
	}

	// Generate the namespace name with a short ID suffix.
	shortID := truncateID(experimentID, 8)
	nsName := fmt.Sprintf("chaos-sec-exp-%s", shortID)

	// If a base name is provided, incorporate it for readability.
	if baseName != "" {
		sanitized := sanitizeName(baseName)
		if len(sanitized) > 20 {
			sanitized = sanitized[:20]
		}
		nsName = fmt.Sprintf("chaos-sec-exp-%s-%s", sanitized, shortID)
	}

	// Kubernetes namespace names must be 63 characters or fewer and follow DNS label format.
	if len(nsName) > 63 {
		nsName = nsName[:63]
	}
	// Ensure it doesn't end with a hyphen after truncation.
	for len(nsName) > 0 && nsName[len(nsName)-1] == '-' {
		nsName = nsName[:len(nsName)-1]
	}

	n.logger.Info("creating experiment namespace",
		zap.String("namespace", nsName),
		zap.String("experiment_id", experimentID),
	)

	// Check if the namespace already exists — if so, return it.
	exists, err := n.NamespaceExists(ctx, nsName)
	if err != nil {
		return "", fmt.Errorf("failed to check namespace existence: %w", err)
	}
	if exists {
		n.logger.Info("namespace already exists, returning existing namespace",
			zap.String("namespace", nsName),
		)
		return nsName, nil
	}

	// Create the namespace with labels for tracking and Pod Security Standards.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":               "chaos-sec",
				"chaos-sec/experiment-id":                    experimentID,
				"chaos-sec/created-at":                       time.Now().UTC().Format(time.RFC3339),
				"pod-security.kubernetes.io/enforce":         "restricted",
				"pod-security.kubernetes.io/enforce-version": "latest",
				"pod-security.kubernetes.io/audit":           "restricted",
				"pod-security.kubernetes.io/audit-version":   "latest",
				"pod-security.kubernetes.io/warn":            "restricted",
				"pod-security.kubernetes.io/warn-version":    "latest",
			},
			Annotations: map[string]string{
				"chaos-sec/experiment-id": experimentID,
				"chaos-sec/created-by":    "namespace-manager",
			},
		},
	}

	createdNS, err := n.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Info("namespace created concurrently, returning existing",
				zap.String("namespace", nsName),
			)
			return nsName, nil
		}
		n.logger.Error("failed to create namespace",
			zap.String("namespace", nsName),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create namespace %s: %w", nsName, err)
	}

	n.logger.Info("namespace created successfully",
		zap.String("namespace", createdNS.Name),
	)

	// Create the ResourceQuota for the namespace to limit total resource consumption.
	if err := n.createResourceQuota(ctx, nsName); err != nil {
		// If quota creation fails, attempt to clean up the namespace.
		n.logger.Error("failed to create resource quota, cleaning up namespace",
			zap.String("namespace", nsName),
			zap.Error(err),
		)
		_ = n.DeleteNamespace(ctx, nsName)
		return "", fmt.Errorf("failed to create resource quota for namespace %s: %w", nsName, err)
	}

	// Create the LimitRange for the namespace to set per-pod defaults and maximums.
	if err := n.createLimitRange(ctx, nsName); err != nil {
		n.logger.Error("failed to create limit range, cleaning up namespace",
			zap.String("namespace", nsName),
			zap.Error(err),
		)
		_ = n.DeleteNamespace(ctx, nsName)
		return "", fmt.Errorf("failed to create limit range for namespace %s: %w", nsName, err)
	}

	n.logger.Info("experiment namespace fully configured",
		zap.String("namespace", nsName),
		zap.String("experiment_id", experimentID),
	)

	return nsName, nil
}

// createResourceQuota creates a ResourceQuota in the specified namespace that limits
// the total resource consumption. This prevents a single experiment from consuming
// excessive cluster resources.
// Limits: 10 pods, 2 CPU, 2Gi memory.
func (n *NamespaceManager) createResourceQuota(ctx context.Context, namespace string) error {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-sec-quota",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "chaos-sec",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourcePods:           mustParseQuantity("10"),
				corev1.ResourceCPU:            mustParseQuantity("2"),
				corev1.ResourceMemory:         mustParseQuantity("2Gi"),
				corev1.ResourceRequestsCPU:    mustParseQuantity("2"),
				corev1.ResourceRequestsMemory: mustParseQuantity("2Gi"),
				corev1.ResourceLimitsCPU:      mustParseQuantity("2"),
				corev1.ResourceLimitsMemory:   mustParseQuantity("2Gi"),
			},
		},
	}

	_, err := n.client.CoreV1().ResourceQuotas(namespace).Create(ctx, quota, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create resource quota in namespace %s: %w", namespace, err)
	}

	n.logger.Debug("resource quota created",
		zap.String("namespace", namespace),
	)

	return nil
}

// createLimitRange creates a LimitRange in the specified namespace that sets
// default and maximum resource limits for individual pods. This ensures that
// even if a pod doesn't specify resource limits, it will be constrained.
// Defaults: 100m CPU, 128Mi memory.
// Maximums: 500m CPU, 512Mi memory.
func (n *NamespaceManager) createLimitRange(ctx context.Context, namespace string) error {
	limitRange := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-sec-limits",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "chaos-sec",
			},
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("100m"),
						corev1.ResourceMemory: mustParseQuantity("128Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("50m"),
						corev1.ResourceMemory: mustParseQuantity("64Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("500m"),
						corev1.ResourceMemory: mustParseQuantity("512Mi"),
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("50m"),
						corev1.ResourceMemory: mustParseQuantity("64Mi"),
					},
					MaxLimitRequestRatio: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("5"),
						corev1.ResourceMemory: mustParseQuantity("4"),
					},
				},
				{
					Type: corev1.LimitTypePod,
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("500m"),
						corev1.ResourceMemory: mustParseQuantity("512Mi"),
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    mustParseQuantity("50m"),
						corev1.ResourceMemory: mustParseQuantity("64Mi"),
					},
				},
			},
		},
	}

	_, err := n.client.CoreV1().LimitRanges(namespace).Create(ctx, limitRange, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create limit range in namespace %s: %w", namespace, err)
	}

	n.logger.Debug("limit range created",
		zap.String("namespace", namespace),
	)

	return nil
}

// DeleteNamespace removes a namespace and all resources within it.
// Kubernetes handles cascading deletion of all resources in the namespace
// when the namespace is deleted. The deletion is graceful with a 60-second
// timeout, after which the namespace is force-deleted.
func (n *NamespaceManager) DeleteNamespace(ctx context.Context, name string) error {
	n.logger.Info("deleting namespace",
		zap.String("namespace", name),
	)

	err := n.client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			n.logger.Debug("namespace not found, already deleted",
				zap.String("namespace", name),
			)
			return nil
		}
		n.logger.Error("failed to delete namespace",
			zap.String("namespace", name),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete namespace %s: %w", name, err)
	}

	n.logger.Info("namespace deletion initiated",
		zap.String("namespace", name),
	)

	return nil
}

// WaitForNamespaceTermination waits for a namespace to be fully terminated and
// removed from the cluster. This is useful to call after DeleteNamespace to
// ensure cleanup is complete before proceeding.
func (n *NamespaceManager) WaitForNamespaceTermination(ctx context.Context, name string, timeout time.Duration) error {
	n.logger.Info("waiting for namespace termination",
		zap.String("namespace", name),
		zap.Duration("timeout", timeout),
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for namespace %s to terminate", name)
		default:
		}

		_, err := n.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				n.logger.Info("namespace terminated successfully",
					zap.String("namespace", name),
				)
				return nil
			}
			// Transient errors — keep trying.
			n.logger.Debug("error checking namespace status, retrying",
				zap.String("namespace", name),
				zap.Error(err),
			)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for namespace %s to terminate", name)
		case <-time.After(2 * time.Second):
		}
	}
}

// NamespaceExists checks whether a namespace with the given name exists
// in the cluster.
func (n *NamespaceManager) NamespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := n.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check namespace %s existence: %w", name, err)
	}
	return true, nil
}

// ListNamespaces returns a list of namespace names matching the given label selector.
// The label selector should be a valid Kubernetes label selector string
// (e.g., "app.kubernetes.io/managed-by=chaos-sec").
func (n *NamespaceManager) ListNamespaces(ctx context.Context, labelSelector string) ([]string, error) {
	listOpts := metav1.ListOptions{}
	if labelSelector != "" {
		listOpts.LabelSelector = labelSelector
	}

	namespaceList, err := n.client.CoreV1().Namespaces().List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces with selector %q: %w", labelSelector, err)
	}

	names := make([]string, 0, len(namespaceList.Items))
	for _, ns := range namespaceList.Items {
		names = append(names, ns.Name)
	}

	return names, nil
}

// ListExperimentNamespaces returns all namespaces managed by chaos-sec
// for experiments, optionally filtered by a specific experiment ID.
func (n *NamespaceManager) ListExperimentNamespaces(ctx context.Context, experimentID string) ([]string, error) {
	selector := "app.kubernetes.io/managed-by=chaos-sec"
	if experimentID != "" {
		selector = fmt.Sprintf("%s,chaos-sec/experiment-id=%s", selector, experimentID)
	}
	return n.ListNamespaces(ctx, selector)
}

// GetNamespaceStatus returns the current status and resource usage for a namespace.
// It includes the namespace phase, pod count, resource consumption, and quota
// information if a ResourceQuota is present.
func (n *NamespaceManager) GetNamespaceStatus(ctx context.Context, name string) (*NamespaceStatus, error) {
	ns, err := n.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("namespace %s not found", name)
		}
		return nil, fmt.Errorf("failed to get namespace %s: %w", name, err)
	}

	status := &NamespaceStatus{
		Name:   ns.Name,
		Phase:  string(ns.Status.Phase),
		Labels: ns.Labels,
	}

	if !ns.CreationTimestamp.IsZero() {
		status.CreatedAt = &ns.CreationTimestamp
	}

	// Count pods in the namespace.
	podList, err := n.client.CoreV1().Pods(name).List(ctx, metav1.ListOptions{})
	if err != nil {
		n.logger.Warn("failed to list pods in namespace for status",
			zap.String("namespace", name),
			zap.Error(err),
		)
	} else {
		status.PodCount = len(podList.Items)
	}

	// Get resource usage from the ResourceQuota.
	resources, err := n.getNamespaceResourceUsage(ctx, name)
	if err != nil {
		n.logger.Warn("failed to get resource usage for namespace",
			zap.String("namespace", name),
			zap.Error(err),
		)
	} else {
		status.Resources = resources
	}

	return status, nil
}

// getNamespaceResourceUsage retrieves resource usage and quota information
// from the ResourceQuota in the namespace.
func (n *NamespaceManager) getNamespaceResourceUsage(ctx context.Context, namespace string) (*NamespaceResources, error) {
	quotaList, err := n.client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource quotas: %w", err)
	}

	resources := &NamespaceResources{}

	if len(quotaList.Items) == 0 {
		return resources, nil
	}

	// Use the first quota (our namespaces should only have one).
	quota := quotaList.Items[0]

	// Extract used resources.
	if used, ok := quota.Status.Used[corev1.ResourceCPU]; ok {
		resources.UsedCPU = used.String()
	}
	if used, ok := quota.Status.Used[corev1.ResourceMemory]; ok {
		resources.UsedMemory = used.String()
	}
	if used, ok := quota.Status.Used[corev1.ResourcePods]; ok {
		podCount, ok := used.AsInt64()
		if ok {
			resources.PodCount = int(podCount)
		}
	}

	// Extract hard limits (quota).
	if hard, ok := quota.Spec.Hard[corev1.ResourceCPU]; ok {
		resources.CPUQuota = hard.String()
	}
	if hard, ok := quota.Spec.Hard[corev1.ResourceMemory]; ok {
		resources.MemoryQuota = hard.String()
	}
	if hard, ok := quota.Spec.Hard[corev1.ResourcePods]; ok {
		podQuota, ok := hard.AsInt64()
		if ok {
			resources.PodQuota = int(podQuota)
		}
	}

	return resources, nil
}

// UpdateNamespaceLabels adds or updates labels on an existing namespace.
// This is useful for adding tracking labels or marking namespaces for cleanup.
func (n *NamespaceManager) UpdateNamespaceLabels(ctx context.Context, name string, labels map[string]string) error {
	ns, err := n.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get namespace %s for label update: %w", name, err)
	}

	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}

	for key, value := range labels {
		ns.Labels[key] = value
	}

	_, err = n.client.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update labels on namespace %s: %w", name, err)
	}

	n.logger.Debug("namespace labels updated",
		zap.String("namespace", name),
		zap.Any("labels", labels),
	)

	return nil
}

// CleanupExperimentNamespaces deletes all namespaces associated with a specific
// experiment ID. It waits for each namespace to be terminated up to the specified
// timeout before moving on to the next one.
func (n *NamespaceManager) CleanupExperimentNamespaces(ctx context.Context, experimentID string, timeout time.Duration) error {
	n.logger.Info("cleaning up experiment namespaces",
		zap.String("experiment_id", experimentID),
	)

	namespaces, err := n.ListExperimentNamespaces(ctx, experimentID)
	if err != nil {
		return fmt.Errorf("failed to list experiment namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		n.logger.Info("no namespaces to clean up",
			zap.String("experiment_id", experimentID),
		)
		return nil
	}

	var lastErr error
	for _, nsName := range namespaces {
		if err := n.DeleteNamespace(ctx, nsName); err != nil {
			n.logger.Error("failed to delete namespace during cleanup",
				zap.String("namespace", nsName),
				zap.Error(err),
			)
			lastErr = err
			continue
		}

		// Wait for termination with a shorter timeout per namespace.
		perNSTimeout := timeout
		if len(namespaces) > 1 {
			perNSTimeout = timeout / time.Duration(len(namespaces))
		}

		if err := n.WaitForNamespaceTermination(ctx, nsName, perNSTimeout); err != nil {
			n.logger.Warn("namespace did not terminate within timeout",
				zap.String("namespace", nsName),
				zap.Duration("timeout", perNSTimeout),
				zap.Error(err),
			)
		}
	}

	return lastErr
}

// GetResourceQuotaStatus returns the current status of the ResourceQuota
// in the specified namespace, including both hard limits and current usage.
func (n *NamespaceManager) GetResourceQuotaStatus(ctx context.Context, namespace string) (hard, used corev1.ResourceList, err error) {
	quotaList, err := n.client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list resource quotas in namespace %s: %w", namespace, err)
	}

	if len(quotaList.Items) == 0 {
		return nil, nil, nil
	}

	quota := quotaList.Items[0]
	return quota.Spec.Hard, quota.Status.Used, nil
}

// CheckResourceAvailability checks whether the namespace has enough remaining
// resources (under its quota) to accommodate the specified CPU and memory
// requirements. Returns true if the resources are available, false otherwise.
func (n *NamespaceManager) CheckResourceAvailability(ctx context.Context, namespace string, requiredCPU, requiredMemory resource.Quantity) (bool, error) {
	hard, used, err := n.GetResourceQuotaStatus(ctx, namespace)
	if err != nil {
		return false, fmt.Errorf("failed to check resource availability: %w", err)
	}

	if hard == nil || used == nil {
		// No quota means no restrictions.
		return true, nil
	}

	// Check CPU availability.
	if cpuHard, ok := hard[corev1.ResourceCPU]; ok {
		if cpuUsed, ok := used[corev1.ResourceCPU]; ok {
			available := cpuHard.DeepCopy()
			available.Sub(cpuUsed)
			if available.Cmp(requiredCPU) < 0 {
				n.logger.Debug("insufficient CPU resources in namespace",
					zap.String("namespace", namespace),
					zap.String("available", available.String()),
					zap.String("required", requiredCPU.String()),
				)
				return false, nil
			}
		}
	}

	// Check memory availability.
	if memHard, ok := hard[corev1.ResourceMemory]; ok {
		if memUsed, ok := used[corev1.ResourceMemory]; ok {
			available := memHard.DeepCopy()
			available.Sub(memUsed)
			if available.Cmp(requiredMemory) < 0 {
				n.logger.Debug("insufficient memory resources in namespace",
					zap.String("namespace", namespace),
					zap.String("available", available.String()),
					zap.String("required", requiredMemory.String()),
				)
				return false, nil
			}
		}
	}

	// Check pod count availability.
	if podsHard, ok := hard[corev1.ResourcePods]; ok {
		if podsUsed, ok := used[corev1.ResourcePods]; ok {
			available := podsHard.DeepCopy()
			available.Sub(podsUsed)
			one := resource.MustParse("1")
			if available.Cmp(one) < 0 {
				n.logger.Debug("insufficient pod quota in namespace",
					zap.String("namespace", namespace),
					zap.String("available_pods", available.String()),
				)
				return false, nil
			}
		}
	}

	return true, nil
}

// sanitizeName converts a human-readable name into a DNS-label-safe string
// suitable for use in Kubernetes namespace names.
func sanitizeName(name string) string {
	var result []rune
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		} else if r >= 'A' && r <= 'Z' {
			result = append(result, r+32) // lowercase
		} else if r == '-' || r == '_' || r == ' ' {
			result = append(result, '-')
		}
	}

	sanitized := string(result)

	// Remove consecutive hyphens.
	prev := '-'
	var cleaned []rune
	for _, r := range sanitized {
		if r == '-' && prev == '-' {
			continue
		}
		cleaned = append(cleaned, r)
		prev = r
	}
	sanitized = string(cleaned)

	// Trim leading/trailing hyphens.
	for len(sanitized) > 0 && sanitized[0] == '-' {
		sanitized = sanitized[1:]
	}
	for len(sanitized) > 0 && sanitized[len(sanitized)-1] == '-' {
		sanitized = sanitized[:len(sanitized)-1]
	}

	return sanitized
}
