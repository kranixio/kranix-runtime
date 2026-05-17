package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cfg       *config.Config
	sshClient *ssh.Client
	host      string
	user      string
	port      int
	connected bool
}

func New(cfg *config.Config) (kraneTypes.RuntimeDriver, error) {
	// Remote driver requires host configuration
	// This is a placeholder - actual implementation would need host config
	return &Driver{
		cfg:       cfg,
		user:      "root",
		port:      22,
		connected: false,
	}, nil
}

func (d *Driver) connect(ctx context.Context, host string) error {
	keyPath := d.expandPath(d.cfg.Remote.SSHKeyPath)
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to parse SSH key: %w", err)
	}

	// Use known_hosts for security
	knownHostsPath := d.expandPath(d.cfg.Remote.KnownHostsPath)
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return fmt.Errorf("failed to create known hosts callback: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            d.user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", host, d.port)
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to dial SSH: %w", err)
	}

	d.sshClient = client
	d.host = host
	d.connected = true
	return nil
}

func (d *Driver) Deploy(ctx context.Context, spec *kraneTypes.WorkloadSpec) (*kraneTypes.WorkloadStatus, error) {
	// Auto-connect if not connected
	if !d.connected {
		// Use configured host or spec host
		host := d.getRemoteHost(spec)
		if err := d.connect(ctx, host); err != nil {
			return nil, fmt.Errorf("failed to connect to remote host: %w", err)
		}
	}

	// Detect runtime on remote host (Docker or Podman)
	runtime, err := d.detectRemoteRuntime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect remote runtime: %w", err)
	}

	// Build runtime-specific command
	cmd := d.buildDeployCommand(spec, runtime)

	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("remote deploy failed: %w, output: %s", err, output)
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) detectRemoteRuntime(ctx context.Context) (string, error) {
	// Check for Docker
	cmd := "which docker"
	output, err := d.executeRemoteCommand(ctx, cmd)
	if err == nil && strings.TrimSpace(output) != "" {
		return "docker", nil
	}

	// Check for Podman
	cmd = "which podman"
	output, err = d.executeRemoteCommand(ctx, cmd)
	if err == nil && strings.TrimSpace(output) != "" {
		return "podman", nil
	}

	return "", fmt.Errorf("no container runtime found on remote host")
}

func (d *Driver) buildDeployCommand(spec *kraneTypes.WorkloadSpec, runtime string) string {
	var cmd string
	if runtime == "docker" {
		cmd = fmt.Sprintf("docker run -d --name %s", spec.Name)
	} else {
		cmd = fmt.Sprintf("podman run -d --name %s", spec.Name)
	}

	// Add environment variables
	for k, v := range spec.Env {
		cmd += fmt.Sprintf(" -e %s=%s", k, v)
	}

	// Add resource limits
	if spec.Resources.CPULimit != "" {
		cmd += fmt.Sprintf(" --cpus=%s", spec.Resources.CPULimit)
	}
	if spec.Resources.MemoryLimit != "" {
		cmd += fmt.Sprintf(" --memory=%s", spec.Resources.MemoryLimit)
	}

	// Add GPU support
	if spec.Resources.GPU != nil {
		if spec.Resources.GPU.Vendor == "nvidia" {
			cmd += fmt.Sprintf(" --device=nvidia.com/gpu=%d", spec.Resources.GPU.Count)
		}
	}

	// Add ports
	for _, port := range spec.Ports {
		cmd += fmt.Sprintf(" -p %d:%d", port.ContainerPort, port.ContainerPort)
	}

	cmd += " " + spec.Image
	return cmd
}

