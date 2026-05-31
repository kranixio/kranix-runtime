package placement

import (
	"strings"

	"github.com/kranix-io/kranix-packages/types"
)

const (
	labelRegion       = "topology.kubernetes.io/region"
	labelZone         = "topology.kubernetes.io/zone"
	labelInstanceType = "node.kubernetes.io/instance-type"
	labelHardware     = "kranix.io/hardware"
)

// ApplyNodePlacement merges region, zone, hardware, and custom labels into scheduling config.
func ApplyNodePlacement(spec *types.WorkloadSpec) {
	if spec == nil {
		return
	}
	if spec.Scheduling == nil {
		spec.Scheduling = &types.SchedulingConfig{}
	}
	s := spec.Scheduling

	if s.NodeSelectors == nil {
		s.NodeSelectors = map[string]string{}
	}

	region := firstNonEmpty(nodePlacementField(s, func(p *types.NodePlacement) string { return p.Region }), firstPreferred(s.PreferredRegions))
	zone := firstNonEmpty(nodePlacementField(s, func(p *types.NodePlacement) string { return p.Zone }), firstPreferred(s.PreferredZones))
	if region != "" {
		s.NodeSelectors[labelRegion] = region
	}
	if zone != "" {
		s.NodeSelectors[labelZone] = zone
	}
	if hw := nodePlacementField(s, func(p *types.NodePlacement) string { return p.HardwareType }); hw != "" {
		s.NodeSelectors[labelHardware] = hw
	}
	if it := nodePlacementField(s, func(p *types.NodePlacement) string { return p.InstanceType }); it != "" {
		s.NodeSelectors[labelInstanceType] = it
	}

	if s.NodePlacement != nil {
		for k, v := range s.NodePlacement.RequiredLabels {
			if k != "" && v != "" {
				s.NodeSelectors[k] = v
			}
		}
		mergePreferredLabels(s, s.NodePlacement.PreferredLabels)
	}
}

func nodePlacementField(s *types.SchedulingConfig, fn func(*types.NodePlacement) string) string {
	if s == nil || s.NodePlacement == nil {
		return ""
	}
	return strings.TrimSpace(fn(s.NodePlacement))
}

func firstPreferred(values []string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

func mergePreferredLabels(s *types.SchedulingConfig, prefs []types.NodeLabelPreference) {
	if len(prefs) == 0 {
		return
	}
	if s.Affinity == nil {
		s.Affinity = &types.AffinityConfig{}
	}
	if s.Affinity.NodeAffinity == nil {
		s.Affinity.NodeAffinity = &types.NodeAffinity{}
	}
	for _, pref := range prefs {
		if pref.Key == "" {
			continue
		}
		weight := pref.Weight
		if weight <= 0 {
			weight = 1
		}
		term := types.NodeSelectorTerm{
			MatchExpressions: []types.NodeSelectorRequirement{
				{Key: pref.Key, Operator: "In", Values: []string{pref.Value}},
			},
		}
		s.Affinity.NodeAffinity.PreferredDuringScheduling = append(
			s.Affinity.NodeAffinity.PreferredDuringScheduling,
			types.PreferredSchedulingTerm{Weight: weight, Preference: term},
		)
	}
}
