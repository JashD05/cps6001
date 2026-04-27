package kubernetes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// mustParseQuantity parses a resource quantity string and panics if it fails.
// This should only be used with validated/constant quantity strings in package
// initialization or configuration where failure indicates a programming error.
// For user-supplied values, use resource.ParseQuantity instead.
func mustParseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse resource quantity %q: %v", s, err))
	}
	return q
}

// newProtocolPtr returns a pointer to the given corev1.Protocol value.
func newProtocolPtr(p corev1.Protocol) *corev1.Protocol {
	return &p
}

// protocolTCP returns a pointer to the TCP protocol constant.
func protocolTCP() *corev1.Protocol {
	return newProtocolPtr(corev1.ProtocolTCP)
}

// protocolUDP returns a pointer to the UDP protocol constant.
func protocolUDP() *corev1.Protocol {
	return newProtocolPtr(corev1.ProtocolUDP)
}

// protocolSCTP returns a pointer to the SCTP protocol constant.
func protocolSCTP() *corev1.Protocol {
	return newProtocolPtr(corev1.ProtocolSCTP)
}

// portFromInt32 creates an intstr.IntOrString from an int32 port value.
// This is used for NetworkPolicy port specifications.
func portFromInt32(port int32) intstr.IntOrString {
	return intstr.FromInt32(port)
}

// portFromString creates an intstr.IntOrString from a named port string.
// This is used for NetworkPolicy port specifications that reference
// named ports rather than numbered ports.
func portFromString(port string) intstr.IntOrString {
	return intstr.FromString(port)
}

// parseProtocol parses a protocol string and returns a corev1.Protocol pointer.
// Returns nil if the protocol string is empty. Returns TCP as default
// if the string is unrecognized.
func parseProtocol(proto string) *corev1.Protocol {
	switch proto {
	case "TCP", "tcp":
		return protocolTCP()
	case "UDP", "udp":
		return protocolUDP()
	case "SCTP", "sctp":
		return protocolSCTP()
	case "":
		return nil
	default:
		// Default to TCP for unrecognized protocols.
		return protocolTCP()
	}
}

// resourceQuantityPtr returns a pointer to the given resource.Quantity.
// This is useful for creating Kubernetes resource requirement objects
// where pointer values are needed.
func resourceQuantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}
