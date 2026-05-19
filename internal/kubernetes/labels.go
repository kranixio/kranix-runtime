package kubernetes

import (
	"strings"

	"github.com/kranix-io/kranix-packages/types"
)

func mergeWorkloadLabels(spec *types.WorkloadSpec, base map[string]string) map[string]string {
	out := make(map[string]string, len(base)+4)
	for k, v := range base {
		out[k] = v
	}
	if spec == nil || spec.Tags == nil {
		return out
	}
	if t := strings.TrimSpace(spec.Tags.Team); t != "" {
		out[types.LabelKeyTeam] = t
	}
	if e := strings.TrimSpace(spec.Tags.Environment); e != "" {
		out[types.LabelKeyEnvironment] = e
	}
	if c := strings.TrimSpace(spec.Tags.CostCenter); c != "" {
		out[types.LabelKeyCostCenter] = c
	}
	for k, v := range spec.Tags.Custom {
		k = strings.TrimSpace(k)
		if k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}
