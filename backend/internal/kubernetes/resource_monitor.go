package kubernetes

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ClusterResources holds the total and available resource capacities for a
// Kubernetes cluster, aggregated across all nodes.
type ClusterResources struct {
	TotalCPU        string `json:"total_cpu"`
	AvailableCPU    string `json:"available_cpu"`
	TotalMemory     string `json:"total_memory"`
	AvailableMemory string `json:"available_memory"`
	NodeCount       int    `json:"node_count"`
	PodCount        int    `json:"pod_count"`
	// Raw quantities for programmatic use.
	TotalCPUQuantity        resource.Quantity `json:"-"`
	AvailableCPUQuantity    resource.Quantity `json:"-"`
	TotalMemoryQuantity     resource.Quantity `json:"-"`
	AvailableMemoryQuantity resource.Quantity `json:"-"`
}

// PodResources holds the resource usage information for a single pod.
type PodResources struct {
	PodName   string `json:"pod_name"`
	Namespace string `json:"namespace,omitempty"`
	CPU       string `json:"cpu,omitempty"`
	Memory    string `json:"memory,omitempty"`
	CPULimit  string `json:"cpu_limit,omitempty"`
	MemLimit  string `json:"mem_limit,omitempty"`
	Status    string `json:"status"`
	NodeName  string `json:"node_name,omitempty"`
}

// ResourceMonitor provides methods for tracking and querying resource usage
// at the cluster, namespace, and pod levels. It uses the Kubernetes Metrics API
// where available, and falls back to resource requests/limits for estimation.
type ResourceMonitor struct {
	client     kubernetes.Interface
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger
}

// NewResourceMonitor creates a new ResourceMonitor for the given cluster client.
func NewResourceMonitor(client *ClusterClient) (*ResourceMonitor, error) {
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

	return &ResourceMonitor{
		client:     cs,
		restConfig: rc,
		clusterID:  client.ClusterID(),
		logger:     client.logger.Named("resource_monitor"),
	}, nil
}

// NewResourceMonitorFromClientset creates a ResourceMonitor directly from a
// clientset and rest config. Useful for testing.
func NewResourceMonitorFromClientset(clientset kubernetes.Interface, restConfig *rest.Config, clusterID string, logger *zap.Logger) *ResourceMonitor {
	return &ResourceMonitor{
		client:     clientset,
		restConfig: restConfig,
		clusterID:  clusterID,
		logger:     logger.Named("resource_monitor"),
	}
}

