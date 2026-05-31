package bandwidth

import (
	"strconv"
	"strings"

	"github.com/kranix-io/kranix-packages/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	annotationIngress = "kubernetes.io/ingress-bandwidth"
	annotationEgress  = "kubernetes.io/egress-bandwidth"
)

// ApplyPodAnnotations sets Kubernetes bandwidth annotations when limits are configured.
func ApplyPodAnnotations(meta *metav1.ObjectMeta, spec *types.WorkloadSpec) {
	if spec == nil || spec.NetworkBandwidth == nil || !spec.NetworkBandwidth.Enabled {
		return
	}
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	if v := normalizeLimit(spec.NetworkBandwidth.IngressLimit); v != "" {
		meta.Annotations[annotationIngress] = v
	}
	if v := normalizeLimit(spec.NetworkBandwidth.EgressLimit); v != "" {
		meta.Annotations[annotationEgress] = v
	}
}

// ApplyDockerLabels stores bandwidth limits as container labels.
func ApplyDockerLabels(spec *types.WorkloadSpec) map[string]string {
	if spec == nil || spec.NetworkBandwidth == nil || !spec.NetworkBandwidth.Enabled {
		return nil
	}
	labels := map[string]string{"kranix.io/bandwidth-managed": "true"}
	if v := normalizeLimit(spec.NetworkBandwidth.EgressLimit); v != "" {
		labels["kranix.io/egress-bandwidth"] = v
	}
	if v := normalizeLimit(spec.NetworkBandwidth.IngressLimit); v != "" {
		labels["kranix.io/ingress-bandwidth"] = v
	}
	return labels
}

func normalizeLimit(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasSuffix(lower, "bit") || strings.HasSuffix(lower, "bps") {
		return raw
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return strconv.Itoa(n) + "Mbit"
	}
	return raw
}
