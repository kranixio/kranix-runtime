package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/kranix-io/kranix-packages/types"
)

type DockerVolumeManager struct {
	cli *Driver
}

func NewDockerVolumeManager(d *Driver) *DockerVolumeManager {
	return &DockerVolumeManager{cli: d}
}

func (m *DockerVolumeManager) Provision(ctx context.Context, spec *types.WorkloadSpec) (*types.VolumeLifecycleResult, error) {
	if spec == nil || len(spec.Volumes) == 0 {
		if spec == nil {
			return &types.VolumeLifecycleResult{}, nil
		}
		return &types.VolumeLifecycleResult{WorkloadID: spec.Name}, nil
	}
	result := &types.VolumeLifecycleResult{
		WorkloadID: spec.Name,
		Volumes:    make([]types.VolumeState, 0, len(spec.Volumes)),
	}
	for _, vol := range spec.Volumes {
		name := volumeName(spec.Name, vol.Name)
		created, err := m.cli.cli.VolumeCreate(ctx, volume.CreateOptions{
			Name:   name,
			Labels: map[string]string{"kranix.io/workload": spec.Name, "managed-by": "kranix"},
		})
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("create volume %s: %w", name, err)
		}
		volID := name
		if created.Name != "" {
			volID = created.Name
		}
		mountPath := vol.MountPath
		if mountPath == "" {
			mountPath = "/data/" + vol.Name
		}
		result.Volumes = append(result.Volumes, types.VolumeState{
			Name:      vol.Name,
			VolumeID:  volID,
			MountPath: mountPath,
			Status:    "bound",
		})
	}
	return result, nil
}

func (m *DockerVolumeManager) Cleanup(ctx context.Context, spec *types.WorkloadSpec) error {
	if spec == nil {
		return nil
	}
	for _, vol := range spec.Volumes {
		if !vol.AutoCleanup {
			continue
		}
		name := volumeName(spec.Name, vol.Name)
		if err := m.cli.cli.VolumeRemove(ctx, name, true); err != nil {
			if !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}
	return nil
}

// CleanupByWorkload removes Docker volumes labeled for a workload.
func (m *DockerVolumeManager) CleanupByWorkload(ctx context.Context, workloadID string) error {
	vols, err := m.cli.cli.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "label",
			Value: "kranix.io/workload=" + workloadID,
		}),
	})
	if err != nil {
		return err
	}
	for _, v := range vols.Volumes {
		if err := m.cli.cli.VolumeRemove(ctx, v.Name, true); err != nil {
			if !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}
	return nil
}

func (m *DockerVolumeManager) Binds(spec *types.WorkloadSpec) []string {
	if spec == nil {
		return nil
	}
	binds := make([]string, 0, len(spec.Volumes))
	for _, vol := range spec.Volumes {
		name := volumeName(spec.Name, vol.Name)
		mountPath := vol.MountPath
		if mountPath == "" {
			mountPath = "/data/" + vol.Name
		}
		binds = append(binds, name+":"+mountPath)
	}
	return binds
}

func volumeName(workload, vol string) string {
	return strings.ToLower(strings.ReplaceAll(workload+"-"+vol, "_", "-"))
}

func (d *Driver) resolveContainerID(ctx context.Context, workloadID string) (string, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: workloadID,
		}),
	})
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", fmt.Errorf("container not found: %s", workloadID)
	}
	return containers[0].ID, nil
}
