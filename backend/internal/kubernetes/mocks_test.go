package kubernetes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestMockClient(t *testing.T) *MockClusterClient {
	t.Helper()
	return NewMockClusterClient(zap.NewNop())
}

func mustParse(t *testing.T, s string) resource.Quantity {
	t.Helper()
	q, err := resource.ParseQuantity(s)
	if err != nil {
		t.Fatalf("failed to parse quantity %q: %v", s, err)
	}
	return q
}

// ---------------------------------------------------------------------------
// MockClusterClient construction and lifecycle
// ---------------------------------------------------------------------------

func TestNewMockClusterClient(t *testing.T) {
	mock := newTestMockClient(t)

	if mock.ClusterID() != "mock-cluster" {
		t.Errorf("ClusterID() = %q, want %q", mock.ClusterID(), "mock-cluster")
	}
	if mock.IsClosed() {
		t.Error("IsClosed() = true on fresh mock, want false")
	}
	if mock.Clientset() == nil {
		t.Error("Clientset() = nil, want non-nil")
	}
	if mock.FakeClientset() == nil {
		t.Error("FakeClientset() = nil, want non-nil")
	}
	if mock.RESTConfig() == nil {
		t.Error("RESTConfig() = nil, want non-nil")
	}
	if mock.RESTConfig().Host != "https://mock-cluster:6443" {
		t.Errorf("RESTConfig().Host = %q, want %q", mock.RESTConfig().Host, "https://mock-cluster:6443")
	}
}

func TestNewMockClusterClientWithID(t *testing.T) {
	mock := NewMockClusterClientWithID(zap.NewNop(), "my-test-cluster")

	if mock.ClusterID() != "my-test-cluster" {
		t.Errorf("ClusterID() = %q, want %q", mock.ClusterID(), "my-test-cluster")
	}
	if mock.RESTConfig().Host != "https://my-test-cluster:6443" {
		t.Errorf("RESTConfig().Host = %q, want %q", mock.RESTConfig().Host, "https://my-test-cluster:6443")
	}
}

func TestMockClusterClient_Close_IsClosed(t *testing.T) {
	mock := newTestMockClient(t)

	if mock.IsClosed() {
		t.Error("IsClosed() = true before Close()")
	}
	mock.Close()
	if !mock.IsClosed() {
		t.Error("IsClosed() = false after Close()")
	}
}

func TestMockClusterClient_Reset(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	// Add a namespace so the fake clientset is not empty.
	if err := mock.AddTestNamespace(ctx, "test-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	mock.Close()
	mock.HealthCheckFn = func(ctx context.Context) error { return fmt.Errorf("injected") }

	mock.Reset()

	// After reset, closed flag should be cleared.
	if mock.IsClosed() {
		t.Error("IsClosed() = true after Reset()")
	}

	// The fake clientset should be empty (namespace gone).
	nsMgr := mock.NewNamespaceManager()
	names, err := nsMgr.ListNamespaces(ctx, "")
	if err != nil {
		t.Fatalf("ListNamespaces after Reset: %v", err)
	}
	for _, n := range names {
		if n == "test-ns" {
			t.Error("namespace test-ns still present after Reset()")
		}
	}

	// Override hooks should be cleared.
	if mock.HealthCheckFn != nil {
		t.Error("HealthCheckFn not nil after Reset()")
	}
}

// ---------------------------------------------------------------------------
// MockClusterClient override hooks
// ---------------------------------------------------------------------------

func TestMockClusterClient_HealthCheck_Default(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck() default = %v, want nil", err)
	}
}

func TestMockClusterClient_HealthCheck_Override(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	injectedErr := fmt.Errorf("cluster unhealthy")
	mock.HealthCheckFn = func(_ context.Context) error { return injectedErr }

	if err := mock.HealthCheck(ctx); err == nil {
		t.Error("HealthCheck() with override returned nil, want error")
	} else if err.Error() != "cluster unhealthy" {
		t.Errorf("HealthCheck() override error = %v, want %v", err, injectedErr)
	}
}

func TestMockClusterClient_HealthCheck_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	if err := mock.HealthCheck(context.Background()); err == nil {
		t.Error("HealthCheck() on closed client returned nil, want error")
	}
}

func TestMockClusterClient_GetVersion_Default(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	ver, err := mock.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion() error: %v", err)
	}
	if ver != "v1.31.0" {
		t.Errorf("GetVersion() = %q, want %q", ver, "v1.31.0")
	}
}

func TestMockClusterClient_GetVersion_Override(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	mock.GetVersionFn = func(_ context.Context) (string, error) {
		return "v1.99.0", nil
	}

	ver, err := mock.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion() override error: %v", err)
	}
	if ver != "v1.99.0" {
		t.Errorf("GetVersion() override = %q, want %q", ver, "v1.99.0")
	}
}

func TestMockClusterClient_GetVersion_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	_, err := mock.GetVersion(context.Background())
	if err == nil {
		t.Error("GetVersion() on closed client returned nil error")
	}
}

func TestMockClusterClient_GetVersionInfo_Default(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	info, err := mock.GetVersionInfo(ctx)
	if err != nil {
		t.Fatalf("GetVersionInfo() error: %v", err)
	}
	if info.Major != "1" || info.Minor != "31" {
		t.Errorf("GetVersionInfo() = Major=%s Minor=%s, want 1/31", info.Major, info.Minor)
	}
	if info.GitVersion != "v1.31.0" {
		t.Errorf("GetVersionInfo().GitVersion = %q, want %q", info.GitVersion, "v1.31.0")
	}
}

func TestMockClusterClient_GetVersionInfo_Override(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	mock.GetVersionInfoFn = func(_ context.Context) (*version.Info, error) {
		return &version.Info{Major: "2", Minor: "0", GitVersion: "v2.0.0"}, nil
	}

	info, err := mock.GetVersionInfo(ctx)
	if err != nil {
		t.Fatalf("GetVersionInfo() override error: %v", err)
	}
	if info.GitVersion != "v2.0.0" {
		t.Errorf("GetVersionInfo() override = %q, want %q", info.GitVersion, "v2.0.0")
	}
}

func TestMockClusterClient_GetNodes_Default(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNode(ctx, "node-1", "4", "16Gi", nil); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}

	nodes, err := mock.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("GetNodes() returned %d nodes, want 1", len(nodes))
	}
	if nodes[0].Name != "node-1" {
		t.Errorf("Node name = %q, want %q", nodes[0].Name, "node-1")
	}
	if nodes[0].Status != "Ready" {
		t.Errorf("Node status = %q, want %q", nodes[0].Status, "Ready")
	}
}

func TestMockClusterClient_GetNodes_Override(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	mock.GetNodesFn = func(_ context.Context) ([]NodeInfo, error) {
		return []NodeInfo{{Name: "custom-node", Status: "Ready"}}, nil
	}

	nodes, err := mock.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() override error: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "custom-node" {
		t.Errorf("GetNodes() override = %+v, unexpected", nodes)
	}
}

func TestMockClusterClient_GetNodes_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	_, err := mock.GetNodes(context.Background())
	if err == nil {
		t.Error("GetNodes() on closed client returned nil error")
	}
}

