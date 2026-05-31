package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/internal/arch"
)

func (d *Driver) deployContainer(ctx context.Context, spec *types.WorkloadSpec) (string, error) {
	arch.ApplyArchitectureScheduling(spec)
	platform := arch.DockerPlatform(spec)

	containerConfig := &container.Config{
		Image: spec.Image,
		Env:   envMapToSlice(spec.Env),
	}

	if platform != "" {
		_, err := d.cli.ImagePull(ctx, spec.Image, image.PullOptions{Platform: platform})
		if err != nil {
			return "", fmt.Errorf("image pull failed for platform %s: %w", platform, err)
		}
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	networkingConfig := &network.NetworkingConfig{}

	resp, err := d.cli.ContainerCreate(ctx, containerConfig, hostConfig, networkingConfig, nil, spec.Name)
	if err != nil {
		return "", fmt.Errorf("container create failed: %w", err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("container start failed: %w", err)
	}

	return resp.ID, nil
}

func envMapToSlice(env map[string]string) []string {
	slice := make([]string, 0, len(env))
	for k, v := range env {
		slice = append(slice, fmt.Sprintf("%s=%s", k, v))
	}
	return slice
}
