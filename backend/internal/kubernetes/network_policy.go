package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NetworkPolicyInfo holds summarized information about a Kubernetes NetworkPolicy.
type NetworkPolicyInfo struct {
	Name         string                    `json:"name"`
	Namespace    string                    `json:"namespace"`
	PodSelector  metav1.LabelSelector      `json:"pod_selector"`
	PolicyTypes  []networkingv1.PolicyType `json:"policy_types"`
	IngressRules []IngressRuleInfo         `json:"ingress_rules,omitempty"`
	EgressRules  []EgressRuleInfo          `json:"egress_rules,omitempty"`
	CreatedAt    metav1.Time               `json:"created_at,omitempty"`
}

// IngressRuleInfo holds a summary of a single ingress rule within a NetworkPolicy.
type IngressRuleInfo struct {
	From     []PeerInfo `json:"from,omitempty"`
	Ports    []PortInfo `json:"ports,omitempty"`
	AllowAll bool       `json:"allow_all,omitempty"`
}

// EgressRuleInfo holds a summary of a single egress rule within a NetworkPolicy.
type EgressRuleInfo struct {
	To       []PeerInfo `json:"to,omitempty"`
	Ports    []PortInfo `json:"ports,omitempty"`
	AllowAll bool       `json:"allow_all,omitempty"`
}

// PeerInfo holds information about a network peer (source or destination)
// in a network policy rule.
type PeerInfo struct {
	IPBlock     *IPBlockInfo          `json:"ip_block,omitempty"`
	Namespace   string                `json:"namespace,omitempty"`
	PodSelector *metav1.LabelSelector `json:"pod_selector,omitempty"`
}

// IPBlockInfo holds information about an IP block in a network policy rule.
type IPBlockInfo struct {
	CIDR   string   `json:"cidr"`
	Except []string `json:"except,omitempty"`
}

// PortInfo holds information about a port specification in a network policy rule.
type PortInfo struct {
	Port     int32  `json:"port,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// Destination describes a network destination for egress policy validation.
type Destination struct {
	IP        string `json:"ip"`
	Port      int32  `json:"port"`
	Protocol  string `json:"protocol"`
	Namespace string `json:"namespace,omitempty"`
}

// Source describes a network source for ingress policy validation.
type Source struct {
	IP          string                `json:"ip"`
	Namespace   string                `json:"namespace,omitempty"`
	PodSelector *metav1.LabelSelector `json:"pod_selector,omitempty"`
}

// PolicyValidationResult holds the result of a network policy validation check.
type PolicyValidationResult struct {
	Allowed     bool   `json:"allowed"`
	PolicyName  string `json:"policy_name,omitempty"`
	RuleDetails string `json:"rule_details,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// TestNetworkPolicyConfig holds the configuration for creating a temporary
// test network policy during a security experiment.
type TestNetworkPolicyConfig struct {
	Name           string                    `json:"name"`
	Namespace      string                    `json:"namespace"`
	PodSelector    metav1.LabelSelector      `json:"pod_selector"`
	PolicyTypes    []networkingv1.PolicyType `json:"policy_types"`
	IngressRules   []TestIngressRule         `json:"ingress_rules,omitempty"`
	EgressRules    []TestEgressRule          `json:"egress_rules,omitempty"`
	ExperimentID   string                    `json:"experiment_id"`
	DenyAllIngress bool                      `json:"deny_all_ingress,omitempty"`
	DenyAllEgress  bool                      `json:"deny_all_egress,omitempty"`
}

// TestIngressRule defines an ingress rule for a test network policy.
type TestIngressRule struct {
	From  []TestPeer `json:"from,omitempty"`
	Ports []TestPort `json:"ports,omitempty"`
}

// TestEgressRule defines an egress rule for a test network policy.
type TestEgressRule struct {
	To    []TestPeer `json:"to,omitempty"`
	Ports []TestPort `json:"ports,omitempty"`
}

// TestPeer defines a network peer for a test network policy rule.
type TestPeer struct {
	IPBlock     string                `json:"ip_block,omitempty"`
	Namespace   string                `json:"namespace,omitempty"`
	PodSelector *metav1.LabelSelector `json:"pod_selector,omitempty"`
}

// TestPort defines a port specification for a test network policy rule.
type TestPort struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}

