package compose

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
	// Check if docker-compose is available
	if _, err := exec.LookPath("docker-compose"); err != nil {
		return nil, fmt.Errorf("docker-compose not found: %w", err)
	}

	return &Driver{
		cfg: cfg,
	}, nil
}

func (d *Driver) Deploy(ctx context.Context, spec *types.WorkloadSpec) (*types.WorkloadStatus, error) {
	// For compose, we assume spec contains a compose file path or we generate one
	// This is a simplified implementation
	composeFile := spec.ComposeFile
	if composeFile == "" {
		return nil, fmt.Errorf("compose file path required")
	}

	args := []string{"-f", composeFile, "up", "-d"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose up failed: %w, output: %s", err, string(output))
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	// workloadID is the compose project name
	args := []string{"-p", workloadID, "down"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	args := []string{"-p", workloadID, "restart"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose restart failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*types.WorkloadStatus, error) {
	args := []string{"-p", workloadID, "ps"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose ps failed: %w, output: %s", err, string(output))
	}

	// Parse output to get status
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("no services found")
	}

	// Simple parsing - in production would be more robust
	state := "Running"
	if strings.Contains(string(output), "Exited") {
		state = "Stopped"
	}

	return &types.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: state,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error) {
	// List all compose projects in current directory
	args := []string{"ls"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose ls failed: %w, output: %s", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	statuses := make([]*types.WorkloadStatus, 0, len(lines)-1)

	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) > 0 {
			statuses = append(statuses, &types.WorkloadStatus{
				ID:    parts[0],
				Name:  parts[0],
				State: "Running", // Simplified
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	args := []string{"logs", "-f", "--tail", fmt.Sprintf("%d", opts.TailLines), podID}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		defer close(logChan)
		buf := make([]byte, 1024)
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

	return logChan, nil
}

func (d *Driver) Ping(ctx context.Context) error {
	args := []string{"version"}
	cmd := exec.CommandContext(ctx, "docker-compose", args...)
	return cmd.Run()
}

func (d *Driver) Backend() string {
	return "compose"
}
