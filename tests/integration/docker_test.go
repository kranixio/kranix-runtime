//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/registry"
)

func TestDockerDriver(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	cfg := config.DefaultConfig()
	driver, err := registry.Get("docker", cfg)
	if err != nil {
		t.Fatalf("failed to get docker driver: %v", err)
	}

	ctx := context.Background()

	// Test Ping
	if err := driver.Ping(ctx); err != nil {
		t.Fatalf("driver ping failed: %v", err)
	}

	// Test Deploy
	spec := &types.WorkloadSpec{
		Name:  "test-nginx",
		Image: "nginx:alpine",
		Env:   map[string]string{"TEST": "value"},
	}

	status, err := driver.Deploy(ctx, spec)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	if status.State != "Running" {
		t.Logf("workload state: %s (may be starting)", status.State)
	}

	// Test GetStatus
	status, err = driver.GetStatus(ctx, spec.Name)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}

	// Test Destroy
	if err := driver.Destroy(ctx, spec.Name); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
}
