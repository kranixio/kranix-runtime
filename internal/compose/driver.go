package compose

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cfg           *config.Config
	composeBinary string
	composeV2     bool
}

func New(cfg *config.Config) (kraneTypes.RuntimeDriver, error) {
	driver := &Driver{
		cfg: cfg,
	}

	// Try to detect Docker Compose v2 (docker compose)
	if _, err := exec.LookPath("docker"); err == nil {
		// Check if docker compose subcommand is available
		cmd := exec.Command("docker", "compose", "version")
		if err := cmd.Run(); err == nil {
			driver.composeBinary = "docker"
			driver.composeV2 = true
			return driver, nil
		}
	}

	// Fallback to Docker Compose v1 (docker-compose)
	if _, err := exec.LookPath("docker-compose"); err == nil {
		driver.composeBinary = "docker-compose"
		driver.composeV2 = false
		return driver, nil
	}

	return nil, fmt.Errorf("neither docker compose (v2) nor docker-compose (v1) found")
}

func (d *Driver) Deploy(ctx context.Context, spec *kraneTypes.WorkloadSpec) (*kraneTypes.WorkloadStatus, error) {
	composeFile := spec.ComposeFile
	if composeFile == "" {
		return nil, fmt.Errorf("compose file path required")
	}

	var args []string
	if d.composeV2 {
		args = []string{"compose", "-f", composeFile, "-p", spec.Name, "up", "-d"}
	} else {
		args = []string{"-f", composeFile, "-p", spec.Name, "up", "-d"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose up failed: %w, output: %s", err, string(output))
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	var args []string
	if d.composeV2 {
		args = []string{"compose", "-p", workloadID, "down", "--volumes", "--remove-orphans"}
	} else {
		args = []string{"-p", workloadID, "down", "--volumes", "--remove-orphans"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	var args []string
	if d.composeV2 {
		args = []string{"compose", "-p", workloadID, "restart"}
	} else {
		args = []string{"-p", workloadID, "restart"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose restart failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*kraneTypes.WorkloadStatus, error) {
	var args []string
	if d.composeV2 {
		args = []string{"compose", "-p", workloadID, "ps"}
	} else {
		args = []string{"-p", workloadID, "ps"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose ps failed: %w, output: %s", err, string(output))
	}

	// Parse output to get status
	state := "Running"
	if strings.Contains(string(output), "Exited") || strings.Contains(string(output), "exited") {
		state = "Stopped"
	} else if strings.Contains(string(output), "Created") {
		state = "Pending"
	}

	return &kraneTypes.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: state,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*kraneTypes.WorkloadStatus, error) {
	var args []string
	if d.composeV2 {
		args = []string{"compose", "ls", "--all"}
	} else {
		args = []string{"ls", "--all"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compose ls failed: %w, output: %s", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	statuses := make([]*kraneTypes.WorkloadStatus, 0, len(lines)-1)

	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) > 0 {
			state := "Running"
			if len(parts) > 1 && parts[1] != "running" {
				state = mapComposeState(parts[1])
			}
			statuses = append(statuses, &kraneTypes.WorkloadStatus{
				ID:    parts[0],
				Name:  parts[0],
				State: state,
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *kraneTypes.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	var args []string
	if d.composeV2 {
		args = []string{"compose", "logs", "-f", "--tail", fmt.Sprintf("%d", opts.TailLines), podID}
	} else {
		args = []string{"logs", "-f", "--tail", fmt.Sprintf("%d", opts.TailLines), podID}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)

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
	var args []string
	if d.composeV2 {
		args = []string{"compose", "version"}
	} else {
		args = []string{"version"}
	}

	cmd := exec.CommandContext(ctx, d.composeBinary, args...)
	return cmd.Run()
}

func (d *Driver) Backend() string {
	return "compose"
}

func (d *Driver) GetInfo() map[string]interface{} {
	return map[string]interface{}{
		"composeBinary": d.composeBinary,
		"composeV2":     d.composeV2,
	}
}

func mapComposeState(state string) string {
	switch strings.ToLower(state) {
	case "running":
		return "Running"
	case "exited", "stopped":
		return "Stopped"
	case "created":
		return "Pending"
	case "restarting":
		return "Degraded"
	default:
		return "Unknown"
	}
}
