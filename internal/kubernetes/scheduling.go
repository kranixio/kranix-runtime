package kubernetes

import (
	"strings"

	"github.com/kranix-io/kranix-packages/types"
	corev1 "k8s.io/api/core/v1"
)

func resolvePriorityClassName(s *types.SchedulingConfig) string {
	if s == nil {
		return ""
	}
	if s.PriorityClassName != "" {
		return s.PriorityClassName
	}
	lvl := strings.ToLower(strings.TrimSpace(s.WorkloadPriority))
	if lvl == "" {
		return ""
	}
	suffix := "-np"
	if s.PreemptionEnabled {
		suffix = ""
	}
	switch lvl {
	case string(types.WorkloadPriorityCritical):
		return "kranix-critical" + suffix
	case string(types.WorkloadPriorityHigh):
		return "kranix-high" + suffix
	case string(types.WorkloadPriorityLow):
		return "kranix-low" + suffix
	default:
		return "kranix-normal" + suffix
	}
}

func mergeSpotTolerations(spec *types.WorkloadSpec) []types.Toleration {
	base := []types.Toleration{}
	if spec.Scheduling != nil {
		base = append(base, spec.Scheduling.Tolerations...)
	}
	if spec.Scheduling == nil || spec.Scheduling.Spot == nil || !spec.Scheduling.Spot.Enabled {
		return base
	}
	if spec.Scheduling.Spot.RescheduleOnNodeTermination {
		seconds := int64(300)
		base = mergeTolerations(base, []types.Toleration{
			{
				Key:               corev1.TaintNodeNotReady,
				Operator:          string(corev1.TolerationOpExists),
				Effect:            string(corev1.TaintEffectNoExecute),
				TolerationSeconds: &seconds,
			},
			{
				Key:               corev1.TaintNodeUnreachable,
				Operator:          string(corev1.TolerationOpExists),
				Effect:            string(corev1.TaintEffectNoExecute),
				TolerationSeconds: &seconds,
			},
		})
	}
	return base
}

func mergeTolerations(current, extra []types.Toleration) []types.Toleration {
	next := append([]types.Toleration{}, current...)
	for _, t := range extra {
		var ts *int64
		if t.TolerationSeconds != nil {
			v := *t.TolerationSeconds
			ts = &v
		}
		if tolExists(next, t.Key, string(t.Operator), t.Value, t.Effect, ts) {
			continue
		}
		next = append(next, t)
	}
	return next
}

func tolExists(existing []types.Toleration, key, op, val, eff string, seconds *int64) bool {
	for _, e := range existing {
		if e.Key != key || e.Operator != op || e.Value != val || e.Effect != eff {
			continue
		}
		if tolerationSecondsEqual(e.TolerationSeconds, seconds) {
			return true
		}
	}
	return false
}

func tolerationSecondsEqual(a, b *int64) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
