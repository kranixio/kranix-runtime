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

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

type Driver struct {
	cfg       *config.Config
	sshClient *ssh.Client
	host      string
}

func New(cfg *config.Config) (types.RuntimeDriver, error) {
	// Remote driver requires host configuration
	// This is a placeholder - actual implementation would need host config
	return &Driver{
		cfg: cfg,
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

	config := &ssh.ClientConfig{
		User: "root", // Configurable
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Should use known_hosts
		Timeout:         30 * time.Second,
	}

	client, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial SSH: %w", err)
	}

	d.sshClient = client
	d.host = host
	return nil
}

func (d *Driver) Deploy(ctx context.Context, spec *types.WorkloadSpec) (*types.WorkloadStatus, error) {
	// Execute docker run on remote host
	cmd := fmt.Sprintf("docker run -d --name %s %s", spec.Name, spec.Image)
	for k, v := range spec.Env {
		cmd += fmt.Sprintf(" -e %s=%s", k, v)
	}

	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("remote deploy failed: %w, output: %s", err, output)
	}

	return d.GetStatus(ctx, spec.Name)
}

func (d *Driver) Destroy(ctx context.Context, workloadID string) error {
	cmd := fmt.Sprintf("docker rm -f %s", workloadID)
	_, err := d.executeRemoteCommand(ctx, cmd)
	return err
}

func (d *Driver) Restart(ctx context.Context, workloadID string) error {
	cmd := fmt.Sprintf("docker restart %s", workloadID)
	_, err := d.executeRemoteCommand(ctx, cmd)
	return err
}

func (d *Driver) GetStatus(ctx context.Context, workloadID string) (*types.WorkloadStatus, error) {
	cmd := fmt.Sprintf("docker inspect %s --format '{{.State.Status}}'", workloadID)
	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}

	state := strings.TrimSpace(output)
	return &types.WorkloadStatus{
		ID:    workloadID,
		Name:  workloadID,
		State: mapDockerState(state),
		Host:  d.host,
	}, nil
}

func (d *Driver) ListWorkloads(ctx context.Context, namespace string) ([]*types.WorkloadStatus, error) {
	cmd := "docker ps --format '{{.Names}}\t{{.State}}\t{{.Image}}'"
	output, err := d.executeRemoteCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	statuses := make([]*types.WorkloadStatus, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 {
			statuses = append(statuses, &types.WorkloadStatus{
				Name:  parts[0],
				State: mapDockerState(parts[1]),
				Image: parts[2],
				Host:  d.host,
			})
		}
	}

	return statuses, nil
}

func (d *Driver) StreamLogs(ctx context.Context, podID string, opts *types.LogOptions) (<-chan string, error) {
	logChan := make(chan string, 100)

	cmd := fmt.Sprintf("docker logs -f --tail %d %s", opts.TailLines, podID)
	session, err := d.createRemoteSession(ctx)
	if err != nil {
		return nil, err
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
	cmd := "docker version"
	_, err := d.executeRemoteCommand(ctx, cmd)
	return err
}

func (d *Driver) Backend() string {
	return "remote"
}

func (d *Driver) executeRemoteCommand(ctx context.Context, cmd string) (string, error) {
	if d.sshClient == nil {
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
	if d.sshClient == nil {
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