func (d *Driver) getRemoteHost(spec *kraneTypes.WorkloadSpec) string {
	// Check if spec has remote host info
	if spec.RemoteHost != "" {
		return spec.RemoteHost
	}
	// Use default from config or environment
	return os.Getenv("KRANIX_REMOTE_HOST")
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	if !d.connected {
		return fmt.Errorf("not connected to remote host")
	}

	// Try Docker first
	cmd := fmt.Sprintf("docker rm -f %s", workloadID)
	_, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		// Fallback to Podman
		cmd = fmt.Sprintf("podman rm -f %s", workloadID)
		_, err = d.executeRemoteCommand(ctx, cmd)
		if err != nil {
			return fmt.Errorf("failed to destroy workload on remote host: %w", err)
		}
	}

	return nil
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	if !d.connected {
		return fmt.Errorf("not connected to remote host")
	}

	cmd := fmt.Sprintf("docker restart %s", workloadID)
	_, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		cmd = fmt.Sprintf("podman restart %s", workloadID)
		_, err = d.executeRemoteCommand(ctx, cmd)
		if err != nil {
			return fmt.Errorf("failed to restart workload on remote host: %w", err)
		}
	}

	return nil
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*kraneTypes.WorkloadStatus, error) {
	if !d.connected {
		return nil, fmt.Errorf("not connected to remote host")
	}

	cmd := fmt.Sprintf("docker inspect %s --format '{{.State.Status}}'", workloadID)
	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		cmd = fmt.Sprintf("podman inspect %s --format '{{.State.Status}}'", workloadID)
		output, err = d.executeRemoteCommand(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to get status from remote host: %w", err)
		}
	}

	state := strings.TrimSpace(output)
	return &kraneTypes.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: mapDockerState(state),
		Host:  d.host,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*kraneTypes.WorkloadStatus, error) {
	if !d.connected {
		return nil, fmt.Errorf("not connected to remote host")
	}

	cmd := "docker ps --format '{{.Names}}\t{{.State}}\t{{.Image}}'"
	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		cmd = "podman ps --format '{{.Names}}\t{{.State}}\t{{.Image}}'"
		output, err = d.executeRemoteCommand(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to list workloads on remote host: %w", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	statuses := make([]*kraneTypes.WorkloadStatus, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 {
			statuses = append(statuses, &kraneTypes.WorkloadStatus{
				Name:  parts[0],
				State: mapDockerState(parts[1]),
				Image: parts[2],
				Host:  d.host,
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *kraneTypes.LogOptions) (<-chan string, error) {
	if !d.connected {
		return nil, fmt.Errorf("not connected to remote host")
	}

	logChan := make(chan string, 100)

	cmd := fmt.Sprintf("docker logs -f --tail %d %s", opts.TailLines, podID)
	session, err := d.createRemoteSession(ctx)
	if err != nil {
		cmd = fmt.Sprintf("podman logs -f --tail %d %s", opts.TailLines, podID)
		session, err = d.createRemoteSession(ctx)
		if err != nil {
			return nil, err
		}
	}

	session.Stdout = io.Discard
	stderrPipe, err := session.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := session.Start(cmd); err != nil {
		return nil, err
	}

	go func() {
		defer close(logChan)
		defer session.Close()

		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
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
	// Check SSH connection
	if !d.connected {
		// Try to connect to configured host
		host := os.Getenv("KRANIX_REMOTE_HOST")
		if host == "" {
			host = "localhost"
		}
		if err := d.connect(ctx, host); err != nil {
			return fmt.Errorf("failed to ping remote host: %w", err)
		}
	}

	// Check container runtime
	cmd := "docker version"
	_, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		cmd = "podman version"
		_, err = d.executeRemoteCommand(ctx, cmd)
		if err != nil {
			return fmt.Errorf("no container runtime available on remote host")
		}
	}

	return nil
}

func (d *Driver) Backend() string {
	return "remote"
}

func (d *Driver) executeRemoteCommand(ctx context.Context, cmd string) (string, error) {
	if !d.connected {
		return "", fmt.Errorf("not connected to remote host")
	}

	session, err := d.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

func (d *Driver) createRemoteSession(ctx context.Context) (*ssh.Session, error) {
	if !d.connected {
		return nil, fmt.Errorf("not connected to remote host")
	}
	return d.sshClient.NewSession()
}

func (d *Driver) expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func (d *Driver) Disconnect() {
	if d.sshClient != nil {
		d.sshClient.Close()
		d.connected = false
	}
}

func mapDockerState(state string) string {
	switch strings.ToLower(state) {
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
