package kubernetes

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// MockClusterClient provides a mock Kubernetes cluster client for testing.
// It wraps a k8s.io/client-go/kubernetes/fake clientset to simulate the
// Kubernetes API in memory and provides convenience helpers for setting up
// test data and creating manager instances.
//
// Because this type lives in the same package as ClusterClient, it can
// construct a real *ClusterClient backed by the fake clientset via
// AsClusterClient, which is useful for code paths that accept *ClusterClient
// directly.
//
// All public methods are safe for concurrent use.
type MockClusterClient struct {
	clientset  *fake.Clientset
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger

	mu     sync.RWMutex
	closed bool

	// Configurable override hooks. When non-nil, these are called instead
	// of the default behaviour, allowing tests to inject failures or
	// custom responses.
	HealthCheckFn    func(ctx context.Context) error
	GetVersionFn     func(ctx context.Context) (string, error)
	GetVersionInfoFn func(ctx context.Context) (*version.Info, error)
	GetNodesFn       func(ctx context.Context) ([]NodeInfo, error)
}

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

// NewMockClusterClient creates a MockClusterClient with a fake clientset and
// sensible defaults. The cluster ID is "mock-cluster".
func NewMockClusterClient(logger *zap.Logger) *MockClusterClient {
	return &MockClusterClient{
		clientset:  fake.NewSimpleClientset(),
		restConfig: &rest.Config{Host: "https://mock-cluster:6443"},
		clusterID:  "mock-cluster",
		logger:     logger.Named("mock_cluster_client"),
	}
}

// NewMockClusterClientWithID creates a MockClusterClient with a specific
// cluster ID. This is useful when multiple mocks are in scope and need to
// be distinguished.
func NewMockClusterClientWithID(logger *zap.Logger, clusterID string) *MockClusterClient {
	return &MockClusterClient{
		clientset:  fake.NewSimpleClientset(),
		restConfig: &rest.Config{Host: "https://" + clusterID + ":6443"},
		clusterID:  clusterID,
		logger:     logger.Named("mock_cluster_client"),
	}
}

// ---------------------------------------------------------------------------
// ClusterClient-compatible accessor methods
// ---------------------------------------------------------------------------

// Clientset returns the underlying clientset as kubernetes.Interface, matching
// the ClusterClient.Clientset() signature.
func (m *MockClusterClient) Clientset() kubernetes.Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientset
}

// FakeClientset returns the underlying *fake.Clientset so that test code can
// access fake-specific features such as adding reaction handlers.
func (m *MockClusterClient) FakeClientset() *fake.Clientset {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientset
}

// RESTConfig returns a copy of the mock REST configuration.
func (m *MockClusterClient) RESTConfig() *rest.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return rest.CopyConfig(m.restConfig)
}

// ClusterID returns the mock cluster identifier.
func (m *MockClusterClient) ClusterID() string {
	return m.clusterID
}

// Logger returns the mock logger (useful when constructing managers manually).
func (m *MockClusterClient) Logger() *zap.Logger {
	return m.logger
}

// ---------------------------------------------------------------------------
// Lifecycle methods
// ---------------------------------------------------------------------------

// Close marks the mock client as closed. Subsequent operations that check
// IsClosed will observe the closed state.
func (m *MockClusterClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

// IsClosed reports whether the mock client has been closed.
func (m *MockClusterClient) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// Reset clears all objects from the fake clientset and resets the closed
// flag, returning the mock to a clean state. Override hooks are also cleared.
func (m *MockClusterClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientset = fake.NewSimpleClientset()
	m.closed = false
	m.HealthCheckFn = nil
	m.GetVersionFn = nil
	m.GetVersionInfoFn = nil
	m.GetNodesFn = nil
}

// ---------------------------------------------------------------------------
// ClusterClient-compatible operational methods
// ---------------------------------------------------------------------------

// HealthCheck simulates a Kubernetes API health check. By default it returns
// nil (healthy). Override via HealthCheckFn to inject failures.
func (m *MockClusterClient) HealthCheck(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client for cluster %s is closed", m.clusterID)
	}
	if m.HealthCheckFn != nil {
		return m.HealthCheckFn(ctx)
	}
	return nil
}

