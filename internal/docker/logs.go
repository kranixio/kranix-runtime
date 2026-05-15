package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
)

func (d *Driver) getLogs(ctx context.Context, containerID string, tail int, follow bool) (io.ReadCloser, error) {
	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
		Follow:     follow,
		Timestamps: true,
	}

	return d.cli.ContainerLogs(ctx, containerID, opts)
}

func (d *Driver) waitForLogReady(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for container logs")
		default:
			container, err := d.cli.ContainerInspect(ctx, containerID)
			if err != nil {
				return err
			}
			if container.State.Running {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
