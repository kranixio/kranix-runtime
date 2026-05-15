package podman

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cfg *config.Config
}

func New(cfg *config.Config) (types.RuntimeDriver, error) {
	// Check if podman is available
	if _, err := exec.LookPath("podman"); err != nil {
		return nil, fmt.Errorf("podman not found: %w", err)
	}

	return &Driver{
		cfg: cfg,
	}, nil
}

func (d *Driver) Deploy(ctx context.Context, spec *types.WorkloadSpec) (*types.WorkloadStatus, error) {
	// Build podman run command
	args := []string{"run", "-d", "--name", spec.Name}

	// Add environment variables
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, spec.Image)

	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman run failed: %w, output: %s", err, string(output))
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	args := []string{"rm", "-f", workloadID}
	cmd := exec.CommandContext(ctx, "podman", args...)
	_, err := cmd.CombinedOutput()
	return err
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	args := []string{"restart", workloadID}
	cmd := exec.CommandContext(ctx, "podman", args...)
	_, err := cmd.CombinedOutput()
	return err
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*types.WorkloadStatus, error) {
	args := []string{"inspect", workloadID, "--format", "{{.State}}"}
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman inspect failed: %w", err)
	}

	state := strings.TrimSpace(string(output))

	// Get image
	args2 := []string{"inspect", workloadID, "--format", "{{.Config.Image}}"}
	cmd2 := exec.CommandContext(ctx, "podman", args2...)
	output2, err2 := cmd2.CombinedOutput()
	image := ""
	if err2 == nil {
		image = strings.TrimSpace(string(output2))
	}

	return &types.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: mapPodmanState(state),
		Image: image,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error) {
	args := []string{"ps", "--format", "{{.ID}}\t{{.Names}}\t{{.State}}\t{{.Image}}"}
	cmd := exec.CommandContext(ctx, "podman", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman ps failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	statuses := make([]*types.WorkloadStatus, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 4 {
			statuses = append(statuses, &types.WorkloadStatus{
				ID:    parts[0],
				Name:  parts[1],
				State: mapPodmanState(parts[2]),
				Image: parts[3],
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	args := []string{"logs", "-f"}
	if opts.TailLines > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", opts.TailLines))
	}
	args = append(args, podID)

	cmd := exec.CommandContext(ctx, "podman", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		defer close(logChan)
		defer cmd.Wait()

		buf := make([]byte, 1024)

		// Read from stdout
		go func() {
			for {
				n, err := stdout.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					logChan <- string(buf[:n])
				}
			}
		}()

		// Read from stderr
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				logChan <- string(buf[:n])
			}
		}
	}()

	return logChan, nil
}

func (d *Driver) Ping(ctx context.Context) error {
	args := []string{"version"}
	cmd := exec.CommandContext(ctx, "podman", args...)
	return cmd.Run()
}

func (d *Driver) Backend() string {
	return "podman"
}

func mapPodmanState(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return "Running"
	case "exited", "stopped":
		return "Stopped"
	case "created":
		return "Pending"
	default:
		return "Unknown"
	}
}