// NetworkPolicyController manages Kubernetes NetworkPolicies for security
// experiments. It provides methods for listing, creating test policies,
// and validating whether specific egress or ingress traffic is blocked
// by the existing network policies in a namespace.
type NetworkPolicyController struct {
	client     kubernetes.Interface
	restConfig *rest.Config
	clusterID  string
	logger     *zap.Logger
}

// NewNetworkPolicyController creates a new NetworkPolicyController for the
// given cluster client.
func NewNetworkPolicyController(client *ClusterClient) (*NetworkPolicyController, error) {
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

	return &NetworkPolicyController{
		client:     cs,
		restConfig: rc,
		clusterID:  client.ClusterID(),
		logger:     client.logger.Named("network_policy_controller"),
	}, nil
}

// NewNetworkPolicyControllerFromClientset creates a NetworkPolicyController
// directly from a clientset and rest config. Useful for testing.
func NewNetworkPolicyControllerFromClientset(clientset kubernetes.Interface, restConfig *rest.Config, clusterID string, logger *zap.Logger) *NetworkPolicyController {
	return &NetworkPolicyController{
		client:     clientset,
		restConfig: restConfig,
		clusterID:  clusterID,
		logger:     logger.Named("network_policy_controller"),
	}
}

// ListNetworkPolicies returns all NetworkPolicies in the specified namespace.
// If no namespace is specified (empty string), it lists policies across all
// namespaces.
func (n *NetworkPolicyController) ListNetworkPolicies(ctx context.Context, namespace string) ([]NetworkPolicyInfo, error) {
	n.logger.Debug("listing network policies",
		zap.String("namespace", namespace),
	)

	var netPols *networkingv1.NetworkPolicyList
	var err error

	if namespace == "" {
		netPols, err = n.client.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	} else {
		netPols, err = n.client.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list network policies in namespace %q: %w", namespace, err)
	}

	policies := make([]NetworkPolicyInfo, 0, len(netPols.Items))
	for _, np := range netPols.Items {
		info := n.convertNetworkPolicyToInfo(&np)
		policies = append(policies, info)
	}

	return policies, nil
}

// GetNetworkPolicy returns detailed information about a specific NetworkPolicy.
func (n *NetworkPolicyController) GetNetworkPolicy(ctx context.Context, name, namespace string) (*NetworkPolicyInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("network policy name must not be empty")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	n.logger.Debug("getting network policy",
		zap.String("name", name),
		zap.String("namespace", namespace),
	)

	np, err := n.client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("network policy %s/%s not found", namespace, name)
		}
		return nil, fmt.Errorf("failed to get network policy %s/%s: %w", namespace, name, err)
	}

	info := n.convertNetworkPolicyToInfo(np)
	return &info, nil
}

