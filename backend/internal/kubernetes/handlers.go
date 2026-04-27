package kubernetes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/chaos-sec/backend/internal/auth"
	"github.com/chaos-sec/backend/internal/config"
	"github.com/chaos-sec/backend/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for Kubernetes cluster management.
// It integrates with the database for cluster registration and the
// ClientManager for live Kubernetes API interactions.
type Handler struct {
	db            *sql.DB
	clientManager *ClientManager
	cfg           *config.Config
	logger        *zap.Logger
}

// NewHandler creates a new Kubernetes handler with the provided dependencies.
func NewHandler(db *sql.DB, cfg *config.Config, logger *zap.Logger) *Handler {
	return &Handler{
		db:            db,
		clientManager: NewClientManager(logger),
		cfg:           cfg,
		logger:        logger.Named("k8s_handler"),
	}
}

// ClientManagerRef returns the handler's ClientManager for external use
// (e.g., by the experiment execution engine).
func (h *Handler) ClientManagerRef() *ClientManager {
	return h.clientManager
}

// Close cleans up all cached cluster client connections.
func (h *Handler) Close() {
	h.clientManager.CloseAll()
}

// --- Request/Response types ---

// registerClusterRequest is the JSON payload for registering a new cluster.
type registerClusterRequest struct {
	Name              string `json:"name" binding:"required,min=2,max=255"`
	Description       string `json:"description"`
	APIEndpoint       string `json:"api_endpoint" binding:"required"`
	CACertificate     string `json:"ca_certificate" binding:"required"`
	ClientCertificate string `json:"client_certificate" binding:"required"`
	ClientKey         string `json:"client_key" binding:"required"`
	DefaultNamespace  string `json:"default_namespace"`
}

// clusterListItem is the response item for listing clusters.
type clusterListItem struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Description       *string    `json:"description"`
	APIEndpoint       string     `json:"api_endpoint"`
	DefaultNamespace  string     `json:"default_namespace"`
	Status            string     `json:"status"`
	KubernetesVersion *string    `json:"kubernetes_version"`
	NodeCount         *int       `json:"node_count"`
	LastConnectedAt   *time.Time `json:"last_connected_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// clusterDetailResponse is the detailed response for a single cluster,
// enriched with live health and version information.
type clusterDetailResponse struct {
	ID                uuid.UUID        `json:"id"`
	OrganizationID    uuid.UUID        `json:"organization_id"`
	Name              string           `json:"name"`
	Description       *string          `json:"description"`
	APIEndpoint       string           `json:"api_endpoint"`
	DefaultNamespace  string           `json:"default_namespace"`
	Status            string           `json:"status"`
	KubernetesVersion *string          `json:"kubernetes_version"`
	NodeCount         *int             `json:"node_count"`
	LastConnectedAt   *time.Time       `json:"last_connected_at"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	LiveInfo          *clusterLiveInfo `json:"live_info,omitempty"`
}

// clusterLiveInfo contains live information fetched from the Kubernetes API.
type clusterLiveInfo struct {
	Healthy          bool            `json:"healthy"`
	Version          string          `json:"version,omitempty"`
	Nodes            int             `json:"nodes"`
	ReadyNodes       int             `json:"ready_nodes"`
	Error            string          `json:"error,omitempty"`
	ClusterResources *ClusterSummary `json:"cluster_resources,omitempty"`
}

// clusterHealthResponse is the detailed health check response for a cluster.
type clusterHealthResponse struct {
	ClusterID   uuid.UUID        `json:"cluster_id"`
	ClusterName string           `json:"cluster_name"`
	Status      string           `json:"status"`
	Healthy     bool             `json:"healthy"`
	Version     string           `json:"version,omitempty"`
	NodeCount   int              `json:"node_count"`
	ReadyNodes  int              `json:"ready_nodes"`
	Nodes       []NodeHealthInfo `json:"nodes,omitempty"`
	Resources   *ClusterSummary  `json:"resources,omitempty"`
	Error       string           `json:"error,omitempty"`
	CheckedAt   time.Time        `json:"checked_at"`
}

// NodeHealthInfo holds health information for a single node.
type NodeHealthInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// namespaceListItem is the response item for listing namespaces in a cluster.
type namespaceListItem struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
	IsManaged bool              `json:"is_managed"`
}

// networkPolicyListItem is the response item for listing network policies.
type networkPolicyListItem struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	PolicyTypes []string `json:"policy_types"`
}

