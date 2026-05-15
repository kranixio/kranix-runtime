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