// CreateTestNetworkPolicy creates a temporary NetworkPolicy as part of a security
// experiment. The policy is labeled with the experiment ID for tracking and
// later cleanup. It supports creating deny-all policies or policies with
// specific ingress/egress rules.
func (n *NetworkPolicyController) CreateTestNetworkPolicy(ctx context.Context, config TestNetworkPolicyConfig) error {
	if config.Name == "" {
		return fmt.Errorf("network policy name must not be empty")
	}
	if config.Namespace == "" {
		return fmt.Errorf("namespace must not be empty")
	}
	if config.ExperimentID == "" {
		return fmt.Errorf("experiment ID must not be empty")
	}

	n.logger.Info("creating test network policy",
		zap.String("name", config.Name),
		zap.String("namespace", config.Namespace),
		zap.String("experiment_id", config.ExperimentID),
	)

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "chaos-sec",
				"chaos-sec/experiment-id":      config.ExperimentID,
				"chaos-sec/policy-type":        "test",
			},
			Annotations: map[string]string{
				"chaos-sec/created-by": "network-policy-controller",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: config.PodSelector,
			PolicyTypes: config.PolicyTypes,
		},
	}

	// Handle deny-all shortcuts.
	if config.DenyAllIngress {
		hasIngress := false
		for _, pt := range np.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeIngress {
				hasIngress = true
				break
			}
		}
		if !hasIngress {
			np.Spec.PolicyTypes = append(np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		}
		// No ingress rules means deny all ingress.
	}

	if config.DenyAllEgress {
		hasEgress := false
		for _, pt := range np.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeEgress {
				hasEgress = true
				break
			}
		}
		if !hasEgress {
			np.Spec.PolicyTypes = append(np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
		}
		// No egress rules means deny all egress.
	}

	// Build ingress rules from config.
	if len(config.IngressRules) > 0 {
		for _, rule := range config.IngressRules {
			netPolRule := n.buildTestIngressRule(rule)
			np.Spec.Ingress = append(np.Spec.Ingress, netPolRule)
		}
	}

	// Build egress rules from config.
	if len(config.EgressRules) > 0 {
		for _, rule := range config.EgressRules {
			netPolRule := n.buildTestEgressRule(rule)
			np.Spec.Egress = append(np.Spec.Egress, netPolRule)
		}
	}

	// Ensure at least one policy type is set.
	if len(np.Spec.PolicyTypes) == 0 {
		np.Spec.PolicyTypes = []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
			networkingv1.PolicyTypeEgress,
		}
	}

	_, err := n.client.NetworkingV1().NetworkPolicies(config.Namespace).Create(ctx, np, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Warn("test network policy already exists",
				zap.String("name", config.Name),
				zap.String("namespace", config.Namespace),
			)
			return fmt.Errorf("test network policy %s/%s already exists", config.Namespace, config.Name)
		}
		n.logger.Error("failed to create test network policy",
			zap.String("name", config.Name),
			zap.String("namespace", config.Namespace),
			zap.Error(err),
		)
		return fmt.Errorf("failed to create test network policy %s/%s: %w", config.Namespace, config.Name, err)
	}

	n.logger.Info("test network policy created successfully",
		zap.String("name", config.Name),
		zap.String("namespace", config.Namespace),
	)

	return nil
}

// DeleteTestNetworkPolicy removes a temporary test NetworkPolicy created during
// a security experiment. It only deletes policies that are labeled as managed
// by chaos-sec to prevent accidental deletion of production policies.
func (n *NetworkPolicyController) DeleteTestNetworkPolicy(ctx context.Context, name, namespace string) error {
	if name == "" {
		return fmt.Errorf("network policy name must not be empty")
	}
	if namespace == "" {
		return fmt.Errorf("namespace must not be empty")
	}

	n.logger.Info("deleting test network policy",
		zap.String("name", name),
		zap.String("namespace", namespace),
	)

	// Verify the policy is managed by chaos-sec before deleting.
	np, err := n.client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			n.logger.Debug("test network policy not found, already deleted",
				zap.String("name", name),
				zap.String("namespace", namespace),
			)
			return nil
		}
		return fmt.Errorf("failed to get network policy %s/%s: %w", namespace, name, err)
	}

	// Safety check: only delete policies managed by chaos-sec.
	if managedBy, ok := np.Labels["app.kubernetes.io/managed-by"]; !ok || managedBy != "chaos-sec" {
		return fmt.Errorf("refusing to delete network policy %s/%s: not managed by chaos-sec (managed-by=%q)", namespace, name, managedBy)
	}

	err = n.client.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		n.logger.Error("failed to delete test network policy",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete test network policy %s/%s: %w", namespace, name, err)
	}

	n.logger.Info("test network policy deleted successfully",
		zap.String("name", name),
		zap.String("namespace", namespace),
	)

	return nil
}