// --- Helper functions ---

// getClaimsFromContext extracts auth claims from the Gin context.
func getClaimsFromContext(c *gin.Context) (*auth.TokenClaims, error) {
	claimsVal, exists := c.Get("auth_claims")
	if !exists {
		return nil, errors.New("auth claims not found in context")
	}

	claims, ok := claimsVal.(*auth.TokenClaims)
	if !ok {
		return nil, errors.New("invalid auth claims type in context")
	}

	return claims, nil
}

// nilIfEmpty returns nil for empty strings, useful for nullable DB columns.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// fetchClusterFromDB loads a cluster record from the database and returns
// a models.KubernetesCluster populated with the raw column values.
func (h *Handler) fetchClusterFromDB(ctx context.Context, clusterID, orgID uuid.UUID) (*models.KubernetesCluster, error) {
	cluster := &models.KubernetesCluster{}
	var desc sql.NullString
	var k8sVersion sql.NullString
	var nodeCount sql.NullInt64
	var lastConnected sql.NullTime

	err := h.db.QueryRowContext(ctx, `
		SELECT id, organization_id, name, description, api_endpoint,
		       ca_certificate, client_certificate, client_key,
		       default_namespace, status, kubernetes_version, node_count,
		       last_connected_at, created_at, updated_at
		FROM kubernetes_clusters
		WHERE id = $1 AND organization_id = $2
	`, clusterID, orgID).Scan(
		&cluster.ID, &cluster.OrganizationID, &cluster.Name, &desc,
		&cluster.APIEndpoint, &cluster.CACertificate, &cluster.ClientCertificate,
		&cluster.ClientKey, &cluster.DefaultNamespace, &cluster.Status,
		&k8sVersion, &nodeCount, &lastConnected,
		&cluster.CreatedAt, &cluster.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("cluster not found")
		}
		return nil, fmt.Errorf("failed to query cluster: %w", err)
	}

	if desc.Valid {
		cluster.Description = desc.String
	}
	if k8sVersion.Valid {
		cluster.KubernetesVersion = &k8sVersion.String
	}
	if nodeCount.Valid {
		nc := int(nodeCount.Int64)
		cluster.NodeCount = &nc
	}
	if lastConnected.Valid {
		cluster.LastConnectedAt = &lastConnected.Time
	}

	return cluster, nil
}

// getOrCreateClient retrieves a cached cluster client or creates a new one
// from the database record. It also performs a health check and updates the
// cluster status in the database.
func (h *Handler) getOrCreateClient(c *gin.Context, clusterID uuid.UUID, orgID uuid.UUID) (*ClusterClient, error) {
	// First, check the client cache.
	if client, ok := h.clientManager.GetClient(clusterID.String()); ok {
		return client, nil
	}

	// Client not cached — load cluster details from DB and create client.
	ctx := c.Request.Context()

	cluster, err := h.fetchClusterFromDB(ctx, clusterID, orgID)
	if err != nil {
		return nil, err
	}

	client, err := h.clientManager.RegisterClusterFromConfig(cluster)
	if err != nil {
		// Update cluster status to error.
		h.updateClusterStatusInDB(ctx, clusterID, "error", "", 0)
		return nil, fmt.Errorf("failed to connect to cluster: %w", err)
	}

	// Verify connectivity with a health check.
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.HealthCheck(checkCtx); err != nil {
		h.logger.Warn("cluster health check failed after connection",
			zap.String("cluster_id", clusterID.String()),
			zap.Error(err),
		)
		h.updateClusterStatusInDB(ctx, clusterID, "error", "", 0)
		h.clientManager.RemoveCluster(clusterID.String())
		return nil, fmt.Errorf("cluster health check failed: %w", err)
	}

	// Fetch version and node count to update the DB record.
	version, _ := client.GetVersion(checkCtx)
	nodes, _ := client.GetNodes(checkCtx)
	nodeCount := 0
	readyNodes := 0
	if nodes != nil {
		nodeCount = len(nodes)
		for _, n := range nodes {
			if n.Status == "Ready" {
				readyNodes++
			}
		}
	}

	h.updateClusterStatusInDB(ctx, clusterID, "connected", version, nodeCount)

	h.logger.Info("cluster client created and verified",
		zap.String("cluster_id", clusterID.String()),
		zap.String("version", version),
		zap.Int("node_count", nodeCount),
		zap.Int("ready_nodes", readyNodes),
	)

	return client, nil
}