// GetClusterResources returns the total and available CPU, memory, and pod
// count across all nodes in the cluster. It aggregates the allocatable
// resources from each node and subtracts the currently requested resources.
func (r *ResourceMonitor) GetClusterResources(ctx context.Context) (*ClusterResources, error) {
	r.logger.Debug("getting cluster resources")

	// Get all nodes in the cluster.
	nodeList, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes for cluster resources: %w", err)
	}

	totalCPU := resource.MustParse("0")
	totalMemory := resource.MustParse("0")
	allocatableCPU := resource.MustParse("0")
	allocatableMemory := resource.MustParse("0")

	for _, node := range nodeList.Items {
		// Use allocatable (not capacity) as that accounts for system reservations.
		if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			allocatableCPU.Add(cpu)
		}
		if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			allocatableMemory.Add(mem)
		}
		if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
			totalCPU.Add(cpu)
		}
		if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
			totalMemory.Add(mem)
		}
	}

	// Get all pods across all namespaces to calculate requested resources.
	// We use non-terminated pods to get the current resource requests.
	requestedCPU := resource.MustParse("0")
	requestedMemory := resource.MustParse("0")
	totalPodCount := 0

	podList, err := r.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase!=Failed,status.phase!=Succeeded",
	})
	if err != nil {
		r.logger.Warn("failed to list pods for resource calculation, using allocatable only",
			zap.Error(err),
		)
		// If we can't list pods, report allocatable as available.
		availableCPU := allocatableCPU.DeepCopy()
		availableMemory := allocatableMemory.DeepCopy()

		return &ClusterResources{
			TotalCPU:                totalCPU.String(),
			AvailableCPU:            availableCPU.String(),
			TotalMemory:             totalMemory.String(),
			AvailableMemory:         availableMemory.String(),
			NodeCount:               len(nodeList.Items),
			PodCount:                0,
			TotalCPUQuantity:        totalCPU,
			AvailableCPUQuantity:    availableCPU,
			TotalMemoryQuantity:     totalMemory,
			AvailableMemoryQuantity: availableMemory,
		}, nil
	}

	totalPodCount = len(podList.Items)

	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				requestedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				requestedMemory.Add(mem)
			}
		}
		// Account for init container requests as well (these may be higher temporarily).
		for _, initContainer := range pod.Spec.InitContainers {
			if cpu, ok := initContainer.Resources.Requests[corev1.ResourceCPU]; ok {
				// Init containers run sequentially, so we take the max, not the sum.
				// For simplicity, we skip init containers in the total calculation
				// since they are transient.
				_ = cpu
			}
		}
	}

	// Calculate available resources by subtracting requested from allocatable.
	availableCPU := allocatableCPU.DeepCopy()
	availableCPU.Sub(requestedCPU)

	availableMemory := allocatableMemory.DeepCopy()
	availableMemory.Sub(requestedMemory)

	// Ensure available doesn't go below zero (can happen with overcommit).
	if availableCPU.CmpInt64(0) < 0 {
		availableCPU = resource.MustParse("0")
	}
	if availableMemory.CmpInt64(0) < 0 {
		availableMemory = resource.MustParse("0")
	}

	clusterResources := &ClusterResources{
		TotalCPU:                totalCPU.String(),
		AvailableCPU:            availableCPU.String(),
		TotalMemory:             totalMemory.String(),
		AvailableMemory:         availableMemory.String(),
		NodeCount:               len(nodeList.Items),
		PodCount:                totalPodCount,
		TotalCPUQuantity:        totalCPU,
		AvailableCPUQuantity:    availableCPU,
		TotalMemoryQuantity:     totalMemory,
		AvailableMemoryQuantity: availableMemory,
	}

	r.logger.Debug("cluster resources calculated",
		zap.String("total_cpu", clusterResources.TotalCPU),
		zap.String("available_cpu", clusterResources.AvailableCPU),
		zap.String("total_memory", clusterResources.TotalMemory),
		zap.String("available_memory", clusterResources.AvailableMemory),
		zap.Int("node_count", clusterResources.NodeCount),
		zap.Int("pod_count", clusterResources.PodCount),
	)

	return clusterResources, nil
}

// GetNamespaceResources returns the resource usage within a specific namespace.
// It calculates the actual requested resources by running pods and includes
// quota information if a ResourceQuota is present.
func (r *ResourceMonitor) GetNamespaceResources(ctx context.Context, namespace string) (*NamespaceResources, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	r.logger.Debug("getting namespace resources",
		zap.String("namespace", namespace),
	)

	// List pods in the namespace to calculate resource usage.
	podList, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	requestedCPU := resource.MustParse("0")
	requestedMemory := resource.MustParse("0")
	runningPodCount := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPodCount++
		}
		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				requestedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				requestedMemory.Add(mem)
			}
		}
	}

	resources := &NamespaceResources{
		UsedCPU:    requestedCPU.String(),
		UsedMemory: requestedMemory.String(),
		PodCount:   len(podList.Items),
	}

	// Enrich with quota information if available.
	quotaList, err := r.client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		r.logger.Warn("failed to list resource quotas for namespace",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
	} else if len(quotaList.Items) > 0 {
		quota := quotaList.Items[0]

		if hardCPU, ok := quota.Spec.Hard[corev1.ResourceCPU]; ok {
			resources.CPUQuota = hardCPU.String()
		}
		if hardMem, ok := quota.Spec.Hard[corev1.ResourceMemory]; ok {
			resources.MemoryQuota = hardMem.String()
		}
		if hardPods, ok := quota.Spec.Hard[corev1.ResourcePods]; ok {
			podQuota, ok := hardPods.AsInt64()
			if ok {
				resources.PodQuota = int(podQuota)
			}
		}
	}

	r.logger.Debug("namespace resources calculated",
		zap.String("namespace", namespace),
		zap.String("used_cpu", resources.UsedCPU),
		zap.String("used_memory", resources.UsedMemory),
		zap.Int("pod_count", resources.PodCount),
	)

	return resources, nil
}

