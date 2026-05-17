package podman

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cfg          *config.Config
	rootless     bool
	daemonless   bool
	socketPath   string
	podmanBinary string
}

func New(cfg *config.Config) (kraneTypes.RuntimeDriver, error) {
	// Check if podman is available
	podmanBinary, err := exec.LookPath("podman")
	if err != nil {
		return nil, fmt.Errorf("podman not found: %w", err)
	}

	driver := &Driver{
		cfg:          cfg,
		podmanBinary: podmanBinary,
	}

	// Detect rootless mode
	driver.rootless = driver.detectRootless()

	// Detect daemonless mode
	driver.daemonless = driver.detectDaemonless()

	// Set socket path based on configuration
	if cfg.Podman.Socket != "" {
		driver.socketPath = cfg.Podman.Socket
	} else if driver.rootless {
		// Default rootless socket path
		xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if xdgRuntimeDir != "" {
			driver.socketPath = filepath.Join(xdgRuntimeDir, "podman", "podman.sock")
		} else {
			homeDir, _ := os.UserHomeDir()
			driver.socketPath = filepath.Join(homeDir, ".local", "share", "podman", "podman.sock")
		}
	} else {
		// Default system socket path
		driver.socketPath = "/run/podman/podman.sock"
	}

	// Verify podman is accessible
	if err := driver.verifyPodman(); err != nil {
		return nil, fmt.Errorf("podman verification failed: %w", err)
	}

	return driver, nil
}

func (d *Driver) detectRootless() bool {
	// Check if running as root
	if os.Getuid() != 0 {
		return true
	}
	// Check for rootless environment variables
	if os.Getenv("PODMAN_ROOTLESS") == "1" {
		return true
	}
	return false
}

func (d *Driver) detectDaemonless() bool {
	// Podman is always daemonless by design
	// This flag indicates we're using the socket API directly
	return true
}

func (d *Driver) verifyPodman() error {
	args := []string{"version"}
	cmd := exec.Command(d.podmanBinary, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman version check failed: %w", err)
	}
	return nil
}

func (d *Driver) Deploy(ctx context.Context, spec *kraneTypes.WorkloadSpec) (*kraneTypes.WorkloadStatus, error) {
	// Build podman run command
	args := []string{"run", "-d", "--name", spec.Name}

	// Add rootless-specific options
	if d.rootless {
		args = append(args, "--userns", "keep-id")
	}

	// Add resource limits if specified
	if spec.Resources.CPULimit != "" {
		args = append(args, "--cpus", spec.Resources.CPULimit)
	}
	if spec.Resources.MemoryLimit != "" {
		args = append(args, "--memory", spec.Resources.MemoryLimit)
	}

	// Add GPU support if specified
	if spec.Resources.GPU != nil {
		if spec.Resources.GPU.Vendor == "nvidia" {
			args = append(args, "--device", "nvidia.com/gpu="+strconv.Itoa(int(spec.Resources.GPU.Count)))
		}
	}

	// Add environment variables
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add ports
	for _, port := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", port.ContainerPort, port.ContainerPort))
	}

	args = append(args, spec.Image)

	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman run failed: %w, output: %s", err, string(output))
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	args := []string{"rm", "-f", workloadID}
	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	_, err := cmd.CombinedOutput()
	return err
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	args := []string{"restart", workloadID}
	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	_, err := cmd.CombinedOutput()
	return err
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*kraneTypes.WorkloadStatus, error) {
	args := []string{"inspect", workloadID, "--format", "{{.State.Status}}"}
	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman inspect failed: %w", err)
	}

	state := strings.TrimSpace(string(output))

	// Get image
	args2 := []string{"inspect", workloadID, "--format", "{{.Config.Image}}"}
	cmd2 := exec.CommandContext(ctx, d.podmanBinary, args2...)
	output2, err2 := cmd2.CombinedOutput()
	image := ""
	if err2 == nil {
		image = strings.TrimSpace(string(output2))
	}

	return &kraneTypes.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: mapPodmanState(state),
		Image: image,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*kraneTypes.WorkloadStatus, error) {
	args := []string{"ps", "--format", "{{.ID}}\t{{.Names}}\t{{.State}}\t{{.Image}}"}
	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman ps failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	statuses := make([]*kraneTypes.WorkloadStatus, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 4 {
			statuses = append(statuses, &kraneTypes.WorkloadStatus{
				ID:    parts[0],
				Name:  parts[1],
				State: mapPodmanState(parts[2]),
				Image: parts[3],
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *kraneTypes.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	args := []string{"logs", "-f"}
	if opts.TailLines > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", opts.TailLines))
	}
	args = append(args, podID)

	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)

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
	cmd := exec.CommandContext(ctx, d.podmanBinary, args...)
	return cmd.Run()
}

func (d *Driver) Backend() string {
	return "podman"
}

func (d *Driver) GetInfo() map[string]interface{} {
	return map[string]interface{}{
		"rootless":   d.rootless,
		"daemonless": d.daemonless,
		"socket":     d.socketPath,
		"binary":     d.podmanBinary,
	}
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