// DeleteTestNetworkPoliciesByExperiment removes all test network policies
// created for a specific experiment across all namespaces managed by chaos-sec.
func (n *NetworkPolicyController) DeleteTestNetworkPoliciesByExperiment(ctx context.Context, namespace, experimentID string) error {
	labelSelector := fmt.Sprintf("app.kubernetes.io/managed-by=chaos-sec,chaos-sec/experiment-id=%s", experimentID)

	n.logger.Info("deleting test network policies for experiment",
		zap.String("namespace", namespace),
		zap.String("experiment_id", experimentID),
	)

	err := n.client.NetworkingV1().NetworkPolicies(namespace).DeleteCollection(
		ctx,
		metav1.DeleteOptions{},
		metav1.ListOptions{
			LabelSelector: labelSelector,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to delete test network policies for experiment %s in namespace %s: %w", experimentID, namespace, err)
	}

	n.logger.Info("test network policies deleted for experiment",
		zap.String("namespace", namespace),
		zap.String("experiment_id", experimentID),
	)

	return nil
}

// ValidateEgressPolicy checks whether egress traffic from pods in the specified
// namespace to the given destination is allowed or blocked by existing network
// policies. It evaluates all network policies in the namespace that select
// pods and determines the effective egress policy.
//
// In Kubernetes, if no network policy selects a pod, all egress is allowed.
// If any network policy selects a pod, only the traffic explicitly allowed
// by those policies is permitted.
func (n *NetworkPolicyController) ValidateEgressPolicy(ctx context.Context, namespace string, destination Destination) (*PolicyValidationResult, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	n.logger.Debug("validating egress policy",
		zap.String("namespace", namespace),
		zap.String("destination_ip", destination.IP),
		zap.Int32("destination_port", destination.Port),
		zap.String("destination_protocol", destination.Protocol),
	)

	// List all network policies in the namespace.
	policies, err := n.ListNetworkPolicies(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies for egress validation: %w", err)
	}

	// Filter to policies that have egress rules.
	var egressPolicies []NetworkPolicyInfo
	for _, p := range policies {
		for _, pt := range p.PolicyTypes {
			if pt == networkingv1.PolicyTypeEgress {
				egressPolicies = append(egressPolicies, p)
				break
			}
		}
	}

	// If no egress policies exist, all egress is allowed by default.
	if len(egressPolicies) == 0 {
		return &PolicyValidationResult{
			Allowed:     true,
			RuleDetails: "No egress network policies found in namespace, all egress is allowed by default",
			Reason:      "default_allow",
		}, nil
	}

	// Evaluate each egress policy.
	for _, policy := range egressPolicies {
		result := n.evaluateEgressRules(policy, destination)
		if result != nil {
			result.PolicyName = policy.Name
			return result, nil
		}
	}

	// If policies exist but none explicitly allow traffic to the destination,
	// the traffic is blocked by default (default deny when policies select pods).
	return &PolicyValidationResult{
		Allowed:     false,
		RuleDetails: fmt.Sprintf("Egress to %s:%d/%s is not explicitly allowed by any network policy", destination.IP, destination.Port, destination.Protocol),
		Reason:      "default_deny",
	}, nil
}

// ValidateIngressPolicy checks whether ingress traffic from the given source
// to pods in the specified namespace is allowed or blocked by existing network
// policies. It evaluates all network policies in the namespace that select
// pods and determines the effective ingress policy.
//
// In Kubernetes, if no network policy selects a pod, all ingress is allowed.
// If any network policy selects a pod, only the traffic explicitly allowed
// by those policies is permitted.
func (n *NetworkPolicyController) ValidateIngressPolicy(ctx context.Context, namespace string, source Source) (*PolicyValidationResult, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	n.logger.Debug("validating ingress policy",
		zap.String("namespace", namespace),
		zap.String("source_ip", source.IP),
		zap.String("source_namespace", source.Namespace),
	)

	// List all network policies in the namespace.
	policies, err := n.ListNetworkPolicies(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies for ingress validation: %w", err)
	}

	// Filter to policies that have ingress rules.
	var ingressPolicies []NetworkPolicyInfo
	for _, p := range policies {
		for _, pt := range p.PolicyTypes {
			if pt == networkingv1.PolicyTypeIngress {
				ingressPolicies = append(ingressPolicies, p)
				break
			}
		}
	}

	// If no ingress policies exist, all ingress is allowed by default.
	if len(ingressPolicies) == 0 {
		return &PolicyValidationResult{
			Allowed:     true,
			RuleDetails: "No ingress network policies found in namespace, all ingress is allowed by default",
			Reason:      "default_allow",
		}, nil
	}

	// Evaluate each ingress policy.
	for _, policy := range ingressPolicies {
		result := n.evaluateIngressRules(policy, source)
		if result != nil {
			result.PolicyName = policy.Name
			return result, nil
		}
	}

	// If policies exist but none explicitly allow traffic from the source,
	// the traffic is blocked by default.
	return &PolicyValidationResult{
		Allowed:     false,
		RuleDetails: fmt.Sprintf("Ingress from %s (namespace=%s) is not explicitly allowed by any network policy", source.IP, source.Namespace),
		Reason:      "default_deny",
	}, nil
}

// --- Internal helpers ---

// convertNetworkPolicyToInfo converts a Kubernetes NetworkPolicy object into
// a simplified NetworkPolicyInfo for API responses.
func (n *NetworkPolicyController) convertNetworkPolicyToInfo(np *networkingv1.NetworkPolicy) NetworkPolicyInfo {
	info := NetworkPolicyInfo{
		Name:        np.Name,
		Namespace:   np.Namespace,
		PodSelector: np.Spec.PodSelector,
		PolicyTypes: np.Spec.PolicyTypes,
		CreatedAt:   np.CreationTimestamp,
	}

	// Convert ingress rules.
	for _, rule := range np.Spec.Ingress {
		ruleInfo := IngressRuleInfo{}

		// Check if this is an allow-all rule (no "from" specified).
		if len(rule.From) == 0 {
			ruleInfo.AllowAll = true
		}

		for _, peer := range rule.From {
			peerInfo := n.convertNetworkPolicyPeer(peer)
			ruleInfo.From = append(ruleInfo.From, peerInfo)
		}

		for _, port := range rule.Ports {
			ruleInfo.Ports = append(ruleInfo.Ports, n.convertNetworkPolicyPort(port))
		}

		info.IngressRules = append(info.IngressRules, ruleInfo)
	}

	// Convert egress rules.
	for _, rule := range np.Spec.Egress {
		ruleInfo := EgressRuleInfo{}

		// Check if this is an allow-all rule (no "to" specified).
		if len(rule.To) == 0 {
			ruleInfo.AllowAll = true
		}

		for _, peer := range rule.To {
			peerInfo := n.convertNetworkPolicyPeer(peer)
			ruleInfo.To = append(ruleInfo.To, peerInfo)
		}

		for _, port := range rule.Ports {
			ruleInfo.Ports = append(ruleInfo.Ports, n.convertNetworkPolicyPort(port))
		}

		info.EgressRules = append(info.EgressRules, ruleInfo)
	}

	return info
}

// convertNetworkPolicyPeer converts a NetworkPolicyPeer into a simplified PeerInfo.
func (n *NetworkPolicyController) convertNetworkPolicyPeer(peer networkingv1.NetworkPolicyPeer) PeerInfo {
	peerInfo := PeerInfo{}

	if peer.IPBlock != nil {
		peerInfo.IPBlock = &IPBlockInfo{
			CIDR:   peer.IPBlock.CIDR,
			Except: peer.IPBlock.Except,
		}
	}

	if peer.NamespaceSelector != nil {
		// We store the namespace selector as a string representation for readability.
		peerInfo.Namespace = labelSelectorToString(peer.NamespaceSelector)
	}

	if peer.PodSelector != nil {
		podSel := *peer.PodSelector
		peerInfo.PodSelector = &podSel
	}

	return peerInfo
}

// convertNetworkPolicyPort converts a NetworkPolicyPort into a simplified PortInfo.
func (n *NetworkPolicyController) convertNetworkPolicyPort(port networkingv1.NetworkPolicyPort) PortInfo {
	portInfo := PortInfo{}

	if port.Port != nil {
		portInfo.Port = port.Port.IntVal
	}

	if port.Protocol != nil {
		portInfo.Protocol = string(*port.Protocol)
	}

	return portInfo
}

// buildTestIngressRule converts a TestIngressRule into a Kubernetes NetworkPolicyIngressRule.
func (n *NetworkPolicyController) buildTestIngressRule(rule TestIngressRule) networkingv1.NetworkPolicyIngressRule {
	netPolRule := networkingv1.NetworkPolicyIngressRule{}

	for _, from := range rule.From {
		peer := n.buildTestPeer(from)
		netPolRule.From = append(netPolRule.From, peer)
	}

	for _, port := range rule.Ports {
		netPolPort := n.buildTestPort(port)
		netPolRule.Ports = append(netPolRule.Ports, netPolPort)
	}

	return netPolRule
}

// buildTestEgressRule converts a TestEgressRule into a Kubernetes NetworkPolicyEgressRule.
func (n *NetworkPolicyController) buildTestEgressRule(rule TestEgressRule) networkingv1.NetworkPolicyEgressRule {
	netPolRule := networkingv1.NetworkPolicyEgressRule{}

	for _, to := range rule.To {
		peer := n.buildTestPeer(to)
		netPolRule.To = append(netPolRule.To, peer)
	}

	for _, port := range rule.Ports {
		netPolPort := n.buildTestPort(port)
		netPolRule.Ports = append(netPolRule.Ports, netPolPort)
	}

	return netPolRule
}

// buildTestPeer converts a TestPeer into a Kubernetes NetworkPolicyPeer.
func (n *NetworkPolicyController) buildTestPeer(peer TestPeer) networkingv1.NetworkPolicyPeer {
	netPeer := networkingv1.NetworkPolicyPeer{}

	if peer.IPBlock != "" {
		netPeer.IPBlock = &networkingv1.IPBlock{
			CIDR: peer.IPBlock,
		}
	}

	if peer.Namespace != "" || peer.PodSelector != nil {
		if peer.Namespace != "" {
			netPeer.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": peer.Namespace,
				},
			}
		}

		if peer.PodSelector != nil {
			podSel := *peer.PodSelector
			netPeer.PodSelector = &podSel
		}
	}

	return netPeer
}

