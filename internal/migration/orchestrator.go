package migration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
)

const shadowSuffix = "-migrating"

// DriverResolver loads a runtime driver by backend name.
type DriverResolver func(name string) (types.RuntimeDriver, error)

// Orchestrator performs zero-downtime workload migrations between runtime backends.
type Orchestrator struct {
	cfg      *config.Config
	resolve  DriverResolver
}

func NewOrchestrator(cfg *config.Config, resolve DriverResolver) *Orchestrator {
	return &Orchestrator{cfg: cfg, resolve: resolve}
}

func (o *Orchestrator) MigrateWorkload(ctx context.Context, req types.WorkloadMigrationRequest) (*types.WorkloadMigrationResult, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	sourceBackend := req.SourceBackend
	if sourceBackend == "" {
		sourceBackend = o.cfg.Runtime.DefaultBackend
	}
	if req.TargetBackend == sourceBackend {
		return nil, fmt.Errorf("target backend %q must differ from source backend %q", req.TargetBackend, sourceBackend)
	}

	sourceDriver, err := o.resolve(sourceBackend)
	if err != nil {
		return nil, fmt.Errorf("source driver: %w", err)
	}
	targetDriver, err := o.resolve(req.TargetBackend)
	if err != nil {
		return nil, fmt.Errorf("target driver: %w", err)
	}

	spec := req.Spec
	if spec == nil {
		status, err := sourceDriver.GetStatus(ctx, req.WorkloadID)
		if err != nil {
			return nil, fmt.Errorf("load workload status: %w", err)
		}
		spec = &types.WorkloadSpec{
			Name:      req.WorkloadID,
			Namespace: firstNonEmpty(req.Namespace, status.Namespace),
			Image:     status.Image,
			Backend:   sourceBackend,
			Replicas:  maxInt(status.Replicas, 1),
		}
	}

	timeout := parseTimeout(req.ReadyTimeout, o.cfg.Migration.ReadyTimeout)
	shadowName := req.WorkloadID + shadowSuffix

	if req.ZeroDowntime {
		shadowSpec := cloneSpec(spec)
		shadowSpec.Name = shadowName
		shadowSpec.Backend = req.TargetBackend
		if _, err := targetDriver.Deploy(ctx, shadowSpec); err != nil {
			return nil, fmt.Errorf("deploy shadow on target: %w", err)
		}
		if err := waitUntilReady(ctx, targetDriver, shadowName, shadowSpec.Replicas, timeout); err != nil {
			_ = targetDriver.Destroy(ctx, shadowName)
			return nil, err
		}
	}

	finalSpec := cloneSpec(spec)
	finalSpec.Name = req.WorkloadID
	finalSpec.Backend = req.TargetBackend
	if _, err := targetDriver.Deploy(ctx, finalSpec); err != nil {
		if req.ZeroDowntime {
			_ = targetDriver.Destroy(ctx, shadowName)
		}
		return nil, fmt.Errorf("deploy on target: %w", err)
	}
	if err := waitUntilReady(ctx, targetDriver, req.WorkloadID, finalSpec.Replicas, timeout); err != nil {
		_ = targetDriver.Destroy(ctx, req.WorkloadID)
		if req.ZeroDowntime {
			_ = targetDriver.Destroy(ctx, shadowName)
		}
		return nil, err
	}

	if err := sourceDriver.Destroy(ctx, req.WorkloadID); err != nil {
		return nil, fmt.Errorf("destroy source workload: %w", err)
	}
	if req.ZeroDowntime {
		_ = targetDriver.Destroy(ctx, shadowName)
	}

	return &types.WorkloadMigrationResult{
		WorkloadID:    req.WorkloadID,
		SourceBackend: sourceBackend,
		TargetBackend: req.TargetBackend,
		State:         "completed",
		CutoverAt:     time.Now().UTC(),
		Message:       "workload migrated with zero downtime cutover",
	}, nil
}

func validateRequest(req types.WorkloadMigrationRequest) error {
	if strings.TrimSpace(req.WorkloadID) == "" {
		return fmt.Errorf("workloadId is required")
	}
	if strings.TrimSpace(req.TargetBackend) == "" {
		return fmt.Errorf("targetBackend is required")
	}
	return nil
}

func waitUntilReady(ctx context.Context, driver types.RuntimeDriver, name string, replicas int, timeout time.Duration) error {
	if replicas <= 0 {
		replicas = 1
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s to become ready", name)
		}
		status, err := driver.GetStatus(ctx, name)
		if err != nil {
			return err
		}
		ready := status.ReadyReplicas
		if ready == 0 {
			ready = status.Ready
		}
		if ready >= replicas || strings.EqualFold(status.State, "Running") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func cloneSpec(spec *types.WorkloadSpec) *types.WorkloadSpec {
	if spec == nil {
		return &types.WorkloadSpec{}
	}
	copy := *spec
	if spec.Env != nil {
		copy.Env = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			copy.Env[k] = v
		}
	}
	return &copy
}

func parseTimeout(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if fallback <= 0 {
			return 5 * time.Minute
		}
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ types.RuntimeMigrationOperations = (*Orchestrator)(nil)
