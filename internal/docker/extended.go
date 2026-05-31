package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/checkpoint"
)

func (d *Driver) CheckpointWorkload(ctx context.Context, req types.CheckpointRequest) (*types.CheckpointResult, error) {
	if err := checkpoint.ValidateCheckpoint(req); err != nil {
		return nil, err
	}
	id, err := d.resolveContainerID(ctx, req.WorkloadID)
	if err != nil {
		return nil, err
	}
	if err := d.cli.ContainerPause(ctx, id); err != nil {
		return nil, fmt.Errorf("pause container: %w", err)
	}
	result := types.CheckpointResult{
		WorkloadID:   req.WorkloadID,
		CheckpointID: id,
		State:        "paused",
		Message:      "container paused (checkpoint)",
	}
	saved := d.checkpoints.Save(req.WorkloadID, result)
	return &saved, nil
}

func (d *Driver) RestoreWorkload(ctx context.Context, req types.RestoreRequest) (*types.RestoreResult, error) {
	if err := checkpoint.ValidateRestore(req); err != nil {
		return nil, err
	}
	id := req.CheckpointID
	if id == "" {
		var err error
		id, err = d.resolveContainerID(ctx, req.WorkloadID)
		if err != nil {
			return nil, err
		}
	}
	if err := d.cli.ContainerUnpause(ctx, id); err != nil {
		if startErr := d.cli.ContainerStart(ctx, id, container.StartOptions{}); startErr != nil {
			return nil, fmt.Errorf("restore container: %w", err)
		}
	}
	return &types.RestoreResult{
		WorkloadID: req.WorkloadID,
		State:      "running",
		RestoredAt: time.Now().UTC(),
		Message:    "container resumed",
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