// buildTestPort converts a TestPort into a Kubernetes NetworkPolicyPort.
func (n *NetworkPolicyController) buildTestPort(port TestPort) networkingv1.NetworkPolicyPort {
	proto := corev1.ProtocolTCP // default to TCP
	if port.Protocol != "" {
		proto = corev1.Protocol(port.Protocol)
	}

	netPort := networkingv1.NetworkPolicyPort{
		Protocol: &proto,
	}

	if port.Port > 0 {
		p := portFromInt32(port.Port)
		netPort.Port = &p
	}

	return netPort
}

// evaluateEgressRules checks whether an egress rule in the given policy
// explicitly allows traffic to the specified destination.
// Returns nil if no rule matches (meaning the policy doesn't allow this traffic).
func (n *NetworkPolicyController) evaluateEgressRules(policy NetworkPolicyInfo, destination Destination) *PolicyValidationResult {
	for _, rule := range policy.EgressRules {
		// If the rule allows all egress, the traffic is permitted.
		if rule.AllowAll {
			return &PolicyValidationResult{
				Allowed:     true,
				RuleDetails: fmt.Sprintf("Policy %s has an allow-all egress rule", policy.Name),
				Reason:      "allow_all_egress",
			}
		}

		// Check if any destination peer matches.
		for _, peer := range rule.To {
			if n.peerMatchesDestination(peer, destination) {
				// Check if the port is allowed (no ports specified means all ports).
				if len(rule.Ports) == 0 {
					return &PolicyValidationResult{
						Allowed:     true,
						RuleDetails: fmt.Sprintf("Policy %s allows egress to %s on all ports", policy.Name, destination.IP),
						Reason:      "explicit_allow",
					}
				}

				// Check if the specific port/protocol is allowed.
				for _, port := range rule.Ports {
					if n.portMatches(port, destination.Port, destination.Protocol) {
						return &PolicyValidationResult{
							Allowed:     true,
							RuleDetails: fmt.Sprintf("Policy %s allows egress to %s:%d/%s", policy.Name, destination.IP, destination.Port, destination.Protocol),
							Reason:      "explicit_allow",
						}
					}
				}
			}
		}

		// If no "to" peers are specified but ports are, the rule allows egress
		// to all destinations on those ports.
		if len(rule.To) == 0 && len(rule.Ports) > 0 {
			for _, port := range rule.Ports {
				if n.portMatches(port, destination.Port, destination.Protocol) {
					return &PolicyValidationResult{
						Allowed:     true,
						RuleDetails: fmt.Sprintf("Policy %s allows egress to all destinations on port %d/%s", policy.Name, destination.Port, destination.Protocol),
						Reason:      "explicit_allow",
					}
				}
			}
		}
	}

	return nil // No matching rule found.
}

