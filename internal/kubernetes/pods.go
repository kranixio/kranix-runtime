package kubernetes

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kranix-io/kranix-packages/types"
)

func (d *Driver) getDeploymentStatus(ctx context.Context, name string) (*types.WorkloadStatus, error) {
	namespace := d.namespace

	deployment, err := d.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	pods, err := d.listPodsForDeployment(ctx, name, namespace)
	if err != nil {
		return nil, err
	}

	status := &types.WorkloadStatus{
		ID:        string(deployment.UID),
		Name:      name,
		Namespace: namespace,
		State:     mapDeploymentState(deployment),
		Image:     deployment.Spec.Template.Spec.Containers[0].Image,
		Replicas:  int(*deployment.Spec.Replicas),
		Ready:     int(deployment.Status.ReadyReplicas),
		Pods:      pods,
	}

	return status, nil
}

func (d *Driver) listDeployments(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error) {
	deployments, err := d.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	statuses := make([]*types.WorkloadStatus, 0, len(deployments.Items))
	for _, dep := range deployments.Items {
		status := &types.WorkloadStatus{
			ID:        string(dep.UID),
			Name:      dep.Name,
			Namespace: dep.Namespace,
			State:     mapDeploymentState(&dep),
			Image:     dep.Spec.Template.Spec.Containers[0].Image,
			Replicas:  int(*dep.Spec.Replicas),
			Ready:     int(dep.Status.ReadyReplicas),
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (d *Driver) listPodsForDeployment(ctx context.Context, deploymentName, namespace string) ([]string, error) {
	pods, err := d.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deploymentName),
	})
	if err != nil {
		return nil, err
	}

	podNames := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podNames = append(podNames, pod.Name)
	}
	return podNames, nil
}

func mapDeploymentState(deployment *appsv1.Deployment) string {
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		if deployment.Status.Replicas > 0 && deployment.Status.Replicas == deployment.Status.ReadyReplicas {
			return "Running"
		}
		if deployment.Status.Replicas > 0 {
			return "Updating"
		}
	}
	return "Pending"
}
