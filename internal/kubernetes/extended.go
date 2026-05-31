package kubernetes

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/checkpoint"
)

func (d *Driver) CheckpointWorkload(ctx context.Context, req types.CheckpointRequest) (*types.CheckpointResult, error) {
	if err := checkpoint.ValidateCheckpoint(req); err != nil {
		return nil, err
	}
	ns := req.Namespace
	if ns == "" {
		ns = d.namespace
	}
	dep, err := d.clientset.AppsV1().Deployments(ns).Get(ctx, req.WorkloadID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	zero := int32(0)
	dep.Spec.Replicas = &zero
	if _, err := d.clientset.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	ckptID := fmt.Sprintf("ckpt-%d", time.Now().UnixNano())
	result := types.CheckpointResult{
		WorkloadID:   req.WorkloadID,
		CheckpointID: ckptID,
		State:        "checkpointed",
		Message:      "scaled deployment to zero replicas",
	}
	saved := d.checkpoints.Save(req.WorkloadID, result)
	return &saved, nil
}

func (d *Driver) RestoreWorkload(ctx context.Context, req types.RestoreRequest) (*types.RestoreResult, error) {
	if err := checkpoint.ValidateRestore(req); err != nil {
		return nil, err
	}
	ns := req.Namespace
	if ns == "" {
		ns = d.namespace
	}
	dep, err := d.clientset.AppsV1().Deployments(ns).Get(ctx, req.WorkloadID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	replicas := int32(1)
	dep.Spec.Replicas = &replicas
	if _, err := d.clientset.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	return &types.RestoreResult{
		WorkloadID: req.WorkloadID,
		State:      "running",
		RestoredAt: time.Now().UTC(),
		Message:    "restored deployment replicas",
	}, nil
}

func (d *Driver) ListCheckpoints(ctx context.Context, workloadID, namespace string) ([]types.CheckpointResult, error) {
	return d.checkpoints.List(workloadID), nil
}

func (d *Driver) ProvisionVolumes(ctx context.Context, spec *types.WorkloadSpec) (*types.VolumeLifecycleResult, error) {
	return d.volMgr.Provision(ctx, spec)
}

func (d *Driver) CleanupVolumes(ctx context.Context, spec *types.WorkloadSpec) error {
	return d.volMgr.Cleanup(ctx, spec)
}

var _ types.RuntimeExtendedOperations = (*Driver)(nil)