// evaluateIngressRules checks whether an ingress rule in the given policy
// explicitly allows traffic from the specified source.
// Returns nil if no rule matches (meaning the policy doesn't allow this traffic).
func (n *NetworkPolicyController) evaluateIngressRules(policy NetworkPolicyInfo, source Source) *PolicyValidationResult {
	for _, rule := range policy.IngressRules {
		// If the rule allows all ingress, the traffic is permitted.
		if rule.AllowAll {
			return &PolicyValidationResult{
				Allowed:     true,
				RuleDetails: fmt.Sprintf("Policy %s has an allow-all ingress rule", policy.Name),
				Reason:      "allow_all_ingress",
			}
		}

		// Check if any source peer matches.
		for _, peer := range rule.From {
			if n.peerMatchesSource(peer, source) {
				// No ports specified means all ports allowed.
				if len(rule.Ports) == 0 {
					return &PolicyValidationResult{
						Allowed:     true,
						RuleDetails: fmt.Sprintf("Policy %s allows ingress from %s on all ports", policy.Name, source.IP),
						Reason:      "explicit_allow",
					}
				}

				// Check specific port matches.
				for _, port := range rule.Ports {
					if source.IP != "" && n.portMatches(port, 0, "") {
						return &PolicyValidationResult{
							Allowed:     true,
							RuleDetails: fmt.Sprintf("Policy %s allows ingress from %s", policy.Name, source.IP),
							Reason:      "explicit_allow",
						}
					}
				}
			}
		}
	}

	return nil // No matching rule found.
}