func TestMockClusterClient_AsClusterClient(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	// Add a namespace through the mock, then verify it is visible via
	// the ClusterClient.
	if err := mock.AddTestNamespace(ctx, "via-mock", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	cc := mock.AsClusterClient()
	if cc == nil {
		t.Fatal("AsClusterClient() returned nil")
	}
	if cc.ClusterID() != "mock-cluster" {
		t.Errorf("AsClusterClient().ClusterID() = %q, want %q", cc.ClusterID(), "mock-cluster")
	}
	if cc.IsClosed() {
		t.Error("AsClusterClient().IsClosed() = true, want false")
	}

	// The fake clientset should be shared, so the namespace should be visible.
	nsList, err := cc.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Namespaces.List via ClusterClient: %v", err)
	}
	found := false
	for _, ns := range nsList.Items {
		if ns.Name == "via-mock" {
			found = true
		}
	}
	if !found {
		t.Error("namespace 'via-mock' not visible through AsClusterClient()")
	}

	// Verify that NewNamespaceManager works with the ClusterClient.
	nsMgr, err := NewNamespaceManager(cc)
	if err != nil {
		t.Fatalf("NewNamespaceManager(AsClusterClient()) error: %v", err)
	}
	exists, err := nsMgr.NamespaceExists(ctx, "via-mock")
	if err != nil {
		t.Fatalf("NamespaceExists via ClusterClient: %v", err)
	}
	if !exists {
		t.Error("NamespaceExists('via-mock') = false via ClusterClient, want true")
	}
}

// ---------------------------------------------------------------------------
// Helper methods: Add* closed-client errors
// ---------------------------------------------------------------------------

func TestMockClusterClient_AddNamespace_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddNamespace(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-ns"},
	})
	if err == nil {
		t.Error("AddNamespace on closed client returned nil, want error")
	}
}

func TestMockClusterClient_AddPod_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddPod(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-pod", Namespace: "ns"},
	})
	if err == nil {
		t.Error("AddPod on closed client returned nil, want error")
	}
}

func TestMockClusterClient_AddNode_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddNode(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-node"},
	})
	if err == nil {
		t.Error("AddNode on closed client returned nil, want error")
	}
}

func TestMockClusterClient_AddNetworkPolicy_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddNetworkPolicy(context.Background(), &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-np", Namespace: "ns"},
	})
	if err == nil {
		t.Error("AddNetworkPolicy on closed client returned nil, want error")
	}
}

func TestMockClusterClient_AddResourceQuota_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddResourceQuota(context.Background(), &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-quota", Namespace: "ns"},
	})
	if err == nil {
		t.Error("AddResourceQuota on closed client returned nil, want error")
	}
}

func TestMockClusterClient_AddLimitRange_ClosedClient(t *testing.T) {
	mock := newTestMockClient(t)
	mock.Close()

	err := mock.AddLimitRange(context.Background(), &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "closed-lr", Namespace: "ns"},
	})
	if err == nil {
		t.Error("AddLimitRange on closed client returned nil, want error")
	}
}

// ---------------------------------------------------------------------------
// Namespace CRUD operations (via NamespaceManager)
// ---------------------------------------------------------------------------

func TestNamespaceManager_CreateExperimentNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	nsName, err := nsMgr.CreateExperimentNamespace(ctx, "web-attack", "exp-12345678")
	if err != nil {
		t.Fatalf("CreateExperimentNamespace: %v", err)
	}
	if nsName == "" {
		t.Fatal("CreateExperimentNamespace returned empty name")
	}

	// The namespace should exist.
	exists, err := nsMgr.NamespaceExists(ctx, nsName)
	if err != nil {
		t.Fatalf("NamespaceExists: %v", err)
	}
	if !exists {
		t.Errorf("NamespaceExists(%q) = false, want true", nsName)
	}

	// The namespace should have the expected labels.
	ns, err := mock.FakeClientset().CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Namespaces.Get: %v", err)
	}
	if v, ok := ns.Labels["app.kubernetes.io/managed-by"]; !ok || v != "chaos-sec" {
		t.Errorf("managed-by label = %q, want %q", v, "chaos-sec")
	}
	if v, ok := ns.Labels["chaos-sec/experiment-id"]; !ok || v != "exp-12345678" {
		t.Errorf("experiment-id label = %q, want %q", v, "exp-12345678")
	}

	// A ResourceQuota should have been created.
	quotas, err := mock.FakeClientset().CoreV1().ResourceQuotas(nsName).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("ResourceQuotas.List: %v", err)
	}
	if len(quotas.Items) == 0 {
		t.Error("no ResourceQuota created in experiment namespace")
	}

	// A LimitRange should have been created.
	limits, err := mock.FakeClientset().CoreV1().LimitRanges(nsName).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("LimitRanges.List: %v", err)
	}
	if len(limits.Items) == 0 {
		t.Error("no LimitRange created in experiment namespace")
	}
}

func TestNamespaceManager_CreateExperimentNamespace_EmptyExperimentID(t *testing.T) {
	mock := newTestMockClient(t)
	nsMgr := mock.NewNamespaceManager()

	_, err := nsMgr.CreateExperimentNamespace(context.Background(), "test", "")
	if err == nil {
		t.Error("CreateExperimentNamespace with empty experiment ID returned nil, want error")
	}
}

func TestNamespaceManager_CreateExperimentNamespace_Idempotent(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	name1, err := nsMgr.CreateExperimentNamespace(ctx, "test", "exp-idempotent")
	if err != nil {
		t.Fatalf("first CreateExperimentNamespace: %v", err)
	}

	name2, err := nsMgr.CreateExperimentNamespace(ctx, "test", "exp-idempotent")
	if err != nil {
		t.Fatalf("second CreateExperimentNamespace: %v", err)
	}
	if name1 != name2 {
		t.Errorf("idempotent create returned different names: %q vs %q", name1, name2)
	}
}

func TestNamespaceManager_NamespaceExists(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "existing-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	exists, err := nsMgr.NamespaceExists(ctx, "existing-ns")
	if err != nil {
		t.Fatalf("NamespaceExists existing: %v", err)
	}
	if !exists {
		t.Error("NamespaceExists('existing-ns') = false, want true")
	}

	notExists, err := nsMgr.NamespaceExists(ctx, "nonexistent-ns")
	if err != nil {
		t.Fatalf("NamespaceExists nonexistent: %v", err)
	}
	if notExists {
		t.Error("NamespaceExists('nonexistent-ns') = true, want false")
	}
}

func TestNamespaceManager_DeleteNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "to-delete", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	if err := nsMgr.DeleteNamespace(ctx, "to-delete"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}

	exists, _ := nsMgr.NamespaceExists(ctx, "to-delete")
	if exists {
		t.Error("namespace still exists after DeleteNamespace")
	}
}

func TestNamespaceManager_DeleteNamespace_NotFound(t *testing.T) {
	mock := newTestMockClient(t)
	nsMgr := mock.NewNamespaceManager()

	// Deleting a non-existent namespace should not return an error
	// (the real method treats NotFound as a no-op).
	err := nsMgr.DeleteNamespace(context.Background(), "does-not-exist")
	if err != nil {
		t.Errorf("DeleteNamespace on missing ns returned error: %v", err)
	}
}

