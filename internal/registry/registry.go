package registry

import (
	"fmt"
	"sync"

	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/compose"
	"github.com/kranix-io/kranix-runtime/internal/docker"
	"github.com/kranix-io/kranix-runtime/internal/kubernetes"
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

func Get(name string, cfg *config.Config) (types.RuntimeDriver, error) {
	mu.RLock()
	factory, exists := drivers[name]
	mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("driver %q not found", name)
	}

	return factory(cfg)
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