// GetPodResources returns the resource usage information for all pods in the
// specified namespace. It reports both the requested resources and the limits
// for each pod, along with the pod's current status.
func (r *ResourceMonitor) GetPodResources(ctx context.Context, namespace string) ([]PodResources, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	r.logger.Debug("getting pod resources",
		zap.String("namespace", namespace),
	)

	podList, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	podResources := make([]PodResources, 0, len(podList.Items))
	for _, pod := range podList.Items {
		// Aggregate resource requests and limits across all containers.
		requestedCPU := resource.MustParse("0")
		requestedMemory := resource.MustParse("0")
		limitedCPU := resource.MustParse("0")
		limitedMemory := resource.MustParse("0")

		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				requestedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				requestedMemory.Add(mem)
			}
			if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
				limitedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
				limitedMemory.Add(mem)
			}
		}

		pr := PodResources{
			PodName:  pod.Name,
			CPU:      requestedCPU.String(),
			Memory:   requestedMemory.String(),
			CPULimit: limitedCPU.String(),
			MemLimit: limitedMemory.String(),
			Status:   string(pod.Status.Phase),
			NodeName: pod.Spec.NodeName,
		}

		podResources = append(podResources, pr)
	}

	r.logger.Debug("pod resources calculated",
		zap.String("namespace", namespace),
		zap.Int("pod_count", len(podResources)),
	)

	return podResources, nil
}

// GetPodResourcesByExperiment returns resource usage for all pods belonging
// to a specific experiment (identified by label selector).
func (r *ResourceMonitor) GetPodResourcesByExperiment(ctx context.Context, namespace, experimentID string) ([]PodResources, error) {
	labelSelector := fmt.Sprintf("chaos-sec/experiment-id=%s", experimentID)

	r.logger.Debug("getting pod resources by experiment",
		zap.String("namespace", namespace),
		zap.String("experiment_id", experimentID),
	)

	podList, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for experiment %s in namespace %s: %w", experimentID, namespace, err)
	}

	podResources := make([]PodResources, 0, len(podList.Items))
	for _, pod := range podList.Items {
		requestedCPU := resource.MustParse("0")
		requestedMemory := resource.MustParse("0")
		limitedCPU := resource.MustParse("0")
		limitedMemory := resource.MustParse("0")

		for _, container := range pod.Spec.Containers {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				requestedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				requestedMemory.Add(mem)
			}
			if cpu, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
				limitedCPU.Add(cpu)
			}
			if mem, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
				limitedMemory.Add(mem)
			}
		}

		pr := PodResources{
			PodName:  pod.Name,
			CPU:      requestedCPU.String(),
			Memory:   requestedMemory.String(),
			CPULimit: limitedCPU.String(),
			MemLimit: limitedMemory.String(),
			Status:   string(pod.Status.Phase),
			NodeName: pod.Spec.NodeName,
		}

		podResources = append(podResources, pr)
	}

	return podResources, nil
}