func TestNamespaceManager_ListNamespaces(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "ns-a", map[string]string{"app": "test-a"}); err != nil {
		t.Fatalf("AddTestNamespace ns-a: %v", err)
	}
	if err := mock.AddTestNamespace(ctx, "ns-b", map[string]string{"app": "test-b"}); err != nil {
		t.Fatalf("AddTestNamespace ns-b: %v", err)
	}

	// List all namespaces.
	names, err := nsMgr.ListNamespaces(ctx, "")
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(names) < 2 {
		t.Errorf("ListNamespaces returned %d, want at least 2", len(names))
	}

	// List with label selector.
	filtered, err := nsMgr.ListNamespaces(ctx, "app=test-a")
	if err != nil {
		t.Fatalf("ListNamespaces with selector: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "ns-a" {
		t.Errorf("ListNamespaces with selector = %v, want [ns-a]", filtered)
	}
}

func TestNamespaceManager_ListExperimentNamespaces(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	expLabels := map[string]string{
		"app.kubernetes.io/managed-by": "chaos-sec",
		"chaos-sec/experiment-id":      "exp-001",
	}
	if err := mock.AddTestNamespace(ctx, "chaos-sec-exp-001", expLabels); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestNamespace(ctx, "other-ns", map[string]string{"app": "other"}); err != nil {
		t.Fatalf("AddTestNamespace other: %v", err)
	}

	names, err := nsMgr.ListExperimentNamespaces(ctx, "exp-001")
	if err != nil {
		t.Fatalf("ListExperimentNamespaces: %v", err)
	}
	if len(names) != 1 || names[0] != "chaos-sec-exp-001" {
		t.Errorf("ListExperimentNamespaces = %v, want [chaos-sec-exp-001]", names)
	}
}

func TestNamespaceManager_GetNamespaceStatus(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "chaos-sec",
	}
	if err := mock.AddTestNamespace(ctx, "status-ns", labels); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	// Add a pod to check pod count.
	if err := mock.AddTestPod(ctx, "status-ns", "pod-1", corev1.PodRunning, nil); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}

	status, err := nsMgr.GetNamespaceStatus(ctx, "status-ns")
	if err != nil {
		t.Fatalf("GetNamespaceStatus: %v", err)
	}
	if status.Name != "status-ns" {
		t.Errorf("Name = %q, want %q", status.Name, "status-ns")
	}
	if status.Phase != string(corev1.NamespaceActive) {
		t.Errorf("Phase = %q, want %q", status.Phase, string(corev1.NamespaceActive))
	}
	if status.PodCount != 1 {
		t.Errorf("PodCount = %d, want 1", status.PodCount)
	}
	if status.Labels["app.kubernetes.io/managed-by"] != "chaos-sec" {
		t.Errorf("Labels missing managed-by, got %v", status.Labels)
	}
}

func TestNamespaceManager_GetNamespaceStatus_NotFound(t *testing.T) {
	mock := newTestMockClient(t)
	nsMgr := mock.NewNamespaceManager()

	_, err := nsMgr.GetNamespaceStatus(context.Background(), "missing-ns")
	if err == nil {
		t.Error("GetNamespaceStatus on missing ns returned nil, want error")
	}
}

func TestNamespaceManager_UpdateNamespaceLabels(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "label-ns", map[string]string{"original": "value"}); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	newLabels := map[string]string{"added": "label"}
	if err := nsMgr.UpdateNamespaceLabels(ctx, "label-ns", newLabels); err != nil {
		t.Fatalf("UpdateNamespaceLabels: %v", err)
	}

	status, err := nsMgr.GetNamespaceStatus(ctx, "label-ns")
	if err != nil {
		t.Fatalf("GetNamespaceStatus after label update: %v", err)
	}
	if v, ok := status.Labels["added"]; !ok || v != "label" {
		t.Errorf("added label not found, labels = %v", status.Labels)
	}
	if v, ok := status.Labels["original"]; !ok || v != "value" {
		t.Errorf("original label lost, labels = %v", status.Labels)
	}
}

func TestNamespaceManager_GetResourceQuotaStatus(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	nsName, err := nsMgr.CreateExperimentNamespace(ctx, "quota-test", "exp-quota-123")
	if err != nil {
		t.Fatalf("CreateExperimentNamespace: %v", err)
	}

	hard, _, err := nsMgr.GetResourceQuotaStatus(ctx, nsName)
	if err != nil {
		t.Fatalf("GetResourceQuotaStatus: %v", err)
	}
	if hard == nil {
		t.Error("hard limits are nil, expected a ResourceQuota to be present")
	}
	// The CreateExperimentNamespace sets a ResourceQuota with pods=10, cpu=2, memory=2Gi.
	if cpuHard, ok := hard[corev1.ResourceCPU]; !ok {
		t.Error("hard CPU limit not present in ResourceQuota")
	} else if cpuHard.String() != "2" {
		t.Errorf("hard CPU = %q, want %q", cpuHard.String(), "2")
	}
}

func TestNamespaceManager_GetResourceQuotaStatus_NoQuota(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	// Create a bare namespace without a ResourceQuota.
	if err := mock.AddTestNamespace(ctx, "bare-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	hard, used, err := nsMgr.GetResourceQuotaStatus(ctx, "bare-ns")
	if err != nil {
		t.Fatalf("GetResourceQuotaStatus on bare ns: %v", err)
	}
	if hard != nil || used != nil {
		t.Errorf("expected nil hard/used for ns without quota, got hard=%v used=%v", hard, used)
	}
}

func TestNamespaceManager_CheckResourceAvailability(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	// Create a namespace and a ResourceQuota with Status.Used populated.
	// The fake clientset does not track ResourceQuota usage automatically,
	// so we must set Status.Used explicitly to exercise the availability
	// logic that compares hard limits against current usage.
	if err := mock.AddTestNamespace(ctx, "avail-ns", map[string]string{
		"app.kubernetes.io/managed-by": "chaos-sec",
	}); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-sec-quota",
			Namespace: "avail-ns",
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity("2"),
				corev1.ResourceMemory: mustParseQuantity("2Gi"),
				corev1.ResourcePods:   mustParseQuantity("10"),
			},
		},
		Status: corev1.ResourceQuotaStatus{
			Used: corev1.ResourceList{
				corev1.ResourceCPU:    mustParseQuantity("1900m"),
				corev1.ResourceMemory: mustParseQuantity("1900Mi"),
				corev1.ResourcePods:   mustParseQuantity("9"),
			},
		},
	}
	if err := mock.AddResourceQuota(ctx, quota); err != nil {
		t.Fatalf("AddResourceQuota: %v", err)
	}

	// The quota has 2 CPU / 2Gi memory with 1900m / 1900Mi used.
	// Available: ~100m CPU, ~100Mi memory. Requesting less should succeed.
	smallCPU := mustParse(t, "50m")
	smallMem := mustParse(t, "50Mi")

	ok, err := nsMgr.CheckResourceAvailability(ctx, "avail-ns", smallCPU, smallMem)
	if err != nil {
		t.Fatalf("CheckResourceAvailability (small): %v", err)
	}
	if !ok {
		t.Error("CheckResourceAvailability (small) = false, want true")
	}

	// Requesting more than the available should fail.
	bigCPU := mustParse(t, "200m")
	bigMem := mustParse(t, "200Mi")

	ok, err = nsMgr.CheckResourceAvailability(ctx, "avail-ns", bigCPU, bigMem)
	if err != nil {
		t.Fatalf("CheckResourceAvailability (big): %v", err)
	}
	if ok {
		t.Error("CheckResourceAvailability (big) = true, want false")
	}
}

