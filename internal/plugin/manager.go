package plugin

import (
	"fmt"
	"sync"

	"github.com/kranix-io/kranix-packages/types"
	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/registry"
)

// Descriptor registers a runtime backend without forking kranix-runtime.
type Descriptor struct {
	Name        string
	Version     string
	Description string
	Builtin     bool
	Factory     registry.DriverFactory
}

// Manager tracks runtime backend plugins and syncs them to the driver registry.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Descriptor
	enabled map[string]bool
}

func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Descriptor),
		enabled: make(map[string]bool),
	}
}

var defaultManager = NewManager()

// Default returns the global plugin manager.
func Default() *Manager { return defaultManager }

// Register adds a custom backend plugin. Call from init() in external packages.
func (m *Manager) Register(desc Descriptor) {
	if desc.Name == "" || desc.Factory == nil {
		panic("plugin: name and factory are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.plugins[desc.Name]; exists {
		panic(fmt.Sprintf("plugin %q already registered", desc.Name))
	}
	m.plugins[desc.Name] = desc
	m.enabled[desc.Name] = true
}

// LoadFromConfig enables plugins listed in runtime config.
func (m *Manager) LoadFromConfig(cfg *config.Config) error {
	if cfg == nil || !cfg.Plugins.Enabled {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range cfg.Plugins.Allow {
		if !p.Enabled {
			m.enabled[p.Name] = false
			continue
		}
		desc, ok := m.plugins[p.Name]
		if !ok {
			return fmt.Errorf("plugin %q enabled in config but not registered", p.Name)
		}
		m.enabled[p.Name] = true
		if err := m.registerDriverLocked(desc); err != nil {
			return err
		}
	}
	return nil
}

// ActivateAll registers all enabled plugins with the global driver registry.
func (m *Manager) ActivateAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for name, desc := range m.plugins {
		if !m.enabled[name] {
			continue
		}
		if err := m.registerDriverLocked(desc); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) registerDriverLocked(desc Descriptor) error {
	if desc.Factory != nil {
		registry.RegisterIfAbsent(desc.Name, desc.Factory)
	}
	return nil
}

// List returns metadata for all known plugins.
func (m *Manager) List() []types.RuntimePluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]types.RuntimePluginInfo, 0, len(m.plugins))
	for name, desc := range m.plugins {
		out = append(out, types.RuntimePluginInfo{
			Name:        name,
			Version:     desc.Version,
			Description: desc.Description,
			Builtin:     desc.Builtin,
			Enabled:     m.enabled[name],
		})
	}
	return out
}

// RegisterBuiltins records built-in drivers as plugins.
func (m *Manager) RegisterBuiltins() {
	builtins := []Descriptor{
		{Name: "docker", Version: "1.0", Description: "Docker Engine API driver", Builtin: true},
		{Name: "kubernetes", Version: "1.0", Description: "Kubernetes API driver", Builtin: true},
		{Name: "podman", Version: "1.0", Description: "Podman CLI driver", Builtin: true},
		{Name: "compose", Version: "1.0", Description: "Docker Compose driver", Builtin: true},
		{Name: "remote", Version: "1.0", Description: "Remote SSH driver", Builtin: true},
	}
	for _, b := range builtins {
		m.mu.Lock()
		m.plugins[b.Name] = b
		m.enabled[b.Name] = true
		m.mu.Unlock()
	}
}
