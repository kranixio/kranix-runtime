package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cli *client.Client
	cfg *config.Config
}

func New(cfg *config.Config) (kraneTypes.RuntimeDriver, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost(cfg.Docker.Host),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Driver{
		cli: cli,
		cfg: cfg,
	}, nil
}

func (d *Driver) Deploy(ctx context.Context, spec *kraneTypes.WorkloadSpec) (*kraneTypes.WorkloadStatus, error) {
	// Pull image
	if err := d.pullImage(ctx, spec.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	env := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	containerConfig := &container.Config{
		Image: spec.Image,
		Env:   env,
	}

	if spec.Command != "" {
		containerConfig.Cmd = []string{spec.Command}
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	resp, err := d.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	if err := d.cli.ContainerRemove(ctx, workloadID, container.RemoveOptions{
		Force: true,
	}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	if err := d.cli.ContainerRestart(ctx, workloadID, container.StopOptions{}); err != nil {
		return fmt.Errorf("failed to restart container: %w", err)
	}
	return nil
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*kraneTypes.WorkloadStatus, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: workloadID,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("container not found: %s", workloadID)
	}

	c := containers[0]
	return &kraneTypes.WorkloadStatus{
		ID:    c.ID,
		Name:  workloadID,
		State: mapState(c.State),
		Image: c.Image,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*kraneTypes.WorkloadStatus, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	statuses := make([]*kraneTypes.WorkloadStatus, 0, len(containers))
	for _, c := range containers {
		statuses = append(statuses, &kraneTypes.WorkloadStatus{
			ID:    c.ID,
			Name:  c.Names[0],
			State: mapState(c.State),
			Image: c.Image,
		})
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *kraneTypes.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	reader, err := d.cli.ContainerLogs(ctx, podID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       fmt.Sprintf("%d", opts.TailLines),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	go func() {
		defer close(logChan)
		defer reader.Close()

		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					return
				}
				break
			}
			if n > 0 {
				logChan <- string(buf[:n])
			}
		}
	}()

	return logChan, nil
}

func (d *Driver) Ping(ctx context.Context) error {
	_, err := d.cli.Ping(ctx)
	return err
}

func (d *Driver) Backend() string {
	return "docker"
}

func (d *Driver) pullImage(ctx context.Context, ref string) error {
	reader, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read image pull response: %w", err)
	}

	return nil
}

func mapState(state string) string {
	switch state {
	case "running":
		return "Running"
	case "exited":
		return "Stopped"
	case "created":
		return "Pending"
	default:
		return "Unknown"
	}
}
