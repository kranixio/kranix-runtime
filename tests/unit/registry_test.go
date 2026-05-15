package unit

import (
	"testing"

	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/registry"
)

func TestRegistryList(t *testing.T) {
	drivers := registry.List()
	if len(drivers) == 0 {
		t.Error("expected at least one registered driver")
	}

	expectedDrivers := []string{"docker", "kubernetes", "podman", "compose", "remote"}
	for _, expected := range expectedDrivers {
		found := false
		for _, driver := range drivers {
			if driver == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected driver %s to be registered", expected)
		}
	}
}

func TestRegistryGet(t *testing.T) {
	cfg := config.DefaultConfig()

	// Test getting docker driver (should not require actual daemon for registry test)
	_, err := registry.Get("docker", cfg)
	if err != nil {
		t.Logf("Docker driver creation failed (expected if no daemon): %v", err)
	}

	// Test unknown driver
	_, err = registry.Get("unknown", cfg)
	if err == nil {
		t.Error("expected error for unknown driver")
	}
}