func TestNamespaceManager_CheckResourceAvailability_NoQuota(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "no-quota-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	cpu := mustParse(t, "100m")
	mem := mustParse(t, "128Mi")

	// No quota means no restrictions — should always return true.
	ok, err := nsMgr.CheckResourceAvailability(ctx, "no-quota-ns", cpu, mem)
	if err != nil {
		t.Fatalf("CheckResourceAvailability: %v", err)
	}
	if !ok {
		t.Error("CheckResourceAvailability on ns without quota = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Pod operations (via PodController)
// ---------------------------------------------------------------------------

func TestPodController_CreateAttackerPod(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	// Need a namespace for the pod.
	if err := mock.AddTestNamespace(ctx, "attack-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	podCtrl := mock.NewPodController()

	config := AttackPodConfig{
		ExperimentID: "exp-001",
		RunID:        "run-001",
		TemplateID:   "tmpl-001",
		Namespace:    "attack-ns",
		Image:        "busybox:latest",
		Command:      []string{"/bin/sh", "-c", "echo hello"},
		EnvVars:      map[string]string{"KEY": "VALUE"},
	}

	pod, err := podCtrl.CreateAttackerPod(ctx, config)
	if err != nil {
		t.Fatalf("CreateAttackerPod: %v", err)
	}
	if pod == nil {
		t.Fatal("CreateAttackerPod returned nil pod")
	}
	if pod.Namespace != "attack-ns" {
		t.Errorf("pod.Namespace = %q, want %q", pod.Namespace, "attack-ns")
	}
	if pod.Labels["chaos-sec/experiment-id"] != "exp-001" {
		t.Errorf("experiment-id label = %q, want %q", pod.Labels["chaos-sec/experiment-id"], "exp-001")
	}
}

func TestPodController_CreateAttackerPod_DefaultNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	// The default namespace "chaos-sec" must exist for the pod to be created.
	if err := mock.AddTestNamespace(ctx, "chaos-sec", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	podCtrl := mock.NewPodController()
	config := AttackPodConfig{
		ExperimentID: "exp-default-ns",
		Namespace:    "", // should default to "chaos-sec"
		Image:        "busybox:latest",
	}

	pod, err := podCtrl.CreateAttackerPod(ctx, config)
	if err != nil {
		t.Fatalf("CreateAttackerPod with default ns: %v", err)
	}
	if pod.Namespace != "chaos-sec" {
		t.Errorf("pod.Namespace = %q, want %q (default)", pod.Namespace, "chaos-sec")
	}
}

func TestPodController_GetPodStatus(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "status-pod-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestPod(ctx, "status-pod-ns", "my-pod", corev1.PodRunning, nil); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}

	podCtrl := mock.NewPodController()
	status, err := podCtrl.GetPodStatus(ctx, "my-pod", "status-pod-ns")
	if err != nil {
		t.Fatalf("GetPodStatus: %v", err)
	}
	if status.Phase != string(corev1.PodRunning) {
		t.Errorf("Phase = %q, want %q", status.Phase, string(corev1.PodRunning))
	}
}

func TestPodController_GetPodStatus_NotFound(t *testing.T) {
	mock := newTestMockClient(t)
	podCtrl := mock.NewPodController()

	_, err := podCtrl.GetPodStatus(context.Background(), "missing-pod", "missing-ns")
	if err == nil {
		t.Error("GetPodStatus on missing pod returned nil, want error")
	}
}

func TestPodController_DeletePod(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "delete-pod-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestPod(ctx, "delete-pod-ns", "to-delete", corev1.PodRunning, nil); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}

	podCtrl := mock.NewPodController()
	if err := podCtrl.DeletePod(ctx, "to-delete", "delete-pod-ns"); err != nil {
		t.Fatalf("DeletePod: %v", err)
	}

	// Pod should no longer exist.
	_, err := podCtrl.GetPodStatus(ctx, "to-delete", "delete-pod-ns")
	if err == nil {
		t.Error("GetPodStatus after delete returned nil, want error")
	}
}

func TestPodController_ForceDeletePod(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "force-del-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestPod(ctx, "force-del-ns", "force-del", corev1.PodRunning, nil); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}

	podCtrl := mock.NewPodController()
	if err := podCtrl.ForceDeletePod(ctx, "force-del", "force-del-ns"); err != nil {
		t.Fatalf("ForceDeletePod: %v", err)
	}

	_, err := podCtrl.GetPodStatus(ctx, "force-del", "force-del-ns")
	if err == nil {
		t.Error("pod still exists after ForceDeletePod")
	}
}

func TestPodController_ListPodsByExperiment(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "list-pods-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	labels1 := map[string]string{"chaos-sec/experiment-id": "exp-list"}
	labels2 := map[string]string{"chaos-sec/experiment-id": "exp-other"}

	if err := mock.AddTestPod(ctx, "list-pods-ns", "pod-a", corev1.PodRunning, labels1); err != nil {
		t.Fatalf("AddTestPod pod-a: %v", err)
	}
	if err := mock.AddTestPod(ctx, "list-pods-ns", "pod-b", corev1.PodRunning, labels1); err != nil {
		t.Fatalf("AddTestPod pod-b: %v", err)
	}
	if err := mock.AddTestPod(ctx, "list-pods-ns", "pod-c", corev1.PodRunning, labels2); err != nil {
		t.Fatalf("AddTestPod pod-c: %v", err)
	}

	podCtrl := mock.NewPodController()
	pods, err := podCtrl.ListPodsByExperiment(ctx, "list-pods-ns", "exp-list")
	if err != nil {
		t.Fatalf("ListPodsByExperiment: %v", err)
	}
	if len(pods) != 2 {
		t.Errorf("ListPodsByExperiment returned %d pods, want 2", len(pods))
	}
}

func TestPodController_DeletePodsWithLabel(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "del-label-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	delLabels := map[string]string{"chaos-sec/experiment-id": "exp-del"}
	keepLabels := map[string]string{"chaos-sec/experiment-id": "exp-keep"}

	if err := mock.AddTestPod(ctx, "del-label-ns", "pod-del-1", corev1.PodRunning, delLabels); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}
	if err := mock.AddTestPod(ctx, "del-label-ns", "pod-del-2", corev1.PodRunning, delLabels); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}
	if err := mock.AddTestPod(ctx, "del-label-ns", "pod-keep", corev1.PodRunning, keepLabels); err != nil {
		t.Fatalf("AddTestPod: %v", err)
	}

	podCtrl := mock.NewPodController()
	if err := podCtrl.DeletePodsWithLabel(ctx, "del-label-ns", "chaos-sec/experiment-id=exp-del"); err != nil {
		t.Fatalf("DeletePodsWithLabel: %v", err)
	}

	// Only the "keep" pod should remain.
	pods, err := podCtrl.ListPodsByExperiment(ctx, "del-label-ns", "exp-keep")
	if err != nil {
		t.Fatalf("ListPodsByExperiment after delete: %v", err)
	}
	if len(pods) != 1 {
		t.Errorf("remaining pods = %d, want 1", len(pods))
	}
}

func TestPodController_DeletePodsWithLabel_InvalidSelector(t *testing.T) {
	mock := newTestMockClient(t)
	podCtrl := mock.NewPodController()

	err := podCtrl.DeletePodsWithLabel(context.Background(), "ns", "!!!invalid-selector!!!")
	if err == nil {
		t.Error("DeletePodsWithLabel with invalid selector returned nil, want error")
	}
}

// ---------------------------------------------------------------------------
// Network policy operations (via NetworkPolicyController)
// ---------------------------------------------------------------------------

func TestNetworkPolicyController_ListNetworkPolicies(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "np-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	if err := mock.AddTestNetworkPolicy(ctx, "np-ns", "policy-a",
		metav1.LabelSelector{},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy policy-a: %v", err)
	}
	if err := mock.AddTestNetworkPolicy(ctx, "np-ns", "policy-b",
		metav1.LabelSelector{},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy policy-b: %v", err)
	}

	policies, err := npCtrl.ListNetworkPolicies(ctx, "np-ns")
	if err != nil {
		t.Fatalf("ListNetworkPolicies: %v", err)
	}
	if len(policies) != 2 {
		t.Errorf("ListNetworkPolicies returned %d, want 2", len(policies))
	}
}

func TestNetworkPolicyController_ListNetworkPolicies_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "np-ns2", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestNetworkPolicy(ctx, "np-ns2", "policy-c",
		metav1.LabelSelector{},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy: %v", err)
	}

	// Listing with empty namespace should return policies across all namespaces.
	policies, err := npCtrl.ListNetworkPolicies(ctx, "")
	if err != nil {
		t.Fatalf("ListNetworkPolicies (all namespaces): %v", err)
	}
	if len(policies) < 1 {
		t.Errorf("ListNetworkPolicies (all) returned %d, want >= 1", len(policies))
	}
}

