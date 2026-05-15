//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/registry"
)

func TestKubernetesDriver(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set, skipping Kubernetes integration test")
	}

	cfg := &config.Config{
		Kubernetes: config.KubernetesConfig{
			Kubeconfig:       kubeconfig,
			DefaultNamespace: "kranix-test",
		},
	}

	driver, err := registry.Get("kubernetes", cfg)
	if err != nil {
		t.Fatalf("failed to get kubernetes driver: %v", err)
	}

	ctx := context.Background()

	// Test Ping
	if err := driver.Ping(ctx); err != nil {
		t.Fatalf("driver ping failed: %v", err)
	}

	// Test Deploy
	spec := &types.WorkloadSpec{
		Name:      "test-nginx",
		Image:     "nginx:alpine",
		Namespace: "kranix-test",
		Replicas:  1,
		Env:       map[string]string{"TEST": "value"},
	}

	status, err := driver.Deploy(ctx, spec)
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
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