// GetVersion returns a mock Kubernetes version string. By default "v1.31.0".
// Override via GetVersionFn for custom behaviour.
func (m *MockClusterClient) GetVersion(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return "", fmt.Errorf("client for cluster %s is closed", m.clusterID)
	}
	if m.GetVersionFn != nil {
		return m.GetVersionFn(ctx)
	}
	return "v1.31.0", nil
}

// GetVersionInfo returns mock Kubernetes version metadata. Override via
// GetVersionInfoFn for custom behaviour.
func (m *MockClusterClient) GetVersionInfo(ctx context.Context) (*version.Info, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, fmt.Errorf("client for cluster %s is closed", m.clusterID)
	}
	if m.GetVersionInfoFn != nil {
		return m.GetVersionInfoFn(ctx)
	}
	return &version.Info{
		Major:        "1",
		Minor:        "31",
		GitVersion:   "v1.31.0",
		GitCommit:    "mock",
		GitTreeState: "clean",
		BuildDate:    "2024-01-01T00:00:00Z",
		GoVersion:    "go1.22.0",
		Compiler:     "gc",
		Platform:     "linux/amd64",
	}, nil
}

// GetNodes returns node information from the fake clientset. Override via
// GetNodesFn for custom behaviour.
func (m *MockClusterClient) GetNodes(ctx context.Context) ([]NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, fmt.Errorf("client for cluster %s is closed", m.clusterID)
	}
	if m.GetNodesFn != nil {
		return m.GetNodesFn(ctx)
	}

	nodeList, err := m.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]NodeInfo, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		ni := NodeInfo{
			Name:   node.Name,
			Status: getNodeStatus(node),
			Labels: node.Labels,
		}
		if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
			ni.CPUCapacity = cpu.String()
		}
		if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
			ni.MemoryCapacity = mem.String()
		}
		nodes = append(nodes, ni)
	}
	return nodes, nil
}

// ---------------------------------------------------------------------------
// Factory methods for creating managers
// ---------------------------------------------------------------------------

// NewNamespaceManager creates a NamespaceManager backed by the mock clientset.
func (m *MockClusterClient) NewNamespaceManager() *NamespaceManager {
	return NewNamespaceManagerFromClientset(m.clientset, m.restConfig, m.clusterID, m.logger)
}

// NewPodController creates a PodController backed by the mock clientset.
func (m *MockClusterClient) NewPodController() *PodController {
	return NewPodControllerFromClientset(m.clientset, m.restConfig, m.clusterID, m.logger)
}

// NewNetworkPolicyController creates a NetworkPolicyController backed by the
// mock clientset.
func (m *MockClusterClient) NewNetworkPolicyController() *NetworkPolicyController {
	return NewNetworkPolicyControllerFromClientset(m.clientset, m.restConfig, m.clusterID, m.logger)
}

// NewResourceMonitor creates a ResourceMonitor backed by the mock clientset.
func (m *MockClusterClient) NewResourceMonitor() *ResourceMonitor {
	return NewResourceMonitorFromClientset(m.clientset, m.restConfig, m.clusterID, m.logger)
}

// ---------------------------------------------------------------------------
// AsClusterClient
// ---------------------------------------------------------------------------

// AsClusterClient returns a real *ClusterClient whose internal clientset is
// the mock's fake clientset. This is useful for code paths that accept
// *ClusterClient directly.
//
// Note: HealthCheck and GetVersion on the returned ClusterClient call the
// fake clientset's Discovery API, which may not behave like a real API
// server. Prefer using the MockClusterClient methods directly when possible.
func (m *MockClusterClient) AsClusterClient() *ClusterClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &ClusterClient{
		clientset:  m.clientset,
		restConfig: m.restConfig,
		clusterID:  m.clusterID,
		logger:     m.logger,
	}
}

// ---------------------------------------------------------------------------
// Low-level helper methods for adding test data
// ---------------------------------------------------------------------------

// AddNamespace inserts a namespace into the fake clientset.
func (m *MockClusterClient) AddNamespace(ctx context.Context, ns *corev1.Namespace) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

// AddPod inserts a pod into the fake clientset. The pod's Namespace field
// determines which namespace it is created in.
func (m *MockClusterClient) AddPod(ctx context.Context, pod *corev1.Pod) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

// AddNode inserts a node into the fake clientset.
func (m *MockClusterClient) AddNode(ctx context.Context, node *corev1.Node) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	return err
}