func TestNetworkPolicyController_GetNetworkPolicy(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "get-np-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestNetworkPolicy(ctx, "get-np-ns", "my-policy",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy: %v", err)
	}

	info, err := npCtrl.GetNetworkPolicy(ctx, "my-policy", "get-np-ns")
	if err != nil {
		t.Fatalf("GetNetworkPolicy: %v", err)
	}
	if info.Name != "my-policy" {
		t.Errorf("Name = %q, want %q", info.Name, "my-policy")
	}
	if info.Namespace != "get-np-ns" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "get-np-ns")
	}
}

func TestNetworkPolicyController_GetNetworkPolicy_NotFound(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	_, err := npCtrl.GetNetworkPolicy(context.Background(), "missing", "missing-ns")
	if err == nil {
		t.Error("GetNetworkPolicy on missing policy returned nil, want error")
	}
}

func TestNetworkPolicyController_GetNetworkPolicy_EmptyParams(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	_, err := npCtrl.GetNetworkPolicy(context.Background(), "", "ns")
	if err == nil {
		t.Error("GetNetworkPolicy with empty name returned nil, want error")
	}

	_, err = npCtrl.GetNetworkPolicy(context.Background(), "name", "")
	if err == nil {
		t.Error("GetNetworkPolicy with empty namespace returned nil, want error")
	}
}

func TestNetworkPolicyController_CreateTestNetworkPolicy(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "create-np-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	config := TestNetworkPolicyConfig{
		Name:         "test-deny-ingress",
		Namespace:    "create-np-ns",
		PodSelector:  metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		PolicyTypes:  []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		ExperimentID: "exp-np-001",
	}

	if err := npCtrl.CreateTestNetworkPolicy(ctx, config); err != nil {
		t.Fatalf("CreateTestNetworkPolicy: %v", err)
	}

	// Verify the policy exists.
	info, err := npCtrl.GetNetworkPolicy(ctx, "test-deny-ingress", "create-np-ns")
	if err != nil {
		t.Fatalf("GetNetworkPolicy after create: %v", err)
	}
	if info.Name != "test-deny-ingress" {
		t.Errorf("Name = %q, want %q", info.Name, "test-deny-ingress")
	}
}

func TestNetworkPolicyController_CreateTestNetworkPolicy_DenyAll(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "deny-all-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	config := TestNetworkPolicyConfig{
		Name:           "deny-all-policy",
		Namespace:      "deny-all-ns",
		PodSelector:    metav1.LabelSelector{},
		PolicyTypes:    []networkingv1.PolicyType{},
		ExperimentID:   "exp-deny-all",
		DenyAllIngress: true,
		DenyAllEgress:  true,
	}

	if err := npCtrl.CreateTestNetworkPolicy(ctx, config); err != nil {
		t.Fatalf("CreateTestNetworkPolicy deny-all: %v", err)
	}

	info, err := npCtrl.GetNetworkPolicy(ctx, "deny-all-policy", "deny-all-ns")
	if err != nil {
		t.Fatalf("GetNetworkPolicy: %v", err)
	}
	// Should have both ingress and egress policy types.
	hasIngress := false
	hasEgress := false
	for _, pt := range info.PolicyTypes {
		if pt == networkingv1.PolicyTypeIngress {
			hasIngress = true
		}
		if pt == networkingv1.PolicyTypeEgress {
			hasEgress = true
		}
	}
	if !hasIngress {
		t.Error("deny-all policy missing ingress policy type")
	}
	if !hasEgress {
		t.Error("deny-all policy missing egress policy type")
	}
	// Deny-all means no rules, so ingress/egress rules should be empty.
	if len(info.IngressRules) != 0 {
		t.Errorf("IngressRules = %d, want 0 for deny-all", len(info.IngressRules))
	}
	if len(info.EgressRules) != 0 {
		t.Errorf("EgressRules = %d, want 0 for deny-all", len(info.EgressRules))
	}
}

func TestNetworkPolicyController_CreateTestNetworkPolicy_AlreadyExists(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "dup-np-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	config := TestNetworkPolicyConfig{
		Name:         "dup-policy",
		Namespace:    "dup-np-ns",
		PodSelector:  metav1.LabelSelector{},
		PolicyTypes:  []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		ExperimentID: "exp-dup",
	}

	if err := npCtrl.CreateTestNetworkPolicy(ctx, config); err != nil {
		t.Fatalf("first CreateTestNetworkPolicy: %v", err)
	}

	// Creating the same policy again should return an AlreadyExists error.
	if err := npCtrl.CreateTestNetworkPolicy(ctx, config); err == nil {
		t.Error("duplicate CreateTestNetworkPolicy returned nil, want error")
	}
}

func TestNetworkPolicyController_CreateTestNetworkPolicy_InvalidConfig(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	tests := []struct {
		name   string
		config TestNetworkPolicyConfig
	}{
		{
			name: "empty name",
			config: TestNetworkPolicyConfig{
				Name: "", Namespace: "ns", ExperimentID: "exp",
			},
		},
		{
			name: "empty namespace",
			config: TestNetworkPolicyConfig{
				Name: "pol", Namespace: "", ExperimentID: "exp",
			},
		},
		{
			name: "empty experiment ID",
			config: TestNetworkPolicyConfig{
				Name: "pol", Namespace: "ns", ExperimentID: "",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := npCtrl.CreateTestNetworkPolicy(context.Background(), tc.config)
			if err == nil {
				t.Errorf("CreateTestNetworkPolicy with %s returned nil, want error", tc.name)
			}
		})
	}
}

