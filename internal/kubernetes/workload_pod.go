package kubernetes

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/arch"
	"github.com/kranix-io/kranix-runtime/internal/gpu"
)

func (d *Driver) workloadContainer(spec *types.WorkloadSpec) (corev1.Container, error) {
	container := corev1.Container{
		Name:  spec.Name,
		Image: spec.Image,
		Env:   envMapToEnvVars(spec.Env),
	}

	if spec.Command != "" {
		container.Command = []string{spec.Command}
	}

	if len(spec.Ports) > 0 {
		ports := make([]corev1.ContainerPort, 0, len(spec.Ports))
		for _, p := range spec.Ports {
			ports = append(ports, corev1.ContainerPort{
				ContainerPort: p.ContainerPort,
				Protocol:      corev1.Protocol(p.Protocol),
			})
		}
		container.Ports = ports
	}

	container.Resources.Requests = make(corev1.ResourceList)
	container.Resources.Limits = make(corev1.ResourceList)

	if spec.Resources.CPURequest != "" {
		container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(spec.Resources.CPURequest)
	}
	if spec.Resources.CPULimit != "" {
		container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(spec.Resources.CPULimit)
	}
	if spec.Resources.MemoryRequest != "" {
		container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(spec.Resources.MemoryRequest)
	}
	if spec.Resources.MemoryLimit != "" {
		container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(spec.Resources.MemoryLimit)
	}

	if spec.Resources.GPU != nil {
		gpuResources, err := gpu.BuildKubernetesResourceRequirements(spec.Resources.GPU)
		if err != nil {
			return container, fmt.Errorf("failed to build GPU resource requirements: %w", err)
		}
		for resourceName, quantity := range gpuResources {
			container.Resources.Limits[corev1.ResourceName(resourceName)] = resource.MustParse(quantity)
		}
	}

	return container, nil
}

func (d *Driver) workloadPodSpec(spec *types.WorkloadSpec) (corev1.PodSpec, error) {
	arch.ApplyArchitectureScheduling(spec)

	container, err := d.workloadContainer(spec)
	if err != nil {
		return corev1.PodSpec{}, err
	}

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}
	if pc := resolvePriorityClassName(spec.Scheduling); pc != "" {
		podSpec.PriorityClassName = pc
	}
	if spec.Scheduling != nil {
		if len(spec.Scheduling.NodeSelectors) > 0 {
			podSpec.NodeSelector = copyStringMap(spec.Scheduling.NodeSelectors)
		}
		if aff := convertAffinityConfig(spec.Scheduling.Affinity); aff != nil {
			podSpec.Affinity = aff
		}
	}
	tols := mergeSpotTolerations(spec)
	if len(tols) > 0 {
		podSpec.Tolerations = convertTolerations(tols)
	}
	if spec.Scheduling != nil && spec.Scheduling.Spot != nil && spec.Scheduling.Spot.Enabled && spec.Scheduling.Spot.RescheduleOnNodeTermination {
		sec := int64(30)
		podSpec.TerminationGracePeriodSeconds = &sec
	}
	return podSpec, nil
}