// peerMatchesDestination checks whether a network policy peer matches the
// given destination specification.
func (n *NetworkPolicyController) peerMatchesDestination(peer PeerInfo, dest Destination) bool {
	// Match IPBlock.
	if peer.IPBlock != nil && dest.IP != "" {
		if cidrContainsIP(peer.IPBlock.CIDR, dest.IP) {
			// Check if the IP is in the except list.
			for _, except := range peer.IPBlock.Except {
				if cidrContainsIP(except, dest.IP) {
					return false
				}
			}
			return true
		}
	}

	// Match namespace and pod selector.
	if dest.Namespace != "" {
		if peer.Namespace != "" && strings.Contains(peer.Namespace, dest.Namespace) {
			return true
		}
	}

	return false
}

// peerMatchesSource checks whether a network policy peer matches the
// given source specification.
func (n *NetworkPolicyController) peerMatchesSource(peer PeerInfo, source Source) bool {
	// Match IPBlock.
	if peer.IPBlock != nil && source.IP != "" {
		if cidrContainsIP(peer.IPBlock.CIDR, source.IP) {
			for _, except := range peer.IPBlock.Except {
				if cidrContainsIP(except, source.IP) {
					return false
				}
			}
			return true
		}
	}

	// Match namespace selector.
	if source.Namespace != "" && peer.Namespace != "" {
		if strings.Contains(peer.Namespace, source.Namespace) {
			return true
		}
	}

	// Match pod selector.
	if source.PodSelector != nil && peer.PodSelector != nil {
		if labelSelectorsMatch(source.PodSelector, peer.PodSelector) {
			return true
		}
	}

	return false
}