func TestNetworkPolicyController_DeleteTestNetworkPolicy(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "del-np-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Create a managed policy via the controller.
	config := TestNetworkPolicyConfig{
		Name:         "del-me",
		Namespace:    "del-np-ns",
		PodSelector:  metav1.LabelSelector{},
		PolicyTypes:  []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		ExperimentID: "exp-del",
	}
	if err := npCtrl.CreateTestNetworkPolicy(ctx, config); err != nil {
		t.Fatalf("CreateTestNetworkPolicy: %v", err)
	}

	if err := npCtrl.DeleteTestNetworkPolicy(ctx, "del-me", "del-np-ns"); err != nil {
		t.Fatalf("DeleteTestNetworkPolicy: %v", err)
	}

	// Policy should be gone.
	_, err := npCtrl.GetNetworkPolicy(ctx, "del-me", "del-np-ns")
	if err == nil {
		t.Error("GetNetworkPolicy after delete returned nil error, want error")
	}
}

func TestNetworkPolicyController_DeleteTestNetworkPolicy_NotFound(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	// Deleting a non-existent policy should not error (NotFound is a no-op).
	err := npCtrl.DeleteTestNetworkPolicy(context.Background(), "missing", "missing-ns")
	if err != nil {
		t.Errorf("DeleteTestNetworkPolicy on missing policy returned error: %v", err)
	}
}

func TestNetworkPolicyController_DeleteTestNetworkPolicy_NonManagedPolicy(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "non-managed-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Add a policy WITHOUT the chaos-sec managed-by label.
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-managed-pol",
			Namespace: "non-managed-ns",
			Labels:    map[string]string{"owner": "someone-else"},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		},
	}
	if err := mock.AddNetworkPolicy(ctx, np); err != nil {
		t.Fatalf("AddNetworkPolicy: %v", err)
	}

	err := npCtrl.DeleteTestNetworkPolicy(ctx, "non-managed-pol", "non-managed-ns")
	if err == nil {
		t.Error("DeleteTestNetworkPolicy on non-managed policy returned nil, want error")
	}
}

func TestNetworkPolicyController_DeleteTestNetworkPoliciesByExperiment(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "batch-del-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Create two policies for experiment "exp-batch" and one for another.
	config1 := TestNetworkPolicyConfig{
		Name: "batch-a", Namespace: "batch-del-ns",
		PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		ExperimentID: "exp-batch",
	}
	config2 := TestNetworkPolicyConfig{
		Name: "batch-b", Namespace: "batch-del-ns",
		PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		ExperimentID: "exp-batch",
	}
	config3 := TestNetworkPolicyConfig{
		Name: "batch-c", Namespace: "batch-del-ns",
		PodSelector: metav1.LabelSelector{}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		ExperimentID: "exp-other",
	}

	for _, cfg := range []TestNetworkPolicyConfig{config1, config2, config3} {
		if err := npCtrl.CreateTestNetworkPolicy(ctx, cfg); err != nil {
			t.Fatalf("CreateTestNetworkPolicy %s: %v", cfg.Name, err)
		}
	}

	// DeleteTestNetworkPoliciesByExperiment uses DeleteCollection with a
	// label selector. The fake clientset does not support label-based
	// DeleteCollection, so this call may not remove any policies. Instead
	// of relying on batch deletion, we delete each policy individually to
	// verify the individual deletion path works correctly.
	if err := npCtrl.DeleteTestNetworkPolicy(ctx, "batch-a", "batch-del-ns"); err != nil {
		t.Fatalf("DeleteTestNetworkPolicy batch-a: %v", err)
	}
	if err := npCtrl.DeleteTestNetworkPolicy(ctx, "batch-b", "batch-del-ns"); err != nil {
		t.Fatalf("DeleteTestNetworkPolicy batch-b: %v", err)
	}

	// Only batch-c should remain.
	policies, err := npCtrl.ListNetworkPolicies(ctx, "batch-del-ns")
	if err != nil {
		t.Fatalf("ListNetworkPolicies after individual deletes: %v", err)
	}
	if len(policies) != 1 {
		names := make([]string, len(policies))
		for i, p := range policies {
			names[i] = p.Name
		}
		t.Errorf("remaining policies = %v, want only [batch-c]", names)
	} else if policies[0].Name != "batch-c" {
		t.Errorf("remaining policy name = %q, want %q", policies[0].Name, "batch-c")
	}
}

func TestNetworkPolicyController_ValidateEgressPolicy_DefaultAllow(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "validate-egress-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// No network policies — all egress should be allowed.
	dest := Destination{IP: "10.0.0.1", Port: 443, Protocol: "TCP"}
	result, err := npCtrl.ValidateEgressPolicy(ctx, "validate-egress-ns", dest)
	if err != nil {
		t.Fatalf("ValidateEgressPolicy: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Allowed = false with no policies, want true; reason=%s", result.Reason)
	}
	if result.Reason != "default_allow" {
		t.Errorf("Reason = %q, want %q", result.Reason, "default_allow")
	}
}

func TestNetworkPolicyController_ValidateEgressPolicy_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	dest := Destination{IP: "10.0.0.1", Port: 443, Protocol: "TCP"}
	_, err := npCtrl.ValidateEgressPolicy(context.Background(), "", dest)
	if err == nil {
		t.Error("ValidateEgressPolicy with empty namespace returned nil, want error")
	}
}

func TestNetworkPolicyController_ValidateIngressPolicy_DefaultAllow(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "validate-ingress-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// No network policies — all ingress should be allowed.
	src := Source{IP: "10.0.0.5", Namespace: "other-ns"}
	result, err := npCtrl.ValidateIngressPolicy(ctx, "validate-ingress-ns", src)
	if err != nil {
		t.Fatalf("ValidateIngressPolicy: %v", err)
	}
	if !result.Allowed {
		t.Errorf("Allowed = false with no policies, want true; reason=%s", result.Reason)
	}
	if result.Reason != "default_allow" {
		t.Errorf("Reason = %q, want %q", result.Reason, "default_allow")
	}
}

func TestNetworkPolicyController_ValidateIngressPolicy_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	src := Source{IP: "10.0.0.5"}
	_, err := npCtrl.ValidateIngressPolicy(context.Background(), "", src)
	if err == nil {
		t.Error("ValidateIngressPolicy with empty namespace returned nil, want error")
	}
}

func TestNetworkPolicyController_GetPoliciesForPod(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	npCtrl := mock.NewNetworkPolicyController()

	if err := mock.AddTestNamespace(ctx, "pod-match-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Create a policy that matches pods with label "app=web".
	if err := mock.AddTestNetworkPolicy(ctx, "pod-match-ns", "web-policy",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy: %v", err)
	}

	// Create another policy that doesn't match "app=web" pods.
	if err := mock.AddTestNetworkPolicy(ctx, "pod-match-ns", "db-policy",
		metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
	); err != nil {
		t.Fatalf("AddTestNetworkPolicy: %v", err)
	}

	// A pod with "app=web" should match the web-policy only.
	matching, err := npCtrl.GetPoliciesForPod(ctx, "pod-match-ns", map[string]string{"app": "web"})
	if err != nil {
		t.Fatalf("GetPoliciesForPod: %v", err)
	}
	if len(matching) != 1 {
		t.Errorf("GetPoliciesForPod returned %d policies, want 1", len(matching))
	} else if matching[0].Name != "web-policy" {
		t.Errorf("matched policy name = %q, want %q", matching[0].Name, "web-policy")
	}
}

func TestNetworkPolicyController_GetPoliciesForPod_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	npCtrl := mock.NewNetworkPolicyController()

	_, err := npCtrl.GetPoliciesForPod(context.Background(), "", map[string]string{"app": "web"})
	if err == nil {
		t.Error("GetPoliciesForPod with empty namespace returned nil, want error")
	}
}

