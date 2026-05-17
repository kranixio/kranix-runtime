package ephemeral

import (
	"context"
	"fmt"
	"sync"
	"time"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

const (
	PhaseCreating    = "Creating"
	PhaseReady       = "Ready"
	PhaseTerminating = "Terminating"
	PhaseTerminated  = "Terminated"
)

type Manager struct {
	cfg             *config.Config
	environments    map[string]*kraneTypes.EphemeralEnvironmentStatus
	mu              sync.RWMutex
	driver          kraneTypes.RuntimeDriver
	stopChan        chan struct{}
	cleanupInterval time.Duration
}

func NewManager(cfg *config.Config, driver kraneTypes.RuntimeDriver) *Manager {
	return &Manager{
		cfg:             cfg,
		environments:    make(map[string]*kraneTypes.EphemeralEnvironmentStatus),
		driver:          driver,
		stopChan:        make(chan struct{}),
		cleanupInterval: 5 * time.Minute,
	}
}

func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupExpiredEnvironments(ctx)
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		}
	}
}

func (m *Manager) Stop() {
	close(m.stopChan)
}

// CreateEnvironment creates a new ephemeral environment based on the spec
func (m *Manager) CreateEnvironment(ctx context.Context, spec *kraneTypes.EphemeralEnvironmentSpec, workloadSpec *kraneTypes.WorkloadSpec) (*kraneTypes.EphemeralEnvironmentStatus, error) {
	if !spec.Enabled {
		return nil, fmt.Errorf("ephemeral environment is disabled")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check max concurrent environments
	activeCount := m.countActiveEnvironments()
	if spec.MaxEnvironments > 0 && int32(activeCount) >= spec.MaxEnvironments {
		return nil, fmt.Errorf("max concurrent environments (%d) reached", spec.MaxEnvironments)
	}

	// Generate environment name
	envName := m.generateEnvironmentName(spec)
	envID := fmt.Sprintf("ephem-%d", time.Now().UnixNano())

	// Calculate expiration time
	ttl, err := time.ParseDuration(spec.TTL)
	if err != nil {
		return nil, fmt.Errorf("invalid TTL duration: %w", err)
	}
	expiresAt := time.Now().Add(ttl)

	// Create status
	status := &kraneTypes.EphemeralEnvironmentStatus{
		ID:          envID,
		Name:        envName,
		Namespace:   m.generateNamespace(spec, envName),
		Phase:       PhaseCreating,
		TriggerType: spec.TriggerType,
		PRNumber:    spec.PRNumber,
		BranchName:  spec.BranchName,
		CommitSHA:   spec.CommitSHA,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}

	m.environments[envID] = status

	// Deploy workload in the ephemeral namespace
	workloadSpec.Namespace = status.Namespace
	_, err = m.driver.Deploy(ctx, workloadSpec)
	if err != nil {
		status.Phase = PhaseTerminated
		status.TerminationReason = fmt.Sprintf("deployment failed: %v", err)
		now := time.Now()
		status.TerminatedAt = &now
		return status, err
	}

	status.Phase = PhaseReady
	return status, nil
}

// TeardownEnvironment tears down an ephemeral environment
func (m *Manager) TeardownEnvironment(ctx context.Context, envID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, exists := m.environments[envID]
	if !exists {
		return fmt.Errorf("environment not found: %s", envID)
	}

	if status.Phase == PhaseTerminated {
		return nil
	}

	status.Phase = PhaseTerminating

	// Destroy workload in the namespace
	err := m.driver.Destroy(ctx, status.Name)
	if err != nil {
		return fmt.Errorf("failed to destroy workload: %w", err)
	}

	status.Phase = PhaseTerminated
	status.TerminationReason = reason
	now := time.Now()
	status.TerminatedAt = &now

	return nil
}

// GetEnvironment returns the status of an ephemeral environment
func (m *Manager) GetEnvironment(envID string) (*kraneTypes.EphemeralEnvironmentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.environments[envID]
	if !exists {
		return nil, fmt.Errorf("environment not found: %s", envID)
	}

	return status, nil
}

// ListEnvironments returns all ephemeral environments
func (m *Manager) ListEnvironments() []*kraneTypes.EphemeralEnvironmentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	environments := make([]*kraneTypes.EphemeralEnvironmentStatus, 0, len(m.environments))
	for _, env := range m.environments {
		environments = append(environments, env)
	}

	return environments
}

// HandlePRMerge handles PR merge event for auto-teardown
func (m *Manager) HandlePRMerge(ctx context.Context, prNumber int32) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, env := range m.environments {
		if env.PRNumber == prNumber && env.Phase == PhaseReady {
			m.mu.RUnlock()
			err := m.TeardownEnvironment(ctx, env.ID, "PR merged")
			m.mu.RLock()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// HandlePRClose handles PR close event for auto-teardown
func (m *Manager) HandlePRClose(ctx context.Context, prNumber int32) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, env := range m.environments {
		if env.PRNumber == prNumber && env.Phase == PhaseReady {
			m.mu.RUnlock()
			err := m.TeardownEnvironment(ctx, env.ID, "PR closed")
			m.mu.RLock()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// cleanupExpiredEnvironments cleans up environments that have expired
func (m *Manager) cleanupExpiredEnvironments(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, env := range m.environments {
		if env.Phase == PhaseReady && now.After(env.ExpiresAt) {
			env.Phase = PhaseTerminating
			err := m.driver.Destroy(ctx, env.Name)
			if err != nil {
				env.TerminationReason = fmt.Sprintf("cleanup failed: %v", err)
			} else {
				env.TerminationReason = "TTL expired"
			}
			env.Phase = PhaseTerminated
			terminatedAt := now
			env.TerminatedAt = &terminatedAt
		}
	}
}

// countActiveEnvironments counts the number of active (non-terminated) environments
func (m *Manager) countActiveEnvironments() int {
	count := 0
	for _, env := range m.environments {
		if env.Phase != PhaseTerminated {
			count++
		}
	}
	return count
}

// generateEnvironmentName generates a unique environment name
func (m *Manager) generateEnvironmentName(spec *kraneTypes.EphemeralEnvironmentSpec) string {
	if spec.PRNumber > 0 {
		return fmt.Sprintf("pr-%d", spec.PRNumber)
	}
	if spec.BranchName != "" {
		return fmt.Sprintf("branch-%s", spec.BranchName)
	}
	return fmt.Sprintf("ephem-%d", time.Now().UnixNano())
}

// generateNamespace generates a namespace name for the ephemeral environment
func (m *Manager) generateNamespace(spec *kraneTypes.EphemeralEnvironmentSpec, envName string) string {
	prefix := spec.NamespacePrefix
	if prefix == "" {
		prefix = "ephem-"
	}
	return prefix + envName
}