// CheckResourceAvailability checks whether the cluster has enough available
// resources (at the cluster level) to accommodate the specified CPU and memory
// requirements. It compares the requested resources against the cluster's
// available (allocatable minus requested) resources.
func (r *ResourceMonitor) CheckResourceAvailability(ctx context.Context, namespace string, requiredCPU, requiredMemory resource.Quantity) (bool, error) {
	r.logger.Debug("checking resource availability",
		zap.String("namespace", namespace),
		zap.String("required_cpu", requiredCPU.String()),
		zap.String("required_memory", requiredMemory.String()),
	)

	// If a namespace is specified, check against the namespace quota first.
	if namespace != "" {
		available, err := r.checkNamespaceResourceAvailability(ctx, namespace, requiredCPU, requiredMemory)
		if err != nil {
			r.logger.Warn("failed to check namespace resource availability, falling back to cluster check",
				zap.String("namespace", namespace),
				zap.Error(err),
			)
		} else if !available {
			r.logger.Info("insufficient namespace resources",
				zap.String("namespace", namespace),
				zap.String("required_cpu", requiredCPU.String()),
				zap.String("required_memory", requiredMemory.String()),
			)
			return false, nil
		}
	}

	// Check cluster-level resource availability.
	clusterResources, err := r.GetClusterResources(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get cluster resources for availability check: %w", err)
	}

	cpuAvailable := clusterResources.AvailableCPUQuantity.Cmp(requiredCPU) >= 0
	memAvailable := clusterResources.AvailableMemoryQuantity.Cmp(requiredMemory) >= 0

	available := cpuAvailable && memAvailable

	r.logger.Info("resource availability check result",
		zap.String("namespace", namespace),
		zap.String("required_cpu", requiredCPU.String()),
		zap.String("required_memory", requiredMemory.String()),
		zap.String("available_cpu", clusterResources.AvailableCPUQuantity.String()),
		zap.String("available_memory", clusterResources.AvailableMemoryQuantity.String()),
		zap.Bool("available", available),
	)

	return available, nil
}

// checkNamespaceResourceAvailability checks whether a namespace has enough
// remaining quota to accommodate the specified resource requirements.
func (r *ResourceMonitor) checkNamespaceResourceAvailability(ctx context.Context, namespace string, requiredCPU, requiredMemory resource.Quantity) (bool, error) {
	quotaList, err := r.client.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list resource quotas: %w", err)
	}

	// If no quotas, assume namespace has no restrictions.
	if len(quotaList.Items) == 0 {
		return true, nil
	}

	quota := quotaList.Items[0]

	// Check CPU availability under quota.
	if hardCPU, ok := quota.Spec.Hard[corev1.ResourceCPU]; ok {
		if usedCPU, ok := quota.Status.Used[corev1.ResourceCPU]; ok {
			availableCPU := hardCPU.DeepCopy()
			availableCPU.Sub(usedCPU)
			if availableCPU.Cmp(requiredCPU) < 0 {
				r.logger.Debug("insufficient CPU under namespace quota",
					zap.String("namespace", namespace),
					zap.String("quota_hard", hardCPU.String()),
					zap.String("quota_used", usedCPU.String()),
					zap.String("available", availableCPU.String()),
					zap.String("required", requiredCPU.String()),
				)
				return false, nil
			}
		}
	}

	// Check memory availability under quota.
	if hardMem, ok := quota.Spec.Hard[corev1.ResourceMemory]; ok {
		if usedMem, ok := quota.Status.Used[corev1.ResourceMemory]; ok {
			availableMem := hardMem.DeepCopy()
			availableMem.Sub(usedMem)
			if availableMem.Cmp(requiredMemory) < 0 {
				r.logger.Debug("insufficient memory under namespace quota",
					zap.String("namespace", namespace),
					zap.String("quota_hard", hardMem.String()),
					zap.String("quota_used", usedMem.String()),
					zap.String("available", availableMem.String()),
					zap.String("required", requiredMemory.String()),
				)
				return false, nil
			}
		}
	}

	// Check pod count availability under quota.
	if hardPods, ok := quota.Spec.Hard[corev1.ResourcePods]; ok {
		if usedPods, ok := quota.Status.Used[corev1.ResourcePods]; ok {
			availablePods := hardPods.DeepCopy()
			availablePods.Sub(usedPods)
			one := resource.MustParse("1")
			if availablePods.Cmp(one) < 0 {
				r.logger.Debug("insufficient pod quota in namespace",
					zap.String("namespace", namespace),
					zap.String("quota_hard", hardPods.String()),
					zap.String("quota_used", usedPods.String()),
					zap.String("available", availablePods.String()),
				)
				return false, nil
			}
		}
	}

	return true, nil
}