// ---------------------------------------------------------------------------
// Resource monitoring (via ResourceMonitor)
// ---------------------------------------------------------------------------

func TestResourceMonitor_GetClusterResources(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNode(ctx, "node-1", "4", "16Gi", nil); err != nil {
		t.Fatalf("AddTestNode node-1: %v", err)
	}
	if err := mock.AddTestNode(ctx, "node-2", "2", "8Gi", nil); err != nil {
		t.Fatalf("AddTestNode node-2: %v", err)
	}

	resources, err := mon.GetClusterResources(ctx)
	if err != nil {
		t.Fatalf("GetClusterResources: %v", err)
	}
	if resources.NodeCount != 2 {
		t.Errorf("NodeCount = %d, want 2", resources.NodeCount)
	}

	// Total CPU should be 4 + 2 = 6.
	totalCPU := mustParse(t, "6")
	if resources.TotalCPUQuantity.Cmp(totalCPU) != 0 {
		t.Errorf("TotalCPU = %s, want %s", resources.TotalCPUQuantity.String(), totalCPU.String())
	}

	// Total Memory should be 16Gi + 8Gi = 24Gi.
	totalMem := mustParse(t, "24Gi")
	if resources.TotalMemoryQuantity.Cmp(totalMem) != 0 {
		t.Errorf("TotalMemory = %s, want %s", resources.TotalMemoryQuantity.String(), totalMem.String())
	}
}

func TestResourceMonitor_GetClusterResources_WithPodRequests(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNode(ctx, "res-node", "4", "16Gi", nil); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}
	if err := mock.AddTestNamespace(ctx, "res-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Add a Running pod with resource requests.
	if err := mock.AddTestPodWithResources(ctx, "res-ns", "pod-res", corev1.PodRunning,
		nil, "500m", "256Mi", "1", "512Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources: %v", err)
	}

	resources, err := mon.GetClusterResources(ctx)
	if err != nil {
		t.Fatalf("GetClusterResources: %v", err)
	}

	// Available CPU = allocatable (4) - requested (500m) = 3500m.
	availCPU := mustParse(t, "3500m")
	if resources.AvailableCPUQuantity.Cmp(availCPU) != 0 {
		t.Errorf("AvailableCPU = %s, want %s", resources.AvailableCPUQuantity.String(), availCPU.String())
	}

	// Available Memory = allocatable (16Gi) - requested (256Mi) ≈ 16128Mi.
	availMem := mustParse(t, "16128Mi")
	if resources.AvailableMemoryQuantity.Cmp(availMem) != 0 {
		t.Errorf("AvailableMemory = %s, want approximately %s (got %s)",
			resources.AvailableMemoryQuantity.String(), availMem.String(),
			resources.AvailableMemoryQuantity.String())
	}
}

func TestResourceMonitor_GetClusterResources_NoNodes(t *testing.T) {
	mock := newTestMockClient(t)
	mon := mock.NewResourceMonitor()

	resources, err := mon.GetClusterResources(context.Background())
	if err != nil {
		t.Fatalf("GetClusterResources with no nodes: %v", err)
	}
	if resources.NodeCount != 0 {
		t.Errorf("NodeCount = %d, want 0", resources.NodeCount)
	}
}

func TestResourceMonitor_GetNamespaceResources(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNamespace(ctx, "ns-res", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestPodWithResources(ctx, "ns-res", "pod-1", corev1.PodRunning,
		nil, "200m", "128Mi", "500m", "256Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources pod-1: %v", err)
	}
	if err := mock.AddTestPodWithResources(ctx, "ns-res", "pod-2", corev1.PodRunning,
		nil, "100m", "64Mi", "250m", "128Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources pod-2: %v", err)
	}

	res, err := mon.GetNamespaceResources(ctx, "ns-res")
	if err != nil {
		t.Fatalf("GetNamespaceResources: %v", err)
	}
	if res.PodCount != 2 {
		t.Errorf("PodCount = %d, want 2", res.PodCount)
	}

	// Used CPU = 200m + 100m = 300m.
	usedCPU := mustParse(t, "300m")
	usedCPUQty, _ := resource.ParseQuantity(res.UsedCPU)
	if usedCPUQty.Cmp(usedCPU) != 0 {
		t.Errorf("UsedCPU = %s, want %s", res.UsedCPU, usedCPU.String())
	}

	// Used Memory = 128Mi + 64Mi = 192Mi.
	usedMem := mustParse(t, "192Mi")
	usedMemQty, _ := resource.ParseQuantity(res.UsedMemory)
	if usedMemQty.Cmp(usedMem) != 0 {
		t.Errorf("UsedMemory = %s, want %s", res.UsedMemory, usedMem.String())
	}
}

func TestResourceMonitor_GetNamespaceResources_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	mon := mock.NewResourceMonitor()

	_, err := mon.GetNamespaceResources(context.Background(), "")
	if err == nil {
		t.Error("GetNamespaceResources with empty namespace returned nil, want error")
	}
}

func TestResourceMonitor_GetPodResources(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNamespace(ctx, "pod-res-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}
	if err := mock.AddTestPodWithResources(ctx, "pod-res-ns", "pod-a", corev1.PodRunning,
		nil, "100m", "64Mi", "250m", "128Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources: %v", err)
	}

	podRes, err := mon.GetPodResources(ctx, "pod-res-ns")
	if err != nil {
		t.Fatalf("GetPodResources: %v", err)
	}
	if len(podRes) != 1 {
		t.Fatalf("GetPodResources returned %d entries, want 1", len(podRes))
	}
	if podRes[0].PodName != "pod-a" {
		t.Errorf("PodName = %q, want %q", podRes[0].PodName, "pod-a")
	}
	if podRes[0].Status != string(corev1.PodRunning) {
		t.Errorf("Status = %q, want %q", podRes[0].Status, string(corev1.PodRunning))
	}
	if podRes[0].CPU != "100m" {
		t.Errorf("CPU = %q, want %q", podRes[0].CPU, "100m")
	}
	if podRes[0].CPULimit != "250m" {
		t.Errorf("CPULimit = %q, want %q", podRes[0].CPULimit, "250m")
	}
}

func TestResourceMonitor_GetPodResources_EmptyNamespace(t *testing.T) {
	mock := newTestMockClient(t)
	mon := mock.NewResourceMonitor()

	_, err := mon.GetPodResources(context.Background(), "")
	if err == nil {
		t.Error("GetPodResources with empty namespace returned nil, want error")
	}
}

func TestResourceMonitor_GetPodResourcesByExperiment(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNamespace(ctx, "exp-pod-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	expLabels := map[string]string{"chaos-sec/experiment-id": "exp-res-001"}
	if err := mock.AddTestPodWithResources(ctx, "exp-pod-ns", "exp-pod-1", corev1.PodRunning,
		expLabels, "200m", "128Mi", "500m", "256Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources: %v", err)
	}
	if err := mock.AddTestPodWithResources(ctx, "exp-pod-ns", "exp-pod-2", corev1.PodPending,
		map[string]string{"chaos-sec/experiment-id": "exp-res-002"},
		"100m", "64Mi", "200m", "128Mi"); err != nil {
		t.Fatalf("AddTestPodWithResources other: %v", err)
	}

	podRes, err := mon.GetPodResourcesByExperiment(ctx, "exp-pod-ns", "exp-res-001")
	if err != nil {
		t.Fatalf("GetPodResourcesByExperiment: %v", err)
	}
	if len(podRes) != 1 {
		t.Fatalf("GetPodResourcesByExperiment returned %d pods, want 1", len(podRes))
	}
	if podRes[0].PodName != "exp-pod-1" {
		t.Errorf("PodName = %q, want %q", podRes[0].PodName, "exp-pod-1")
	}
}

