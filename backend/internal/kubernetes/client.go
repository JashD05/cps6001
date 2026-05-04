package kubernetes

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chaos-sec/backend/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// NodeInfo holds summarized information about a Kubernetes node.
type NodeInfo struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	CPUCapacity    string            `json:"cpu_capacity"`
	MemoryCapacity string            `json:"memory_capacity"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// ClusterClient wraps a Kubernetes clientset with configuration and metadata.
// It provides a high-level interface for interacting with a single Kubernetes cluster.
type ClusterClient struct {
	clientset  kubernetes.Interface
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger

	mu     sync.RWMutex
	closed bool
}

// NewClusterClient creates a ClusterClient from a kubeconfig file path.
// The kubeconfig file must be a valid Kubernetes configuration file in YAML or JSON format.
func NewClusterClient(kubeconfigPath string, logger *zap.Logger) (*ClusterClient, error) {
	if kubeconfigPath == "" {
		return nil, fmt.Errorf("kubeconfig path must not be empty")
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	configOverrides := &clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config from kubeconfig at %s: %w", kubeconfigPath, err)
	}

	return newClusterClientFromRestConfig(restConfig, "", logger)
}

// NewInClusterClient creates a ClusterClient using the in-cluster configuration.
// This should be used when the application is running as a Pod inside a Kubernetes cluster.
func NewInClusterClient(logger *zap.Logger) (*ClusterClient, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	return newClusterClientFromRestConfig(restConfig, "", logger)
}

// NewClusterClientFromConfig creates a ClusterClient from a stored KubernetesCluster model.
// This is used when connecting to a cluster that has been registered in the database
// with its API endpoint, CA certificate, and client certificates.
func NewClusterClientFromConfig(cluster *models.KubernetesCluster, logger *zap.Logger) (*ClusterClient, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster configuration must not be nil")
	}

	if cluster.APIEndpoint == "" {
		return nil, fmt.Errorf("cluster API endpoint must not be empty")
	}

	restConfig := &rest.Config{
		Host: cluster.APIEndpoint,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
		},
		Timeout: 30 * time.Second,
		Burst:   100,
		QPS:     50,
	}

	// Load CA certificate for server verification.
	if cluster.CACertificate != "" {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM([]byte(cluster.CACertificate)) {
			return nil, fmt.Errorf("failed to parse CA certificate for cluster %s", cluster.ID)
		}
		restConfig.TLSClientConfig.CAData = []byte(cluster.CACertificate)
	}

	// Load client certificate and key for mutual TLS authentication.
	if cluster.ClientCertificate != "" && cluster.ClientKey != "" {
		// Validate the client certificate by parsing it.
		certPEMBlock, _ := pem.Decode([]byte(cluster.ClientCertificate))
		if certPEMBlock == nil {
			return nil, fmt.Errorf("failed to parse client certificate PEM for cluster %s", cluster.ID)
		}
		if _, err := x509.ParseCertificate(certPEMBlock.Bytes); err != nil {
			return nil, fmt.Errorf("failed to parse client certificate for cluster %s: %w", cluster.ID, err)
		}

		// Validate the client key by attempting to parse it.
		keyPEMBlock, _ := pem.Decode([]byte(cluster.ClientKey))
		if keyPEMBlock == nil {
			return nil, fmt.Errorf("failed to parse client key PEM for cluster %s", cluster.ID)
		}

		restConfig.TLSClientConfig.CertData = []byte(cluster.ClientCertificate)
		restConfig.TLSClientConfig.KeyData = []byte(cluster.ClientKey)
		_ = keyPEMBlock // keyPEMBlock is validated by PEM decode; actual use is via KeyData
	}

	// Set up a transport that validates the server certificate.
	if restConfig.Transport == nil && restConfig.TLSClientConfig.CAData != nil {
		restConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			if ht, ok := rt.(*http.Transport); ok {
				ht.TLSClientConfig.RootCAs = x509.NewCertPool()
				ht.TLSClientConfig.RootCAs.AppendCertsFromPEM(restConfig.TLSClientConfig.CAData)
			}
			return rt
		}
	}

	return newClusterClientFromRestConfig(restConfig, cluster.ID.String(), logger)
}

// newClusterClientFromRestConfig is the internal constructor that creates a ClusterClient
// from a validated rest.Config. It configures sensible defaults for QPS, burst, and timeouts.
func newClusterClientFromRestConfig(restConfig *rest.Config, clusterID string, logger *zap.Logger) (*ClusterClient, error) {
	// Apply sensible defaults if not already set.
	if restConfig.QPS == 0 {
		restConfig.QPS = 50
	}
	if restConfig.Burst == 0 {
		restConfig.Burst = 100
	}
	if restConfig.Timeout == 0 {
		restConfig.Timeout = 30 * time.Second
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	cc := &ClusterClient{
		clientset:  clientset,
		restConfig: restConfig,
		clusterID:  clusterID,
		logger:     logger.Named("cluster_client"),
		closed:     false,
	}

	cc.logger.Info("kubernetes cluster client created",
		zap.String("cluster_id", clusterID),
		zap.String("host", restConfig.Host),
	)

	return cc, nil
}

// Clientset returns the underlying Kubernetes clientset for direct API access.
func (c *ClusterClient) Clientset() kubernetes.Interface {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientset
}

// RESTConfig returns a copy of the REST configuration used by this client.
func (c *ClusterClient) RESTConfig() *rest.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return rest.CopyConfig(c.restConfig)
}

// ClusterID returns the identifier of the cluster this client is connected to.
func (c *ClusterClient) ClusterID() string {
	return c.clusterID
}

// Close marks the client as closed. Future operations should check IsClosed().
func (c *ClusterClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.logger.Info("kubernetes cluster client closed", zap.String("cluster_id", c.clusterID))
}

// IsClosed returns whether the client has been closed.
func (c *ClusterClient) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// HealthCheck verifies that the Kubernetes API server is reachable and responsive.
// It performs a lightweight request to the /healthz endpoint of the API server.
func (c *ClusterClient) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return fmt.Errorf("client for cluster %s is closed", c.clusterID)
	}

	_, err := c.clientset.Discovery().RESTClient().Get().AbsPath("/healthz").DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("kubernetes API health check failed for cluster %s: %w", c.clusterID, err)
	}

	return nil
}

// GetVersion returns the Kubernetes version string of the cluster.
func (c *ClusterClient) GetVersion(ctx context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return "", fmt.Errorf("client for cluster %s is closed", c.clusterID)
	}

	versionInfo, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get kubernetes version for cluster %s: %w", c.clusterID, err)
	}

	return fmt.Sprintf("%s.%s", versionInfo.Major, versionInfo.Minor), nil
}

// GetVersionInfo returns the full Kubernetes version information.
func (c *ClusterClient) GetVersionInfo(ctx context.Context) (*version.Info, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, fmt.Errorf("client for cluster %s is closed", c.clusterID)
	}

	versionInfo, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes version info for cluster %s: %w", c.clusterID, err)
	}

	return versionInfo, nil
}

// GetNodes returns information about all nodes in the cluster.
func (c *ClusterClient) GetNodes(ctx context.Context) ([]NodeInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, fmt.Errorf("client for cluster %s is closed", c.clusterID)
	}

	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes for cluster %s: %w", c.clusterID, err)
	}

	nodes := make([]NodeInfo, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodeInfo := NodeInfo{
			Name:   node.Name,
			Status: getNodeStatus(node),
			Labels: node.Labels,
		}

		if cpu, ok := node.Status.Capacity[corev1.ResourceCPU]; ok {
			nodeInfo.CPUCapacity = cpu.String()
		}
		if mem, ok := node.Status.Capacity[corev1.ResourceMemory]; ok {
			nodeInfo.MemoryCapacity = mem.String()
		}

		nodes = append(nodes, nodeInfo)
	}

	return nodes, nil
}

// getNodeStatus determines the overall status of a node from its conditions.
func getNodeStatus(node corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// ClientManager manages connections to multiple Kubernetes clusters.
// It provides a thread-safe cache of ClusterClient instances, allowing the
// application to interact with multiple clusters concurrently.
//
// For backward compatibility with code that uses a single-cluster pattern
// (e.g., the experiment Engine), ClientManager also maintains a "default"
// cluster client. Methods like CreateNamespace, CreatePod, etc. delegate
// to this default client. Call SetDefaultCluster or rely on the first
// registered cluster becoming the default.
type ClientManager struct {
	clients sync.Map // map[string]*ClusterClient
	logger  *zap.Logger

	mu            sync.Mutex // protects registration/removal operations for atomicity
	defaultClient *ClusterClient
}

// NewClientManager creates a new ClientManager for managing multiple cluster connections.
func NewClientManager(logger *zap.Logger) *ClientManager {
	return &ClientManager{
		logger: logger.Named("client_manager"),
	}
}

// SetDefaultCluster sets the default cluster client used by backward-compatible
// methods (CreateNamespace, CreatePod, etc.). If no default is explicitly set,
// the first cluster registered becomes the default.
func (m *ClientManager) SetDefaultCluster(clusterID string) {
	if client, ok := m.GetClient(clusterID); ok {
		m.mu.Lock()
		m.defaultClient = client
		m.mu.Unlock()
		m.logger.Info("default cluster set", zap.String("cluster_id", clusterID))
	}
}

// DefaultClient returns the default ClusterClient, or nil if none is set.
func (m *ClientManager) DefaultClient() *ClusterClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.defaultClient
}

// GetClient retrieves a cached ClusterClient for the given cluster ID.
// Returns the client and true if found, or nil and false if not cached.
func (m *ClientManager) GetClient(clusterID string) (*ClusterClient, bool) {
	val, ok := m.clients.Load(clusterID)
	if !ok {
		return nil, false
	}

	client, ok := val.(*ClusterClient)
	if !ok {
		m.logger.Error("invalid client type in cache",
			zap.String("cluster_id", clusterID),
		)
		m.clients.Delete(clusterID)
		return nil, false
	}

	if client.IsClosed() {
		m.logger.Warn("cached client is closed, removing from cache",
			zap.String("cluster_id", clusterID),
		)
		m.clients.Delete(clusterID)
		return nil, false
	}

	return client, true
}

// RegisterCluster creates and caches a new ClusterClient for the given cluster
// using a kubeconfig file path. If a client for this clusterID already exists
// and is not closed, it is returned. Otherwise a new one is created.
func (m *ClientManager) RegisterCluster(clusterID, kubeconfigPath string) (*ClusterClient, error) {
	// Check if we already have a healthy client.
	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring the lock (prevent double creation).
	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	client, err := NewClusterClient(kubeconfigPath, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client for %s: %w", clusterID, err)
	}

	client.clusterID = clusterID
	m.clients.Store(clusterID, client)

	// If no default client is set, make this the default.
	if m.defaultClient == nil {
		m.defaultClient = client
	}

	m.logger.Info("cluster client registered",
		zap.String("cluster_id", clusterID),
	)

	return client, nil
}

// RegisterClusterFromConfig creates and caches a new ClusterClient from a stored
// KubernetesCluster model. If a client for this clusterID already exists
// and is not closed, it is returned. Otherwise a new one is created.
func (m *ClientManager) RegisterClusterFromConfig(cluster *models.KubernetesCluster) (*ClusterClient, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster configuration must not be nil")
	}

	clusterID := cluster.ID.String()

	// Check if we already have a healthy client.
	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring the lock.
	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	client, err := NewClusterClientFromConfig(cluster, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client from config for %s: %w", clusterID, err)
	}

	m.clients.Store(clusterID, client)

	// If no default client is set, make this the default.
	if m.defaultClient == nil {
		m.defaultClient = client
	}

	m.logger.Info("cluster client registered from stored config",
		zap.String("cluster_id", clusterID),
		zap.String("api_endpoint", cluster.APIEndpoint),
	)

	return client, nil
}

// RegisterInCluster creates and caches a ClusterClient using in-cluster configuration.
func (m *ClientManager) RegisterInCluster(clusterID string) (*ClusterClient, error) {
	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.GetClient(clusterID); ok {
		return existing, nil
	}

	client, err := NewInClusterClient(m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster client for %s: %w", clusterID, err)
	}

	client.clusterID = clusterID
	m.clients.Store(clusterID, client)

	// If no default client is set, make this the default.
	if m.defaultClient == nil {
		m.defaultClient = client
	}

	m.logger.Info("in-cluster client registered",
		zap.String("cluster_id", clusterID),
	)

	return client, nil
}

// RemoveCluster removes a cluster client from the cache and closes it.
// It returns true if the client was found and removed, false otherwise.
func (m *ClientManager) RemoveCluster(clusterID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	val, loaded := m.clients.LoadAndDelete(clusterID)
	if !loaded {
		return false
	}

	client, ok := val.(*ClusterClient)
	if ok {
		client.Close()

		// Clear defaultClient if this was the default.
		if m.defaultClient != nil && m.defaultClient.ClusterID() == clusterID {
			m.defaultClient = nil
		}
	}

	m.logger.Info("cluster client removed",
		zap.String("cluster_id", clusterID),
	)

	return true
}

// ListClusters returns the IDs of all currently cached cluster clients.
func (m *ClientManager) ListClusters() []string {
	var clusterIDs []string
	m.clients.Range(func(key, value interface{}) bool {
		if id, ok := key.(string); ok {
			clusterIDs = append(clusterIDs, id)
		}
		return true
	})
	return clusterIDs
}

// ClusterCount returns the number of active cached cluster clients.
func (m *ClientManager) ClusterCount() int {
	count := 0
	m.clients.Range(func(key, value interface{}) bool {
		if client, ok := value.(*ClusterClient); ok && !client.IsClosed() {
			count++
		}
		return true
	})
	return count
}

// CloseAll removes and closes all cached cluster clients.
func (m *ClientManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients.Range(func(key, value interface{}) bool {
		if client, ok := value.(*ClusterClient); ok {
			client.Close()
		}
		m.clients.Delete(key)
		return true
	})

	m.defaultClient = nil

	m.logger.Info("all cluster clients closed and removed from cache")
}

// HealthCheckAll performs a health check on all cached cluster clients
// and returns a map of cluster IDs to their health check results.
func (m *ClientManager) HealthCheckAll(ctx context.Context) map[string]error {
	results := make(map[string]error)

	m.clients.Range(func(key, value interface{}) bool {
		clusterID, ok := key.(string)
		if !ok {
			return true
		}

		client, ok := value.(*ClusterClient)
		if !ok {
			results[clusterID] = fmt.Errorf("invalid client type in cache")
			return true
		}

		if client.IsClosed() {
			results[clusterID] = fmt.Errorf("client is closed")
			return true
		}

		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		results[clusterID] = client.HealthCheck(checkCtx)
		return true
	})

	return results
}

// ---------------------------------------------------------------------------
// Backward-compatible methods for single-cluster usage (engine.go compat)
// ---------------------------------------------------------------------------

// These methods delegate to the default cluster client's PodController and
// NamespaceManager. They exist for backward compatibility with code that was
// written for the old single-cluster ClientManager (e.g., experiment/engine.go).

// getClientOrFail returns the default ClusterClient or an error if none is set.
func (m *ClientManager) getClientOrFail() (*ClusterClient, error) {
	m.mu.Lock()
	client := m.defaultClient
	m.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("no default cluster client configured; register a cluster first")
	}
	if client.IsClosed() {
		return nil, fmt.Errorf("default cluster client is closed")
	}
	return client, nil
}

// CreateNamespace creates an isolated namespace for an experiment run.
// The namespace is labeled with metadata for tracking and cleanup.
// This is a backward-compatible method that delegates to NamespaceManager.
func (m *ClientManager) CreateNamespace(ctx context.Context, name string, labels map[string]string) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	nsMgr, err := NewNamespaceManager(client)
	if err != nil {
		return fmt.Errorf("failed to create namespace manager: %w", err)
	}

	// Build an experiment ID from labels if available.
	experimentID := ""
	if id, ok := labels["chaos-sec.io/experiment-id"]; ok {
		experimentID = id
	}

	// If the caller provides a name, use CreateExperimentNamespace with that base name.
	nsName, err := nsMgr.CreateExperimentNamespace(ctx, name, experimentID)
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// If the generated name differs from the requested name, add extra labels.
	if nsName != name && len(labels) > 0 {
		_ = nsMgr.UpdateNamespaceLabels(ctx, nsName, labels)
	}

	return nil
}

// DeleteNamespace deletes a namespace and all resources within it.
// This is a backward-compatible method that delegates to NamespaceManager.
func (m *ClientManager) DeleteNamespace(ctx context.Context, name string) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	nsMgr, err := NewNamespaceManager(client)
	if err != nil {
		return fmt.Errorf("failed to create namespace manager: %w", err)
	}

	return nsMgr.DeleteNamespace(ctx, name)
}

// WaitForNamespaceDeletion waits until a namespace is fully deleted.
// This is a backward-compatible method that delegates to NamespaceManager.
func (m *ClientManager) WaitForNamespaceDeletion(ctx context.Context, name string, timeout time.Duration) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	nsMgr, err := NewNamespaceManager(client)
	if err != nil {
		return fmt.Errorf("failed to create namespace manager: %w", err)
	}

	return nsMgr.WaitForNamespaceTermination(ctx, name, timeout)
}

// CreatePod creates a pod in the specified namespace from the provided pod spec.
// This is a backward-compatible method that delegates to the underlying clientset.
func (m *ClientManager) CreatePod(ctx context.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return nil, err
	}

	created, err := client.Clientset().CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod %s/%s: %w", namespace, pod.Name, err)
	}

	m.logger.Info("pod created",
		zap.String("namespace", namespace),
		zap.String("pod_name", created.Name),
	)
	return created, nil
}

// WaitForPodReady waits until a pod reaches the Running phase or the context expires.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) WaitForPodReady(ctx context.Context, namespace, podName string, timeout time.Duration) (*corev1.Pod, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return nil, err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create pod controller: %w", err)
	}

	if err := podCtrl.WaitForPodReady(ctx, podName, namespace, timeout); err != nil {
		return nil, err
	}

	// Fetch the final pod state.
	pod, err := client.Clientset().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s after ready: %w", namespace, podName, err)
	}

	m.logger.Info("pod is ready",
		zap.String("namespace", namespace),
		zap.String("pod_name", podName),
		zap.String("phase", string(pod.Status.Phase)),
	)
	return pod, nil
}

// DeletePod deletes a pod by name from the specified namespace.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) DeletePod(ctx context.Context, namespace, podName string) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return fmt.Errorf("failed to create pod controller: %w", err)
	}

	return podCtrl.ForceDeletePod(ctx, podName, namespace)
}

// DeletePodsByLabel deletes all pods matching the specified label selector in a namespace.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) DeletePodsByLabel(ctx context.Context, namespace, labelSelector string) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return fmt.Errorf("failed to create pod controller: %w", err)
	}

	return podCtrl.DeletePodsWithLabel(ctx, namespace, labelSelector)
}

// GetPodLogs retrieves the logs from a pod's main container.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) GetPodLogs(ctx context.Context, namespace, podName string, tailLines *int64) (string, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return "", err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return "", fmt.Errorf("failed to create pod controller: %w", err)
	}

	opts := &corev1.PodLogOptions{
		Container:  "",
		Follow:     false,
		Previous:   false,
		Timestamps: true,
	}
	if tailLines != nil {
		opts.TailLines = tailLines
	} else {
		defaultTail := int64(500)
		opts.TailLines = &defaultTail
	}

	return podCtrl.GetPodLogs(ctx, podName, namespace, opts)
}

// ExecutePodCommand executes a command inside a running pod container
// and returns the combined stdout/stderr output.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) ExecutePodCommand(ctx context.Context, namespace, podName, container string, command []string) (string, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return "", err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return "", fmt.Errorf("failed to create pod controller: %w", err)
	}

	// Join command parts into a single shell command string.
	cmdStr := ""
	if len(command) > 0 {
		// If the command is /bin/sh -c "..." style, extract the actual command.
		if len(command) >= 3 && command[0] == "/bin/sh" && command[1] == "-c" {
			cmdStr = command[2]
		} else {
			// Join all parts as a single command.
			for i, part := range command {
				if i > 0 {
					cmdStr += " "
				}
				cmdStr += part
			}
		}
	}

	return podCtrl.ExecuteInPod(ctx, podName, namespace, container, cmdStr)
}

// ListPodsByLabel lists pods matching a label selector in a namespace.
// This is a backward-compatible method that delegates to PodController.
func (m *ClientManager) ListPodsByLabel(ctx context.Context, namespace, labelSelector string) (*corev1.PodList, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return nil, err
	}

	return client.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// CleanupExperimentNamespace removes all pods in a namespace and then deletes the namespace.
// This is a backward-compatible method that delegates to PodController and NamespaceManager.
func (m *ClientManager) CleanupExperimentNamespace(ctx context.Context, namespace string) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}

	podCtrl, err := NewPodController(client)
	if err != nil {
		return fmt.Errorf("failed to create pod controller: %w", err)
	}

	nsMgr, err := NewNamespaceManager(client)
	if err != nil {
		return fmt.Errorf("failed to create namespace manager: %w", err)
	}

	m.logger.Info("cleaning up experiment namespace", zap.String("namespace", namespace))

	// Delete all pods with the experiment label.
	if err := podCtrl.DeletePodsWithLabel(ctx, namespace, "chaos-sec.io/type=experiment"); err != nil {
		m.logger.Warn("failed to delete experiment pods during cleanup",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		// Continue with namespace deletion anyway.
	}

	// Also clean up chaos-sec attacker pods.
	if err := podCtrl.DeletePodsWithLabel(ctx, namespace, "app=chaos-sec-attacker"); err != nil {
		m.logger.Warn("failed to delete attacker pods during cleanup",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
	}

	// Delete the namespace.
	if err := nsMgr.DeleteNamespace(ctx, namespace); err != nil {
		return fmt.Errorf("failed to delete namespace during cleanup: %w", err)
	}

	// Wait for namespace deletion (with a reasonable timeout).
	cleanupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := nsMgr.WaitForNamespaceTermination(cleanupCtx, namespace, 2*time.Minute); err != nil {
		m.logger.Warn("namespace deletion did not complete within timeout",
			zap.String("namespace", namespace),
			zap.Error(err),
		)
	} else {
		m.logger.Info("experiment namespace cleaned up", zap.String("namespace", namespace))
	}

	return nil
}

// GetPod retrieves a pod by name from the specified namespace.
// This is a backward-compatible method.
func (m *ClientManager) GetPod(ctx context.Context, namespace, podName string) (*corev1.Pod, error) {
	client, err := m.getClientOrFail()
	if err != nil {
		return nil, err
	}

	return client.Clientset().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
}

// IsPodRunning checks if a pod is in the Running phase.
// This is a backward-compatible method.
func (m *ClientManager) IsPodRunning(ctx context.Context, namespace, podName string) (bool, error) {
	pod, err := m.GetPod(ctx, namespace, podName)
	if err != nil {
		return false, err
	}
	return pod.Status.Phase == corev1.PodRunning, nil
}

// GetPodIP returns the IP address of a running pod.
// This is a backward-compatible method.
func (m *ClientManager) GetPodIP(ctx context.Context, namespace, podName string) (string, error) {
	pod, err := m.GetPod(ctx, namespace, podName)
	if err != nil {
		return "", err
	}
	if pod.Status.PodIP == "" {
		return "", fmt.Errorf("pod %s/%s has no IP address", namespace, podName)
	}
	return pod.Status.PodIP, nil
}

// GetPodNodeName returns the node name where a pod is running.
// This is a backward-compatible method.
func (m *ClientManager) GetPodNodeName(ctx context.Context, namespace, podName string) (string, error) {
	pod, err := m.GetPod(ctx, namespace, podName)
	if err != nil {
		return "", err
	}
	if pod.Spec.NodeName == "" {
		return "", fmt.Errorf("pod %s/%s is not scheduled on a node", namespace, podName)
	}
	return pod.Spec.NodeName, nil
}

// HealthCheck verifies connectivity to the default cluster's Kubernetes API server.
// This is a backward-compatible method.
func (m *ClientManager) HealthCheck(ctx context.Context) error {
	client, err := m.getClientOrFail()
	if err != nil {
		return err
	}
	return client.HealthCheck(ctx)
}

// buildKubeconfigFromCluster creates a client-go api.Config from a models.KubernetesCluster.
// This can be used with clientcmd.BuildFromConfig for programmatic kubeconfig creation.
func buildKubeconfigFromCluster(cluster *models.KubernetesCluster) api.Config {
	clusterName := cluster.Name
	if clusterName == "" {
		clusterName = cluster.ID.String()
	}

	return api.Config{
		Clusters: map[string]*api.Cluster{
			clusterName: {
				Server:                   cluster.APIEndpoint,
				CertificateAuthorityData: []byte(cluster.CACertificate),
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			clusterName: {
				ClientCertificateData: []byte(cluster.ClientCertificate),
				ClientKeyData:         []byte(cluster.ClientKey),
			},
		},
		Contexts: map[string]*api.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: clusterName,
			},
		},
		CurrentContext: clusterName,
	}
}