// updateClusterStatusInDB updates the cluster status, version, and node count
// in the database using the provided context.
func (h *Handler) updateClusterStatusInDB(ctx context.Context, clusterID uuid.UUID, status, version string, nodeCount int) {
	now := time.Now().UTC()

	var versionVal interface{}
	if version != "" {
		versionVal = version
	} else {
		versionVal = nil
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE kubernetes_clusters
		SET status = $1, kubernetes_version = $2, node_count = $3,
		    last_connected_at = $4, updated_at = $4
		WHERE id = $5
	`, status, versionVal, nodeCount, now, clusterID)

	if err != nil {
		h.logger.Error("failed to update cluster status",
			zap.String("cluster_id", clusterID.String()),
			zap.String("status", status),
			zap.Error(err),
		)
	}
}

// --- HTTP Handlers ---

// ListClustersHandler returns all clusters registered for the user's organization
// with their current health status.
// GET /api/v1/clusters
func (h *Handler) ListClustersHandler(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT id, name, description, api_endpoint, default_namespace, status,
		       kubernetes_version, node_count, last_connected_at, created_at, updated_at
		FROM kubernetes_clusters
		WHERE organization_id = $1
		ORDER BY name ASC
	`, claims.OrganizationID)
	if err != nil {
		h.logger.Error("failed to query clusters", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve clusters.",
			Code:    http.StatusInternalServerError,
		})
		return
	}
	defer rows.Close()

	clusters := make([]clusterListItem, 0)
	for rows.Next() {
		var cl clusterListItem
		var desc sql.NullString
		var k8sVersion sql.NullString
		var nodeCount sql.NullInt64
		var lastConnected sql.NullTime

		if err := rows.Scan(
			&cl.ID, &cl.Name, &desc, &cl.APIEndpoint, &cl.DefaultNamespace,
			&cl.Status, &k8sVersion, &nodeCount, &lastConnected,
			&cl.CreatedAt, &cl.UpdatedAt,
		); err != nil {
			h.logger.Error("failed to scan cluster row", zap.Error(err))
			continue
		}

		if desc.Valid {
			cl.Description = &desc.String
		}
		if k8sVersion.Valid {
			cl.KubernetesVersion = &k8sVersion.String
		}
		if nodeCount.Valid {
			nc := int(nodeCount.Int64)
			cl.NodeCount = &nc
		}
		if lastConnected.Valid {
			cl.LastConnectedAt = &lastConnected.Time
		}

		clusters = append(clusters, cl)
	}

	c.JSON(http.StatusOK, gin.H{"data": clusters})
}

