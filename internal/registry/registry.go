package registry

import (
	"fmt"
	"sync"

	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/compose"
	"github.com/kranix-io/kranix-runtime/internal/docker"
	"github.com/kranix-io/kranix-runtime/internal/kubernetes"
	"github.com/kranix-io/kranix-runtime/internal/migration"
	"github.com/kranix-io/kranix-runtime/internal/podman"
	"github.com/kranix-io/kranix-runtime/internal/remote"
	"github.com/kranix-io/kranix-packages/types"
)

var (
	mu      sync.RWMutex
	drivers = make(map[string]DriverFactory)
)

type DriverFactory func(cfg *config.Config) (types.RuntimeDriver, error)

func Register(name string, factory DriverFactory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := drivers[name]; exists {
		panic(fmt.Sprintf("driver %q already registered", name))
	}
	drivers[name] = factory
}

// RegisterIfAbsent adds a driver factory only when the name is not already registered.
func RegisterIfAbsent(name string, factory DriverFactory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := drivers[name]; exists {
		return
	}
	drivers[name] = factory
}

func Get(name string, cfg *config.Config) (types.RuntimeDriver, error) {
	mu.RLock()
	factory, exists := drivers[name]
	mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("driver %q not found", name)
	}

	return factory(cfg)
}

// GetNodeOperations returns node lifecycle APIs when the driver supports them.
func GetNodeOperations(name string, cfg *config.Config) (types.NodeOperations, error) {
	driver, err := Get(name, cfg)
	if err != nil {
		return nil, err
	}
	if ops, ok := driver.(types.NodeOperations); ok {
		return ops, nil
	}
	return nil, fmt.Errorf("driver %q does not support node operations", name)
}

// GetExtendedOperations returns checkpoint/volume/bandwidth APIs when supported.
func GetExtendedOperations(name string, cfg *config.Config) (types.RuntimeExtendedOperations, error) {
	driver, err := Get(name, cfg)
	if err != nil {
		return nil, err
	}
	if ops, ok := driver.(types.RuntimeExtendedOperations); ok {
		return ops, nil
	}
	return nil, fmt.Errorf("driver %q does not support extended operations", name)
}

// GetMigrationOperations returns the cross-backend migration orchestrator.
func GetMigrationOperations(cfg *config.Config) types.RuntimeMigrationOperations {
	return migration.NewOrchestrator(cfg, func(name string) (types.RuntimeDriver, error) {
		return Get(name, cfg)
	})
}

func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(drivers))
	for name := range drivers {
		names = append(names, name)
	}
	return names
}

func init() {
	// Register all built-in drivers
	Register("docker", docker.New)
	Register("kubernetes", kubernetes.New)
	Register("podman", podman.New)
	Register("compose", compose.New)
	Register("remote", remote.New)
}
