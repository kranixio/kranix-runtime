package volume

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kranix-io/kranix-packages/types"
)

// K8sManager provisions PVCs and returns pod volume mounts.
type K8sManager struct {
	client    kubernetes.Interface
	namespace string
}

func NewK8sManager(client kubernetes.Interface, namespace string) *K8sManager {
	return &K8sManager{client: client, namespace: namespace}
}

func (m *K8sManager) Provision(ctx context.Context, spec *types.WorkloadSpec) (*types.VolumeLifecycleResult, error) {
	if spec == nil || len(spec.Volumes) == 0 {
		if spec == nil {
			return &types.VolumeLifecycleResult{}, nil
		}
		return &types.VolumeLifecycleResult{WorkloadID: spec.Name}, nil
	}
	ns := spec.Namespace
	if ns == "" {
		ns = m.namespace
	}
	result := &types.VolumeLifecycleResult{
		WorkloadID: spec.Name,
		Volumes:    make([]types.VolumeState, 0, len(spec.Volumes)),
	}
	for _, vol := range spec.Volumes {
		claimName := pvcName(spec.Name, vol.Name)
		size := vol.Size
		if size == "" {
			size = "1Gi"
		}
		accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		if strings.EqualFold(vol.AccessMode, "ReadWriteMany") {
			accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: ns,
				Labels: map[string]string{
					"app":        sanitizeName(spec.Name),
					"managed-by": "kranix",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: accessModes,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(size),
					},
				},
			},
		}
		if vol.StorageClass != "" {
			pvc.Spec.StorageClassName = &vol.StorageClass
		}
		created, err := m.client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, pvc, metav1.CreateOptions{})
		if err != nil && !alreadyExists(err) {
			return nil, fmt.Errorf("create pvc %s: %w", claimName, err)
		}
		status := "pending"
		if created != nil && created.Status.Phase == corev1.ClaimBound {
			status = "bound"
		}
		result.Volumes = append(result.Volumes, types.VolumeState{
			Name:      vol.Name,
			ClaimName: claimName,
			MountPath: vol.MountPath,
			Status:    status,
		})
	}
	return result, nil
}

func (m *K8sManager) Cleanup(ctx context.Context, spec *types.WorkloadSpec) error {
	if spec == nil {
		return nil
	}
	ns := spec.Namespace
	if ns == "" {
		ns = m.namespace
	}
	for _, vol := range spec.Volumes {
		if !vol.AutoCleanup {
			continue
		}
		claimName := pvcName(spec.Name, vol.Name)
		if err := m.client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, claimName, metav1.DeleteOptions{}); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

// CleanupByWorkload removes PVCs labeled for a workload when auto-cleanup is enabled.
func (m *K8sManager) CleanupByWorkload(ctx context.Context, workloadID, namespace string) error {
	ns := namespace
	if ns == "" {
		ns = m.namespace
	}
	list, err := m.client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + sanitizeName(workloadID) + ",managed-by=kranix",
	})
	if err != nil {
		return err
	}
	for _, pvc := range list.Items {
		if err := m.client.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, pvc.Name, metav1.DeleteOptions{}); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

// PodVolumes returns Kubernetes pod volumes and mounts for a workload spec.
func PodVolumes(spec *types.WorkloadSpec) ([]corev1.Volume, []corev1.VolumeMount) {
	if spec == nil || len(spec.Volumes) == 0 {
		return nil, nil
	}
	volumes := make([]corev1.Volume, 0, len(spec.Volumes))
	mounts := make([]corev1.VolumeMount, 0, len(spec.Volumes))
	for _, vol := range spec.Volumes {
		volName := sanitizeName(vol.Name)
		claimName := pvcName(spec.Name, vol.Name)
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
		mountPath := vol.MountPath
		if mountPath == "" {
			mountPath = "/data/" + volName
		}
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: mountPath,
		})
	}
	return volumes, mounts
}

func pvcName(workload, volume string) string {
	return sanitizeName(workload + "-" + volume + "-pvc")
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func alreadyExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "AlreadyExists")
}

func notFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NotFound")
}