// portMatches checks whether a port specification matches the given
// port number and protocol.
func (n *NetworkPolicyController) portMatches(port PortInfo, targetPort int32, targetProtocol string) bool {
	// If no port/protocol is specified in the rule, all ports are allowed.
	if port.Port == 0 && port.Protocol == "" {
		return true
	}

	// Check protocol match.
	if port.Protocol != "" && targetProtocol != "" {
		if !strings.EqualFold(port.Protocol, targetProtocol) {
			return false
		}
	}

	// Check port match (0 means all ports).
	if port.Port != 0 && targetPort != 0 {
		if port.Port != targetPort {
			return false
		}
	}

	return true
}

// --- Utility functions ---

// labelSelectorToString converts a LabelSelector to a human-readable string.
func labelSelectorToString(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}

	parts := make([]string, 0, len(selector.MatchLabels)+len(selector.MatchExpressions))

	for key, value := range selector.MatchLabels {
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}

	for _, expr := range selector.MatchExpressions {
		parts = append(parts, fmt.Sprintf("%s %s %v", expr.Key, expr.Operator, expr.Values))
	}

	return strings.Join(parts, ",")
}

// labelSelectorsMatch checks if two LabelSelectors have overlapping match criteria.
// This is a simplified check that compares match labels for equality.
func labelSelectorsMatch(a, b *metav1.LabelSelector) bool {
	if a == nil || b == nil {
		return false
	}

	// Check if any match labels overlap.
	for key, valA := range a.MatchLabels {
		if valB, ok := b.MatchLabels[key]; ok && valA == valB {
			return true
		}
	}

	return false
}

// cidrContainsIP performs a simple CIDR prefix check to determine if an IP
// address falls within a CIDR range. For production use, consider using
// the net package's ParseCIDR and Contains for full accuracy.
func cidrContainsIP(cidr, ip string) bool {
	// Handle the common case of /0 (all IPs) and /32 or /128 (exact match).
	if strings.HasSuffix(cidr, "/0") {
		return true
	}

	// Extract the network portion of the CIDR.
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		// If it's just an IP with no prefix, do exact match.
		return cidr == ip
	}

	networkIP := parts[0]
	prefixLen := 0
	for _, c := range parts[1] {
		prefixLen = prefixLen*10 + int(c-'0')
	}

	// For /32 (IPv4) or /128 (IPv6), exact match.
	if prefixLen >= 32 && strings.Contains(networkIP, ".") {
		return networkIP == ip
	}
	if prefixLen >= 128 && strings.Contains(networkIP, ":") {
		return networkIP == ip
	}

	// For other prefix lengths, do a simple prefix match.
	// This is a simplified check; for full accuracy, use net.ParseCIDR.
	ipParts := strings.Split(ip, ".")
	netParts := strings.Split(networkIP, ".")

	if len(ipParts) != len(netParts) {
		return false
	}

	// Calculate how many octets to compare based on the prefix length.
	octetsToCompare := prefixLen / 8
	for i := 0; i < octetsToCompare && i < len(ipParts) && i < len(netParts); i++ {
		if ipParts[i] != netParts[i] {
			return false
		}
	}

	return true
}

// GetPoliciesForPod returns all network policies that select pods with the
// given labels in the specified namespace. This is useful for understanding
// which policies apply to a specific attacker pod.
func (n *NetworkPolicyController) GetPoliciesForPod(ctx context.Context, namespace string, podLabels map[string]string) ([]NetworkPolicyInfo, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace must not be empty")
	}

	policies, err := n.ListNetworkPolicies(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies for pod matching: %w", err)
	}

	var matchingPolicies []NetworkPolicyInfo
	for _, policy := range policies {
		selector, err := metav1.LabelSelectorAsSelector(&policy.PodSelector)
		if err != nil {
			n.logger.Warn("failed to convert pod selector",
				zap.String("policy", policy.Name),
				zap.Error(err),
			)
			continue
		}

		if selector.Matches(labels.Set(podLabels)) {
			matchingPolicies = append(matchingPolicies, policy)
		}
	}

	return matchingPolicies, nil
}

// Ensure the intstr import is used (compile-time check).
var _ = intstr.FromInt32