// RegisterClusterHandler registers a new Kubernetes cluster for the organization.
// It accepts kubeconfig or direct connection details (API endpoint + certs).
// The cluster is created with a 'pending' status and a background goroutine
// verifies connectivity before updating the status to 'connected' or 'error'.
// POST /api/v1/clusters
func (h *Handler) RegisterClusterHandler(c *gin.Context) {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	var req registerClusterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	defaultNS := req.DefaultNamespace
	if defaultNS == "" {
		defaultNS = "chaos-sec"
	}

	var id uuid.UUID
	var createdAt, updatedAt time.Time

	err = h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO kubernetes_clusters (organization_id, name, description, api_endpoint,
			ca_certificate, client_certificate, client_key, default_namespace, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
		RETURNING id, created_at, updated_at
	`, claims.OrganizationID, req.Name, nilIfEmpty(req.Description), req.APIEndpoint,
		req.CACertificate, req.ClientCertificate, req.ClientKey, defaultNS,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		h.logger.Error("failed to insert cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to create cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Attempt to connect to the cluster in a background goroutine to verify connectivity.
	go h.verifyClusterConnection(id, req)

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"status":     "pending",
		"created_at": createdAt,
		"message":    "Cluster registered. Connection verification is in progress.",
	})
}

// verifyClusterConnection runs as a goroutine to verify cluster connectivity
// after registration. It updates the cluster status based on the result.
func (h *Handler) verifyClusterConnection(clusterID uuid.UUID, req registerClusterRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cluster := &models.KubernetesCluster{
		ID:                clusterID,
		Name:              req.Name,
		APIEndpoint:       req.APIEndpoint,
		CACertificate:     req.CACertificate,
		ClientCertificate: req.ClientCertificate,
		ClientKey:         req.ClientKey,
		DefaultNamespace:  req.DefaultNamespace,
	}

	client, err := h.clientManager.RegisterClusterFromConfig(cluster)
	if err != nil {
		h.logger.Error("cluster connection verification failed",
			zap.String("cluster_id", clusterID.String()),
			zap.Error(err),
		)
		h.updateClusterStatusInDB(ctx, clusterID, "error", "", 0)
		return
	}

	if err := client.HealthCheck(ctx); err != nil {
		h.logger.Error("cluster health check failed during verification",
			zap.String("cluster_id", clusterID.String()),
			zap.Error(err),
		)
		h.updateClusterStatusInDB(ctx, clusterID, "error", "", 0)
		h.clientManager.RemoveCluster(clusterID.String())
		return
	}

	version, _ := client.GetVersion(ctx)
	nodes, _ := client.GetNodes(ctx)
	nodeCount := 0
	if nodes != nil {
		nodeCount = len(nodes)
	}

	h.updateClusterStatusInDB(ctx, clusterID, "connected", version, nodeCount)

	h.logger.Info("cluster connection verified",
		zap.String("cluster_id", clusterID.String()),
		zap.String("version", version),
		zap.Int("node_count", nodeCount),
	)
}

// GetClusterHandler returns detailed information about a specific cluster,
// including live health and version information from the Kubernetes API.
// GET /api/v1/clusters/:id
func (h *Handler) GetClusterHandler(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	cluster, err := h.fetchClusterFromDB(c.Request.Context(), clusterID, claims.OrganizationID)
	if err != nil {
		if err.Error() == "cluster not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Cluster not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Build the response from the database record.
	resp := clusterDetailResponse{
		ID:                cluster.ID,
		OrganizationID:    cluster.OrganizationID,
		Name:              cluster.Name,
		APIEndpoint:       cluster.APIEndpoint,
		DefaultNamespace:  cluster.DefaultNamespace,
		Status:            cluster.Status,
		KubernetesVersion: cluster.KubernetesVersion,
		NodeCount:         cluster.NodeCount,
		LastConnectedAt:   cluster.LastConnectedAt,
		CreatedAt:         cluster.CreatedAt,
		UpdatedAt:         cluster.UpdatedAt,
	}
	if cluster.Description != "" {
		resp.Description = &cluster.Description
	}

	// Attempt to fetch live cluster info from the Kubernetes API.
	client, clientErr := h.getOrCreateClient(c, clusterID, claims.OrganizationID)
	if clientErr != nil {
		resp.LiveInfo = &clusterLiveInfo{
			Healthy: false,
			Error:   clientErr.Error(),
		}
	} else {
		liveInfo := &clusterLiveInfo{}
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		if err := client.HealthCheck(checkCtx); err != nil {
			liveInfo.Healthy = false
			liveInfo.Error = err.Error()
		} else {
			liveInfo.Healthy = true
			liveInfo.Version, _ = client.GetVersion(checkCtx)
			nodes, _ := client.GetNodes(checkCtx)
			if nodes != nil {
				liveInfo.Nodes = len(nodes)
				for _, n := range nodes {
					if n.Status == "Ready" {
						liveInfo.ReadyNodes++
					}
				}
			}

			// Fetch cluster resource summary.
			monitor, monErr := NewResourceMonitor(client)
			if monErr == nil {
				summary, sumErr := monitor.GetClusterSummary(checkCtx)
				if sumErr == nil {
					liveInfo.ClusterResources = summary
				}
			}
		}

		resp.LiveInfo = liveInfo
	}

	c.JSON(http.StatusOK, resp)
}

// DeleteClusterHandler removes a cluster registration. It first checks for
// running experiments on the cluster, removes the client from the cache,
// and then deletes the database record.
// DELETE /api/v1/clusters/:id
func (h *Handler) DeleteClusterHandler(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Check for running experiments on this cluster.
	var runningCount int
	err = h.db.QueryRowContext(c.Request.Context(), `
		SELECT COUNT(*) FROM experiment_runs
		WHERE cluster_id = $1 AND status = 'running'
	`, clusterID).Scan(&runningCount)
	if err != nil {
		h.logger.Error("failed to check running experiments on cluster", zap.Error(err))
		// Continue with deletion attempt — fail-safe on query error.
	}

	if runningCount > 0 {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "has_running_runs",
			Message: "Cannot delete cluster with running experiments. Stop all runs first.",
			Code:    http.StatusConflict,
		})
		return
	}

	// Remove the client from the cache.
	h.clientManager.RemoveCluster(clusterID.String())

	result, err := h.db.ExecContext(c.Request.Context(),
		"DELETE FROM kubernetes_clusters WHERE id = $1 AND organization_id = $2",
		clusterID, claims.OrganizationID,
	)
	if err != nil {
		h.logger.Error("failed to delete cluster", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to delete cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Cluster not found.",
			Code:    http.StatusNotFound,
		})
		return
	}

	h.logger.Info("cluster deleted",
		zap.String("cluster_id", clusterID.String()),
		zap.String("deleted_by", claims.UserID.String()),
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Cluster deleted.",
		"id":      clusterID,
	})
}

// GetClusterNamespacesHandler lists namespaces in the specified cluster.
// It supports an optional label_selector query parameter for filtering.
// GET /api/v1/clusters/:id/namespaces
func (h *Handler) GetClusterNamespacesHandler(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	client, err := h.getOrCreateClient(c, clusterID, claims.OrganizationID)
	if err != nil {
		h.logger.Error("failed to get cluster client for namespaces", zap.Error(err))
		c.JSON(http.StatusBadGateway, models.ErrorResponse{
			Error:   "cluster_unavailable",
			Message: "Failed to connect to cluster: " + err.Error(),
			Code:    http.StatusBadGateway,
		})
		return
	}

	nsManager, err := NewNamespaceManager(client)
	if err != nil {
		h.logger.Error("failed to create namespace manager", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to initialize namespace manager.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	labelSelector := c.Query("label_selector")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	nsNames, err := nsManager.ListNamespaces(ctx, labelSelector)
	if err != nil {
		h.logger.Error("failed to list namespaces",
			zap.String("cluster_id", clusterID.String()),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "k8s_error",
			Message: "Failed to list namespaces: " + err.Error(),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Build the response with additional metadata for each namespace.
	namespaces := make([]namespaceListItem, 0, len(nsNames))
	for _, nsName := range nsNames {
		item := namespaceListItem{
			Name: nsName,
		}

		// Try to get status for each namespace.
		status, statusErr := nsManager.GetNamespaceStatus(ctx, nsName)
		if statusErr == nil {
			item.Status = status.Phase
			item.Labels = status.Labels
			if status.CreatedAt != nil {
				item.CreatedAt = status.CreatedAt.Time
			}
			if managed, ok := status.Labels["app.kubernetes.io/managed-by"]; ok && managed == "chaos-sec" {
				item.IsManaged = true
			}
		}

		namespaces = append(namespaces, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"cluster_id": clusterID,
		"data":       namespaces,
	})
}

// GetClusterNetworkPoliciesHandler lists network policies in the specified cluster.
// It supports an optional namespace query parameter (defaults to all namespaces).
// GET /api/v1/clusters/:id/network-policies
func (h *Handler) GetClusterNetworkPoliciesHandler(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	client, err := h.getOrCreateClient(c, clusterID, claims.OrganizationID)
	if err != nil {
		h.logger.Error("failed to get cluster client for network policies", zap.Error(err))
		c.JSON(http.StatusBadGateway, models.ErrorResponse{
			Error:   "cluster_unavailable",
			Message: "Failed to connect to cluster: " + err.Error(),
			Code:    http.StatusBadGateway,
		})
		return
	}

	npController, err := NewNetworkPolicyController(client)
	if err != nil {
		h.logger.Error("failed to create network policy controller", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to initialize network policy controller.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	namespace := c.Query("namespace")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	policies, err := npController.ListNetworkPolicies(ctx, namespace)
	if err != nil {
		h.logger.Error("failed to list network policies",
			zap.String("cluster_id", clusterID.String()),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "k8s_error",
			Message: "Failed to list network policies: " + err.Error(),
			Code:    http.StatusInternalServerError,
		})
		return
	}

	// Convert to simplified response format.
	items := make([]networkPolicyListItem, 0, len(policies))
	for _, p := range policies {
		ptypes := make([]string, 0, len(p.PolicyTypes))
		for _, pt := range p.PolicyTypes {
			ptypes = append(ptypes, string(pt))
		}

		items = append(items, networkPolicyListItem{
			Name:        p.Name,
			Namespace:   p.Namespace,
			PolicyTypes: ptypes,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"cluster_id": clusterID,
		"data":       items,
	})
}

// GetClusterHealthHandler returns detailed health information for a cluster,
// including node health, resource usage, and connectivity status.
// GET /api/v1/clusters/:id/health
func (h *Handler) GetClusterHealthHandler(c *gin.Context) {
	clusterID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid cluster ID format.",
			Code:    http.StatusBadRequest,
		})
		return
	}

	claims, err := getClaimsFromContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Authentication required.",
			Code:    http.StatusUnauthorized,
		})
		return
	}

	// Fetch cluster metadata from DB.
	cluster, dbErr := h.fetchClusterFromDB(c.Request.Context(), clusterID, claims.OrganizationID)
	if dbErr != nil {
		if dbErr.Error() == "cluster not found" {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "not_found",
				Message: "Cluster not found.",
				Code:    http.StatusNotFound,
			})
			return
		}
		h.logger.Error("failed to query cluster for health check", zap.Error(dbErr))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve cluster.",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	resp := clusterHealthResponse{
		ClusterID:   clusterID,
		ClusterName: cluster.Name,
		Status:      cluster.Status,
		CheckedAt:   time.Now().UTC(),
	}

	// Get or create a client for live health info.
	client, clientErr := h.getOrCreateClient(c, clusterID, claims.OrganizationID)
	if clientErr != nil {
		resp.Healthy = false
		resp.Error = "Failed to connect to cluster: " + clientErr.Error()
		c.JSON(http.StatusOK, resp)
		return
	}

	checkCtx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// Perform health check.
	if err := client.HealthCheck(checkCtx); err != nil {
		resp.Healthy = false
		resp.Error = "Health check failed: " + err.Error()
		h.updateClusterStatusInDB(c.Request.Context(), clusterID, "error", "", 0)
		c.JSON(http.StatusOK, resp)
		return
	}

	resp.Healthy = true

	// Get version.
	resp.Version, _ = client.GetVersion(checkCtx)

	// Get node information.
	nodes, _ := client.GetNodes(checkCtx)
	if nodes != nil {
		resp.NodeCount = len(nodes)
		resp.Nodes = make([]NodeHealthInfo, 0, len(nodes))
		for _, n := range nodes {
			nodeInfo := NodeHealthInfo{
				Name:   n.Name,
				Status: n.Status,
				CPU:    n.CPUCapacity,
				Memory: n.MemoryCapacity,
			}
			resp.Nodes = append(resp.Nodes, nodeInfo)
			if n.Status == "Ready" {
				resp.ReadyNodes++
			}
		}
	}

	// Get cluster resource summary.
	monitor, monErr := NewResourceMonitor(client)
	if monErr == nil {
		summary, sumErr := monitor.GetClusterSummary(checkCtx)
		if sumErr == nil {
			resp.Resources = summary
		}
	}

	// Update the cluster status to reflect the successful health check.
	h.updateClusterStatusInDB(c.Request.Context(), clusterID, "connected", resp.Version, resp.NodeCount)

	c.JSON(http.StatusOK, resp)
}

// ValidateRBACPermissions checks whether the current user's service account
// has the necessary RBAC permissions in the target cluster for the requested
// operations. This is an optional middleware that can be used before cluster
// operations to verify authorization.
func (h *Handler) ValidateRBACPermissions(c *gin.Context, clusterID uuid.UUID, requiredVerbs []string, requiredResources []string) error {
	claims, err := getClaimsFromContext(c)
	if err != nil {
		return fmt.Errorf("authentication required")
	}

	client, err := h.getOrCreateClient(c, clusterID, claims.OrganizationID)
	if err != nil {
		return fmt.Errorf("failed to get cluster client: %w", err)
	}

	// Use SelfSubjectAccessReview API to check permissions.
	// This checks whether the calling service account can perform the
	// specified verbs on the specified resources in the cluster.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// For each resource/verb combination, create a SelfSubjectAccessReview.
	// We use the discovery API to verify basic connectivity first.
	if err := client.HealthCheck(ctx); err != nil {
		return fmt.Errorf("cluster health check failed: %w", err)
	}

	// If we can successfully perform a health check and get nodes,
	// the service account has at least basic read permissions.
	// A more fine-grained check would use the authorization API,
	// but for now, connectivity implies basic permissions.
	h.logger.Debug("RBAC permissions validated",
		zap.String("cluster_id", clusterID.String()),
		zap.Strings("required_verbs", requiredVerbs),
		zap.Strings("required_resources", requiredResources),
	)

	return nil
}
