package arch

import (
	"strings"

	"github.com/kranix-io/kranix-packages/types"
)

const k8sArchLabel = "kubernetes.io/arch"

// DetectFromImage infers CPU architecture from image name/tag heuristics.
func DetectFromImage(image string) types.Architecture {
	lower := strings.ToLower(image)
	switch {
	case strings.Contains(lower, "arm64"), strings.Contains(lower, "aarch64"):
		return types.ArchARM64
	case strings.Contains(lower, "arm/v7"), strings.Contains(lower, "arm32"):
		return types.Architecture("arm")
	case strings.Contains(lower, "amd64"), strings.Contains(lower, "x86_64"):
		return types.ArchAMD64
	default:
		return ""
	}
}

// NormalizeArchitecture maps aliases to Kubernetes arch values.
func NormalizeArchitecture(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "arm64", "aarch64":
		return string(types.ArchARM64)
	case "amd64", "x86_64", "x64":
		return string(types.ArchAMD64)
	default:
		return strings.ToLower(strings.TrimSpace(arch))
	}
}

// ResolveArchitecture picks explicit scheduling architecture or image inference.
func ResolveArchitecture(spec *types.WorkloadSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Scheduling != nil && spec.Scheduling.Architecture != "" {
		return NormalizeArchitecture(spec.Scheduling.Architecture)
	}
	if detected := DetectFromImage(spec.Image); detected != "" {
		return string(detected)
	}
	return ""
}

// ApplyArchitectureScheduling injects kubernetes.io/arch node selection for multi-arch routing.
func ApplyArchitectureScheduling(spec *types.WorkloadSpec) {
	arch := ResolveArchitecture(spec)
	if arch == "" {
		return
	}
	if spec.Scheduling == nil {
		spec.Scheduling = &types.SchedulingConfig{}
	}
	if spec.Scheduling.NodeSelectors == nil {
		spec.Scheduling.NodeSelectors = map[string]string{}
	}
	spec.Scheduling.NodeSelectors[k8sArchLabel] = arch

	if spec.Scheduling.Affinity == nil {
		spec.Scheduling.Affinity = &types.AffinityConfig{}
	}
	if spec.Scheduling.Affinity.NodeAffinity == nil {
		spec.Scheduling.Affinity.NodeAffinity = &types.NodeAffinity{}
	}
	required := spec.Scheduling.Affinity.NodeAffinity.RequiredDuringScheduling
	hasArch := false
	for _, term := range required {
		for _, expr := range term.MatchExpressions {
			if expr.Key == k8sArchLabel {
				hasArch = true
				break
			}
		}
	}
	if !hasArch {
		spec.Scheduling.Affinity.NodeAffinity.RequiredDuringScheduling = append(required, types.NodeSelectorTerm{
			MatchExpressions: []types.NodeSelectorRequirement{
				{Key: k8sArchLabel, Operator: "In", Values: []string{arch}},
			},
		})
	}
}

// DockerPlatform returns the OCI platform string for local Docker/Podman runs.
func DockerPlatform(spec *types.WorkloadSpec) string {
	arch := ResolveArchitecture(spec)
	switch arch {
	case string(types.ArchARM64):
		return "linux/arm64"
	case string(types.ArchAMD64):
		return "linux/amd64"
	default:
		return ""
	}
}
