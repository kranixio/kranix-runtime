package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/kranix-io/kranix-packages/types"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	namespaceNameLabelKey = "kubernetes.io/metadata.name"
	kubeSystemNS          = "kube-system"
)

func ensureCrossNamespaceNetworkPolicy(ctx context.Context, cs kubernetes.Interface, namespace, workloadName string, selector map[string]string, policy *types.CrossNamespaceTrafficPolicy) error {
	if policy == nil || !policy.Enabled {
		return nil
	}

	if workloadName == "" {
		workloadName = "workload"
	}
	name := fmt.Sprintf("kranix-ns-traffic-%s", sanitizePolicyName(workloadName))

	allowSameNs := policy.AllowSameNamespace == nil || *policy.AllowSameNamespace

	ingress := []networkingv1.NetworkPolicyIngressRule{}
	fromPeers := []networkingv1.NetworkPolicyPeer{}

	if allowSameNs {
		fromPeers = append(fromPeers, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{},
		})
	}
	for _, ns := range policy.AllowedIngressNamespaces {
		if ns == "" {
			continue
		}
		fromPeers = append(fromPeers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{namespaceNameLabelKey: ns},
			},
			PodSelector: &metav1.LabelSelector{},
		})
	}
	if len(fromPeers) > 0 {
		ingress = append(ingress, networkingv1.NetworkPolicyIngressRule{From: fromPeers})
	}

	egress := []networkingv1.NetworkPolicyEgressRule{}
	toPeers := []networkingv1.NetworkPolicyPeer{}

	if allowSameNs {
		toPeers = append(toPeers, networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{},
		})
	}
	for _, ns := range policy.AllowedEgressNamespaces {
		if ns == "" {
			continue
		}
		toPeers = append(toPeers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{namespaceNameLabelKey: ns},
			},
			PodSelector: &metav1.LabelSelector{},
		})
	}

	ud := corev1.ProtocolUDP
	td := corev1.ProtocolTCP
	pUDP := intstr.FromInt32(53)
	pTCP := intstr.FromInt32(53)
	if !policy.BlockClusterDNS {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &ud, Port: &pUDP},
				{Protocol: &td, Port: &pTCP},
			},
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{namespaceNameLabelKey: kubeSystemNS},
					},
				},
			},
		})
	}

	if policy.AllowEgressInternet {
		tc := corev1.ProtocolTCP
		w80 := intstr.FromInt32(80)
		w443 := intstr.FromInt32(443)
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tc, Port: &w80},
				{Protocol: &tc, Port: &w443},
			},
		})
	}

	if len(toPeers) > 0 {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{To: toPeers})
	}

	if len(ingress) == 0 && len(toPeers) == 0 && !policy.AllowEgressInternet && policy.BlockClusterDNS {
		return fmt.Errorf("crossNamespaceTraffic.enabled blocks DNS and defines no permissive egress rules")
	}

	policyTypes := []networkingv1.PolicyType{}
	if len(ingress) > 0 {
		policyTypes = append(policyTypes, networkingv1.PolicyTypeIngress)
	}
	if len(egress) > 0 {
		policyTypes = append(policyTypes, networkingv1.PolicyTypeEgress)
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kranix",
				"kranix.io/workload":           workloadName,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: copyStringMap(selector),
			},
			PolicyTypes: policyTypes,
			Ingress:     ingress,
			Egress:      egress,
		},
	}

	if _, err := cs.NetworkingV1().NetworkPolicies(namespace).Create(ctx, np, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create network policy %s/%s: %w", namespace, name, err)
	}
	return nil
}

func sanitizePolicyName(name string) string {
	s := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	if len(s) <= 40 {
		return s
	}
	return s[:40]
}
