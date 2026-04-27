package kubernetes

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// ClientManager constructor tests
// ---------------------------------------------------------------------------

func TestNewClientManager(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)
	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}
}

func TestClientManager_RegisterCluster_EmptyKubeconfigPath(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	_, err := cm.RegisterCluster("test-cluster", "")
	if err == nil {
		t.Error("RegisterCluster with empty kubeconfig path should return error")
	}
}

func TestClientManager_RegisterCluster_InvalidPath(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	_, err := cm.RegisterCluster("test-cluster", "/nonexistent/kubeconfig")
	if err == nil {
		t.Error("RegisterCluster with invalid path should return error")
	}
}

func TestClientManager_RegisterClusterFromConfig_NilCluster(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	_, err := cm.RegisterClusterFromConfig(nil)
	if err == nil {
		t.Error("RegisterClusterFromConfig with nil cluster should return error")
	}
}

func TestClientManager_GetClient_NotRegistered(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	_, ok := cm.GetClient("nonexistent")
	if ok {
		t.Error("GetClient for nonexistent cluster should return false")
	}
}

func TestClientManager_RemoveCluster_NotRegistered(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	// Removing a non-existent cluster should return false and not panic.
	removed := cm.RemoveCluster("nonexistent")
	if removed {
		t.Error("RemoveCluster for non-existent cluster should return false")
	}
}

func TestClientManager_ListClusters_Empty(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	clusters := cm.ListClusters()
	if len(clusters) != 0 {
		t.Errorf("ListClusters on empty manager returned %d items, want 0", len(clusters))
	}
}

func TestClientManager_ClusterCount_Empty(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	count := cm.ClusterCount()
	if count != 0 {
		t.Errorf("ClusterCount on empty manager = %d, want 0", count)
	}
}

// ---------------------------------------------------------------------------
// ClusterConfig tests
// ---------------------------------------------------------------------------

func TestClientManager_DefaultClient_Nil(t *testing.T) {
	logger := zap.NewNop()
	cm := NewClientManager(logger)

	// DefaultClient on a new manager should return nil.
	client := cm.DefaultClient()
	if client != nil {
		t.Error("DefaultClient on empty manager should return nil")
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require KUBECONFIG or real K8s cluster)
// ---------------------------------------------------------------------------

func TestNewClusterClient_EmptyPath(t *testing.T) {
	logger := zap.NewNop()
	_, err := NewClusterClient("", logger)
	if err == nil {
		t.Error("NewClusterClient with empty path should return error")
	}
}

func TestNewClusterClient_InvalidPath(t *testing.T) {
	logger := zap.NewNop()
	_, err := NewClusterClient("/nonexistent/kubeconfig", logger)
	if err == nil {
		t.Error("NewClusterClient with invalid path should return error")
	}
}

func TestNewInClusterClient_OutsideCluster(t *testing.T) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		t.Skip("running inside a cluster, skipping out-of-cluster test")
	}
	logger := zap.NewNop()
	_, err := NewInClusterClient(logger)
	if err == nil {
		t.Error("NewInClusterClient outside a cluster should return error")
	}
}