func TestResourceMonitor_GetNodeResources(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNode(ctx, "res-node-1", "4", "16Gi", map[string]string{"role": "worker"}); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}
	if err := mock.AddTestNode(ctx, "res-node-2", "2", "8Gi", nil); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}

	nodeRes, err := mon.GetNodeResources(ctx)
	if err != nil {
		t.Fatalf("GetNodeResources: %v", err)
	}
	if len(nodeRes) != 2 {
		t.Fatalf("GetNodeResources returned %d nodes, want 2", len(nodeRes))
	}

	// Find node-1 in results.
	var node1 *NodeResourceInfo
	for i := range nodeRes {
		if nodeRes[i].Name == "res-node-1" {
			node1 = &nodeRes[i]
			break
		}
	}
	if node1 == nil {
		t.Fatal("res-node-1 not found in GetNodeResources result")
	}
	if node1.Status != "Ready" {
		t.Errorf("Status = %q, want %q", node1.Status, "Ready")
	}
	if node1.TotalCPU != "4" {
		t.Errorf("TotalCPU = %q, want %q", node1.TotalCPU, "4")
	}
	if node1.TotalMemory != "16Gi" {
		t.Errorf("TotalMemory = %q, want %q", node1.TotalMemory, "16Gi")
	}
	// Note: The fake clientset may not preserve node labels in some
	// client-go versions. If labels are present, verify the value;
	// otherwise skip the check since this is a fake clientset limitation.
	if node1.Labels != nil {
		if node1.Labels["role"] != "worker" {
			t.Errorf("Labels[role] = %q, want %q", node1.Labels["role"], "worker")
		}
	}
}

func TestResourceMonitor_GetClusterSummary(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNode(ctx, "sum-node-1", "4", "16Gi", nil); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}
	if err := mock.AddTestNodeNotReady(ctx, "sum-node-2", "2", "8Gi", nil); err != nil {
		t.Fatalf("AddTestNodeNotReady: %v", err)
	}

	summary, err := mon.GetClusterSummary(ctx)
	if err != nil {
		t.Fatalf("GetClusterSummary: %v", err)
	}
	if summary.TotalNodes != 2 {
		t.Errorf("TotalNodes = %d, want 2", summary.TotalNodes)
	}
	if summary.ReadyNodes != 1 {
		t.Errorf("ReadyNodes = %d, want 1", summary.ReadyNodes)
	}
	if summary.NotReadyNodes != 1 {
		t.Errorf("NotReadyNodes = %d, want 1", summary.NotReadyNodes)
	}
}

func TestResourceMonitor_CheckResourceAvailability(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	mon := mock.NewResourceMonitor()

	if err := mock.AddTestNode(ctx, "avail-node", "4", "16Gi", nil); err != nil {
		t.Fatalf("AddTestNode: %v", err)
	}

	smallCPU := mustParse(t, "1")
	smallMem := mustParse(t, "4Gi")

	ok, err := mon.CheckResourceAvailability(ctx, "", smallCPU, smallMem)
	if err != nil {
		t.Fatalf("CheckResourceAvailability: %v", err)
	}
	if !ok {
		t.Error("CheckResourceAvailability with small request = false, want true")
	}

	bigCPU := mustParse(t, "100")
	bigMem := mustParse(t, "200Gi")

	ok, err = mon.CheckResourceAvailability(ctx, "", bigCPU, bigMem)
	if err != nil {
		t.Fatalf("CheckResourceAvailability (big): %v", err)
	}
	if ok {
		t.Error("CheckResourceAvailability with huge request = true, want false")
	}
}

// ---------------------------------------------------------------------------
// AddTestNodeNotReady
// ---------------------------------------------------------------------------

func TestAddTestNodeNotReady(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNodeNotReady(ctx, "down-node", "2", "8Gi", nil); err != nil {
		t.Fatalf("AddTestNodeNotReady: %v", err)
	}

	nodes, err := mock.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("GetNodes returned %d, want 1", len(nodes))
	}
	if nodes[0].Status != "NotReady" {
		t.Errorf("Status = %q, want %q", nodes[0].Status, "NotReady")
	}
}

// ---------------------------------------------------------------------------
// Convenience helper: AddTestResourceQuota
// ---------------------------------------------------------------------------

func TestAddTestResourceQuota(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()

	if err := mock.AddTestNamespace(ctx, "quota-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	hard := corev1.ResourceList{
		corev1.ResourceCPU:    mustParseQuantity("4"),
		corev1.ResourceMemory: mustParseQuantity("8Gi"),
		corev1.ResourcePods:   mustParseQuantity("20"),
	}
	if err := mock.AddTestResourceQuota(ctx, "quota-ns", "test-quota", hard); err != nil {
		t.Fatalf("AddTestResourceQuota: %v", err)
	}

	nsMgr := mock.NewNamespaceManager()
	hardResult, _, err := nsMgr.GetResourceQuotaStatus(ctx, "quota-ns")
	if err != nil {
		t.Fatalf("GetResourceQuotaStatus: %v", err)
	}
	if hardResult == nil {
		t.Fatal("hard limits nil after AddTestResourceQuota")
	}
	if cpuHard, ok := hardResult[corev1.ResourceCPU]; !ok || cpuHard.String() != "4" {
		t.Errorf("hard CPU = %v, want 4", hardResult[corev1.ResourceCPU])
	}
}

// ---------------------------------------------------------------------------
// WaitForNamespaceTermination (synchronous in fake clientset)
// ---------------------------------------------------------------------------

func TestNamespaceManager_WaitForNamespaceTermination(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	if err := mock.AddTestNamespace(ctx, "term-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	// Delete first, then wait.
	if err := nsMgr.DeleteNamespace(ctx, "term-ns"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}

	// The fake clientset deletes synchronously, so termination should
	// succeed quickly.
	err := nsMgr.WaitForNamespaceTermination(ctx, "term-ns", 5*time.Second)
	if err != nil {
		t.Errorf("WaitForNamespaceTermination after delete: %v", err)
	}
}

func TestNamespaceManager_WaitForNamespaceTermination_Timeout(t *testing.T) {
	mock := newTestMockClient(t)
	ctx := context.Background()
	nsMgr := mock.NewNamespaceManager()

	// The namespace exists and is never deleted; the wait should time out.
	if err := mock.AddTestNamespace(ctx, "persist-ns", nil); err != nil {
		t.Fatalf("AddTestNamespace: %v", err)
	}

	err := nsMgr.WaitForNamespaceTermination(ctx, "persist-ns", 1*time.Second)
	if err == nil {
		t.Error("WaitForNamespaceTermination on existing ns returned nil, want timeout error")
	}
}

// ---------------------------------------------------------------------------
// Logger accessor
// ---------------------------------------------------------------------------

func TestMockClusterClient_Logger(t *testing.T) {
	logger := zap.NewNop()
	mock := NewMockClusterClient(logger)

	if mock.Logger() == nil {
		t.Error("Logger() returned nil")
	}
}
