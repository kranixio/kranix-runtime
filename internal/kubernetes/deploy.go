package kubernetes

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kranix-io/kranix-packages/types"
)

func (d *Driver) createDeployment(ctx context.Context, spec *types.WorkloadSpec) (*appsv1.Deployment, error) {
	// Create namespace if needed
	if err := d.ensureNamespace(ctx, spec.Namespace); err != nil {
		return nil, fmt.Errorf("failed to ensure namespace: %w", err)
	}

	namespace := spec.Namespace
	if namespace == "" {
		namespace = d.namespace
	}

	// Build container spec
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

	// Create deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        spec.Name,
				"managed-by": "kranix",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(spec.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": spec.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": spec.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}

	return d.clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
}

func (d *Driver) deleteDeployment(ctx context.Context, name string) error {
	namespace := d.namespace
	return d.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (d *Driver) restartDeployment(ctx context.Context, name string) error {
	namespace := d.namespace

	// Get current deployment
	deployment, err := d.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Trigger rollout restart by updating annotations
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kranix/restartedAt"] = "now"

	_, err = d.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (d *Driver) ensureNamespace(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := d.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		// Ignore if already exists
		return nil
	}
	return nil
}

func envMapToEnvVars(env map[string]string) []corev1.EnvVar {
	vars := make([]corev1.EnvVar, 0, len(env))
	for k, v := range env {
		vars = append(vars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}
	return vars
}

func int32Ptr(i int) *int32 {
	i32 := int32(i)
	return &i32
}

// convertAffinityConfig converts kranix AffinityConfig to Kubernetes Affinity.
func convertAffinityConfig(affinity *types.AffinityConfig) *corev1.Affinity {
	if affinity == nil {
		return nil
	}

	k8sAffinity := &corev1.Affinity{}

	if affinity.NodeAffinity != nil {
		k8sAffinity.NodeAffinity = convertNodeAffinity(affinity.NodeAffinity)
	}

	if affinity.PodAffinity != nil {
		k8sAffinity.PodAffinity = convertPodAffinity(affinity.PodAffinity)
	}

	if affinity.PodAntiAffinity != nil {
		k8sAffinity.PodAntiAffinity = convertPodAntiAffinity(affinity.PodAntiAffinity)
	}

	return k8sAffinity
}

// convertNodeAffinity converts kranix NodeAffinity to Kubernetes NodeAffinity.
func convertNodeAffinity(nodeAffinity *types.NodeAffinity) *corev1.NodeAffinity {
	if nodeAffinity == nil {
		return nil
	}

	k8sNodeAffinity := &corev1.NodeAffinity{}

	if len(nodeAffinity.RequiredDuringScheduling) > 0 {
		k8sNodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: convertNodeSelectorTerms(nodeAffinity.RequiredDuringScheduling),
		}
	}

	if len(nodeAffinity.PreferredDuringScheduling) > 0 {
		k8sNodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = convertPreferredSchedulingTerms(nodeAffinity.PreferredDuringScheduling)
	}

	return k8sNodeAffinity
}

// convertNodeSelectorTerms converts kranix NodeSelectorTerm to Kubernetes NodeSelectorTerm.
func convertNodeSelectorTerms(terms []types.NodeSelectorTerm) []corev1.NodeSelectorTerm {
	k8sTerms := make([]corev1.NodeSelectorTerm, len(terms))
	for i, term := range terms {
		k8sTerms[i] = corev1.NodeSelectorTerm{
			MatchExpressions: convertNodeSelectorRequirements(term.MatchExpressions),
			MatchFields:      convertNodeSelectorRequirements(term.MatchFields),
		}
	}
	return k8sTerms
}

// convertNodeSelectorRequirements converts kranix NodeSelectorRequirement to Kubernetes NodeSelectorRequirement.
func convertNodeSelectorRequirements(reqs []types.NodeSelectorRequirement) []corev1.NodeSelectorRequirement {
	k8sReqs := make([]corev1.NodeSelectorRequirement, len(reqs))
	for i, req := range reqs {
		k8sReqs[i] = corev1.NodeSelectorRequirement{
			Key:      req.Key,
			Operator: corev1.NodeSelectorOperator(req.Operator),
			Values:   req.Values,
		}
	}
	return k8sReqs
}

// convertPreferredSchedulingTerms converts kranix PreferredSchedulingTerm to Kubernetes PreferredSchedulingTerm.
func convertPreferredSchedulingTerms(terms []types.PreferredSchedulingTerm) []corev1.PreferredSchedulingTerm {
	k8sTerms := make([]corev1.PreferredSchedulingTerm, len(terms))
	for i, term := range terms {
		k8sTerms[i] = corev1.PreferredSchedulingTerm{
			Weight:     term.Weight,
			Preference: convertNodeSelectorTerm(term.Preference),
		}
	}
	return k8sTerms
}

// convertNodeSelectorTerm converts a single kranix NodeSelectorTerm to Kubernetes NodeSelectorTerm.
func convertNodeSelectorTerm(term types.NodeSelectorTerm) corev1.NodeSelectorTerm {
	return corev1.NodeSelectorTerm{
		MatchExpressions: convertNodeSelectorRequirements(term.MatchExpressions),
		MatchFields:      convertNodeSelectorRequirements(term.MatchFields),
	}
}

// convertPodAffinity converts kranix PodAffinity to Kubernetes PodAffinity.
func convertPodAffinity(podAffinity *types.PodAffinity) *corev1.PodAffinity {
	if podAffinity == nil {
		return nil
	}

	k8sPodAffinity := &corev1.PodAffinity{}

	if len(podAffinity.RequiredDuringScheduling) > 0 {
		k8sPodAffinity.RequiredDuringSchedulingIgnoredDuringExecution = convertPodAffinityTerms(podAffinity.RequiredDuringScheduling)
	}

	if len(podAffinity.PreferredDuringScheduling) > 0 {
		k8sPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution = convertWeightedPodAffinityTerms(podAffinity.PreferredDuringScheduling)
	}

	return k8sPodAffinity
}

// convertPodAffinityTerms converts kranix PodAffinityTerm to Kubernetes PodAffinityTerm.
func convertPodAffinityTerms(terms []types.PodAffinityTerm) []corev1.PodAffinityTerm {
	k8sTerms := make([]corev1.PodAffinityTerm, len(terms))
	for i, term := range terms {
		k8sTerms[i] = corev1.PodAffinityTerm{
			LabelSelector: convertLabelSelector(term.LabelSelector),
			Namespaces:    term.Namespaces,
			TopologyKey:   term.TopologyKey,
		}
	}
	return k8sTerms
}

// convertLabelSelector converts a map to Kubernetes LabelSelector.
func convertLabelSelector(selector map[string]string) *metav1.LabelSelector {
	if selector == nil {
		return nil
	}
	return &metav1.LabelSelector{
		MatchLabels: selector,
	}
}

// convertWeightedPodAffinityTerms converts kranix WeightedPodAffinityTerm to Kubernetes WeightedPodAffinityTerm.
func convertWeightedPodAffinityTerms(terms []types.WeightedPodAffinityTerm) []corev1.WeightedPodAffinityTerm {
	k8sTerms := make([]corev1.WeightedPodAffinityTerm, len(terms))
	for i, term := range terms {
		k8sTerms[i] = corev1.WeightedPodAffinityTerm{
			Weight:          term.Weight,
			PodAffinityTerm: convertPodAffinityTerm(term.PodAffinityTerm),
		}
	}
	return k8sTerms
}

// convertPodAffinityTerm converts a single kranix PodAffinityTerm to Kubernetes PodAffinityTerm.
func convertPodAffinityTerm(term types.PodAffinityTerm) corev1.PodAffinityTerm {
	return corev1.PodAffinityTerm{
		LabelSelector: convertLabelSelector(term.LabelSelector),
		Namespaces:    term.Namespaces,
		TopologyKey:   term.TopologyKey,
	}
}

// convertPodAntiAffinity converts kranix PodAffinity to Kubernetes PodAntiAffinity.
func convertPodAntiAffinity(podAntiAffinity *types.PodAffinity) *corev1.PodAntiAffinity {
	if podAntiAffinity == nil {
		return nil
	}

	k8sPodAntiAffinity := &corev1.PodAntiAffinity{}

	if len(podAntiAffinity.RequiredDuringScheduling) > 0 {
		k8sPodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = convertPodAffinityTerms(podAntiAffinity.RequiredDuringScheduling)
	}

	if len(podAntiAffinity.PreferredDuringScheduling) > 0 {
		k8sPodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = convertWeightedPodAffinityTerms(podAntiAffinity.PreferredDuringScheduling)
	}

	return k8sPodAntiAffinity
}

// convertTolerations converts kranix Toleration to Kubernetes Toleration.
func convertTolerations(tolerations []types.Toleration) []corev1.Toleration {
	k8sTolerations := make([]corev1.Toleration, len(tolerations))
	for i, tol := range tolerations {
		k8sTolerations[i] = corev1.Toleration{
			Key:               tol.Key,
			Operator:          corev1.TolerationOperator(tol.Operator),
			Value:             tol.Value,
			Effect:            corev1.TaintEffect(tol.Effect),
			TolerationSeconds: tol.TolerationSeconds,
		}
	}
	return k8sTolerations
}