// GetNodeResources returns the resource capacity and allocatable resources
// for each node in the cluster.
func (r *ResourceMonitor) GetNodeResources(ctx context.Context) ([]NodeResourceInfo, error) {
	nodeList, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]NodeResourceInfo, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		info := NodeResourceInfo{
			Name:   node.Name,
			Status: getNodeStatus(node),
		}

		if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
			info.TotalCPU = cpu.String()
			info.TotalCPUQuantity = cpu.DeepCopy()
		}
		if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
			info.TotalMemory = mem.String()
			info.TotalMemoryQuantity = mem.DeepCopy()
		}
		if cpu, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			info.AllocatableCPU = cpu.String()
			info.AllocatableCPUQuantity = cpu.DeepCopy()
		}
		if mem, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			info.AllocatableMemory = mem.String()
			info.AllocatableMemoryQuantity = mem.DeepCopy()
		}
		if pods, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			info.MaxPods = pods.String()
		}

		nodes = append(nodes, info)
	}

	return nodes, nil
}

// NodeResourceInfo holds resource information for a single node.
type NodeResourceInfo struct {
	Name                      string            `json:"name"`
	Status                    string            `json:"status"`
	TotalCPU                  string            `json:"total_cpu"`
	AllocatableCPU            string            `json:"allocatable_cpu"`
	TotalMemory               string            `json:"total_memory"`
	AllocatableMemory         string            `json:"allocatable_memory"`
	MaxPods                   string            `json:"max_pods,omitempty"`
	Labels                    map[string]string `json:"labels,omitempty"`
	TotalCPUQuantity          resource.Quantity `json:"-"`
	AllocatableCPUQuantity    resource.Quantity `json:"-"`
	TotalMemoryQuantity       resource.Quantity `json:"-"`
	AllocatableMemoryQuantity resource.Quantity `json:"-"`
}

// GetClusterSummary returns a simplified summary of cluster health and resources
// suitable for API responses and dashboards.
func (r *ResourceMonitor) GetClusterSummary(ctx context.Context) (*ClusterSummary, error) {
	clusterResources, err := r.GetClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster resources for summary: %w", err)
	}

	nodeList, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes for summary: %w", err)
	}

	readyNodes := 0
	notReadyNodes := 0
	for _, node := range nodeList.Items {
		status := getNodeStatus(node)
		if status == "Ready" {
			readyNodes++
		} else {
			notReadyNodes++
		}
	}

	summary := &ClusterSummary{
		TotalNodes:      len(nodeList.Items),
		ReadyNodes:      readyNodes,
		NotReadyNodes:   notReadyNodes,
		TotalCPU:        clusterResources.TotalCPU,
		AvailableCPU:    clusterResources.AvailableCPU,
		TotalMemory:     clusterResources.TotalMemory,
		AvailableMemory: clusterResources.AvailableMemory,
		TotalPods:       clusterResources.PodCount,
	}

	return summary, nil
}

// ClusterSummary holds a simplified cluster health and resource summary.
type ClusterSummary struct {
	TotalNodes      int    `json:"total_nodes"`
	ReadyNodes      int    `json:"ready_nodes"`
	NotReadyNodes   int    `json:"not_ready_nodes"`
	TotalCPU        string `json:"total_cpu"`
	AvailableCPU    string `json:"available_cpu"`
	TotalMemory     string `json:"total_memory"`
	AvailableMemory string `json:"available_memory"`
	TotalPods       int    `json:"total_pods"`
}