// AddNetworkPolicy inserts a network policy into the fake clientset. The
// policy's Namespace field determines which namespace it is created in.
func (m *MockClusterClient) AddNetworkPolicy(ctx context.Context, np *networkingv1.NetworkPolicy) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.NetworkingV1().NetworkPolicies(np.Namespace).Create(ctx, np, metav1.CreateOptions{})
	return err
}

// AddResourceQuota inserts a resource quota into the fake clientset.
func (m *MockClusterClient) AddResourceQuota(ctx context.Context, quota *corev1.ResourceQuota) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.CoreV1().ResourceQuotas(quota.Namespace).Create(ctx, quota, metav1.CreateOptions{})
	return err
}

// AddLimitRange inserts a limit range into the fake clientset.
func (m *MockClusterClient) AddLimitRange(ctx context.Context, lr *corev1.LimitRange) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	_, err := m.clientset.CoreV1().LimitRanges(lr.Namespace).Create(ctx, lr, metav1.CreateOptions{})
	return err
}

// ---------------------------------------------------------------------------
// Convenience helpers for creating common test objects
// ---------------------------------------------------------------------------

// AddTestNamespace creates a namespace with the given name and labels. If
// labels is nil, the namespace is created without labels.
func (m *MockClusterClient) AddTestNamespace(ctx context.Context, name string, labels map[string]string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}
	return m.AddNamespace(ctx, ns)
}

// AddTestPod creates a simple pod with a single "main" container (busybox)
// in the specified namespace, with the given name, phase, and labels.
func (m *MockClusterClient) AddTestPod(ctx context.Context, namespace, name string, phase corev1.PodPhase, labels map[string]string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main", Image: "busybox:latest"},
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	return m.AddPod(ctx, pod)
}

// AddTestPodWithResources creates a pod with explicit CPU and memory
// requests and limits, which is necessary for ResourceMonitor queries that
// aggregate resource consumption.
func (m *MockClusterClient) AddTestPodWithResources(
	ctx context.Context,
	namespace, name string,
	phase corev1.PodPhase,
	labels map[string]string,
	cpuReq, memReq, cpuLimit, memLimit string,
) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "busybox:latest",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    mustParseQuantity(cpuReq),
							corev1.ResourceMemory: mustParseQuantity(memReq),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    mustParseQuantity(cpuLimit),
							corev1.ResourceMemory: mustParseQuantity(memLimit),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	return m.AddPod(ctx, pod)
}

// AddTestNode creates a Ready node with the given CPU and memory capacity.
// Both capacity and allocatable are set to the same values. If labels is nil
// the node is created without labels.
func (m *MockClusterClient) AddTestNode(ctx context.Context, name, cpuCapacity, memoryCapacity string, labels map[string]string) error {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity(cpuCapacity),
				corev1.ResourceMemory: mustParseQuantity(memoryCapacity),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity(cpuCapacity),
				corev1.ResourceMemory: mustParseQuantity(memoryCapacity),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	return m.AddNode(ctx, node)
}

// AddTestNodeNotReady creates a node that is in the NotReady state. This is
// useful for testing cluster summary and node resource queries that depend
// on node readiness.
func (m *MockClusterClient) AddTestNodeNotReady(ctx context.Context, name, cpuCapacity, memoryCapacity string, labels map[string]string) error {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity(cpuCapacity),
				corev1.ResourceMemory: mustParseQuantity(memoryCapacity),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity(cpuCapacity),
				corev1.ResourceMemory: mustParseQuantity(memoryCapacity),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}
	return m.AddNode(ctx, node)
}

// AddTestNetworkPolicy creates a network policy labelled as managed by
// chaos-sec in the given namespace.
func (m *MockClusterClient) AddTestNetworkPolicy(
	ctx context.Context,
	namespace, name string,
	podSelector metav1.LabelSelector,
	policyTypes []networkingv1.PolicyType,
) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "chaos-sec",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: policyTypes,
		},
	}
	return m.AddNetworkPolicy(ctx, np)
}

// AddTestResourceQuota creates a resource quota in the given namespace with
// the specified hard limits.
func (m *MockClusterClient) AddTestResourceQuota(
	ctx context.Context,
	namespace, name string,
	hard corev1.ResourceList,
) error {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "chaos-sec",
			},
		},
		Spec: corev1.ResourceQuotaSpec{Hard: hard},
	}
	return m.AddResourceQuota(ctx, quota)
}
