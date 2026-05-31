package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/health"
)

const drainTaintKey = "kranix.io/drain"

// ListBackendHealth returns health score for the kubernetes backend.
func (d *Driver) ListBackendHealth(ctx context.Context) ([]types.BackendHealthReport, error) {
	start := time.Now()
	err := d.Ping(ctx)
	latency := float64(time.Since(start).Milliseconds())
	errorRate := 0.0
	if err != nil {
		errorRate = 1.0
	}
	if d.tracker != nil {
		d.tracker.Record(latency, err != nil)
		latency, errorRate = d.tracker.Snapshot()
	}
	score := health.ScoreFromSignals(latency, errorRate)
	return []types.BackendHealthReport{{
		Backend:   d.Backend(),
		Score:     score,
		LatencyMs: latency,
		ErrorRate: errorRate,
		Healthy:   score >= 50 && err == nil,
		CheckedAt: time.Now().UTC(),
	}}, nil
}

// ListNodeHealth scores each Kubernetes node 0-100.
func (d *Driver) ListNodeHealth(ctx context.Context) ([]types.NodeHealthReport, error) {
	nodes, err := d.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	reports := make([]types.NodeHealthReport, 0, len(nodes.Items))
	now := time.Now().UTC()
	for _, node := range nodes.Items {
		reports = append(reports, d.scoreNode(node, now))
	}
	return reports, nil
}

func (d *Driver) scoreNode(node corev1.Node, checkedAt time.Time) types.NodeHealthReport {
	ready := false
	var conditions []string
	memoryPressure := false
	diskPressure := false
	pidPressure := false
	for _, c := range node.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case corev1.NodeReady:
			ready = true
		case corev1.NodeMemoryPressure:
			memoryPressure = true
			conditions = append(conditions, "MemoryPressure")
		case corev1.NodeDiskPressure:
			diskPressure = true
			conditions = append(conditions, "DiskPressure")
		case corev1.NodePIDPressure:
			pidPressure = true
			conditions = append(conditions, "PIDPressure")
		}
	}
	if !ready {
		conditions = append(conditions, "NotReady")
	}

	draining := nodeDraining(node)
	score := health.ScoreNodeReady(100, ready, memoryPressure, diskPressure, pidPressure, node.Spec.Unschedulable, draining)

	arch := node.Labels["kubernetes.io/arch"]
	if arch == "" {
		arch = node.Labels["beta.kubernetes.io/arch"]
	}

	return types.NodeHealthReport{
		Name:          node.Name,
		Backend:       d.Backend(),
		Score:         score,
		Architecture:  arch,
		Ready:         ready,
		Draining:      draining,
		Unschedulable: node.Spec.Unschedulable,
		Conditions:    conditions,
		CheckedAt:     checkedAt,
	}
}

func nodeDraining(node corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == drainTaintKey {
			return true
		}
	}
	return node.Labels["kranix.io/drain"] == "true"
}

// DrainNode cordons a node and evict workloads before maintenance.
func (d *Driver) DrainNode(ctx context.Context, req types.NodeDrainRequest) (*types.NodeDrainResult, error) {
	if req.NodeName == "" {
		return nil, fmt.Errorf("nodeName is required")
	}

	node, err := d.clientset.CoreV1().Nodes().Get(ctx, req.NodeName, metav1.GetOptions{})
	if err != nil {
		return &types.NodeDrainResult{
			NodeName: req.NodeName,
			Phase:    types.DrainPhaseFailed,
			Message:  err.Error(),
		}, err
	}

	node.Spec.Unschedulable = true
	drainValue := "maintenance"
	if req.Reason != "" {
		drainValue = sanitizeDrainLabel(req.Reason)
	}
	node.Labels = cloneLabels(node.Labels)
	node.Labels["kranix.io/drain"] = "true"
	node.Spec.Taints = appendTaint(node.Spec.Taints, corev1.Taint{
		Key:    drainTaintKey,
		Value:  drainValue,
		Effect: corev1.TaintEffectNoSchedule,
	})

	if _, err := d.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
		return &types.NodeDrainResult{
			NodeName: req.NodeName,
			Phase:    types.DrainPhaseFailed,
			Message:  fmt.Sprintf("cordon failed: %v", err),
		}, err
	}

	grace := int64(30)
	if req.GracePeriodSeconds > 0 {
		grace = int64(req.GracePeriodSeconds)
	}

	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", req.NodeName).String()
	pods, err := d.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return &types.NodeDrainResult{
			NodeName: req.NodeName,
			Phase:    types.DrainPhaseFailed,
			Message:  fmt.Sprintf("list pods: %v", err),
		}, err
	}

	evicted := 0
	remaining := 0
	for _, pod := range pods.Items {
		if req.IgnoreDaemonSets && isDaemonSetPod(&pod) {
			remaining++
			continue
		}
		if pod.Namespace == "kube-system" && pod.Labels["component"] == "kube-proxy" {
			continue
		}
		policy := metav1.DeletePropagationForeground
		deleteOpts := metav1.DeleteOptions{
			GracePeriodSeconds: &grace,
			PropagationPolicy:  &policy,
		}
		if err := d.clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, deleteOpts); err != nil {
			remaining++
			continue
		}
		evicted++
	}

	phase := types.DrainPhaseDrained
	msg := fmt.Sprintf("cordoned node and evicted %d pod(s)", evicted)
	if remaining > 0 {
		phase = types.DrainPhaseEvicting
		msg = fmt.Sprintf("cordoned node; evicted %d pod(s), %d remaining", evicted, remaining)
	}

	return &types.NodeDrainResult{
		NodeName:      req.NodeName,
		Phase:         phase,
		PodsEvicted:   evicted,
		PodsRemaining: remaining,
		Message:       msg,
	}, nil
}

func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

func appendTaint(existing []corev1.Taint, taint corev1.Taint) []corev1.Taint {
	for _, t := range existing {
		if t.Key == taint.Key && t.Effect == taint.Effect {
			return existing
		}
	}
	return append(existing, taint)
}

func cloneLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sanitizeDrainLabel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "-")
	if len(v) > 63 {
		return v[:63]
	}
	return v
}

// Ensure Driver implements NodeOperations at compile time.
var _ types.NodeOperations = (*Driver)(nil)
