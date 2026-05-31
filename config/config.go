package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Runtime    RuntimeConfig    `yaml:"runtime"`
	Docker     DockerConfig     `yaml:"docker"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	Podman     PodmanConfig     `yaml:"podman"`
	Remote     RemoteConfig     `yaml:"remote"`
	GPU        GPUConfig        `yaml:"gpu"`
	Ephemeral  EphemeralConfig  `yaml:"ephemeral"`
	EdgeAgent  EdgeAgentConfig  `yaml:"edge_agent"`
	ImageCache ImageCacheConfig `yaml:"image_cache"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Plugins    PluginsConfig    `yaml:"plugins"`
	Checkpoint CheckpointConfig `yaml:"checkpoint"`
	Bandwidth  BandwidthConfig  `yaml:"bandwidth"`
	Volumes    VolumesConfig    `yaml:"volumes"`
	NodeOps    NodeOpsConfig    `yaml:"node_ops"`
}

type PluginsConfig struct {
	Enabled bool           `yaml:"enabled"`
	Allow   []PluginConfig `yaml:"allow"`
}

type PluginConfig struct {
	Name        string `yaml:"name"`
	Module      string `yaml:"module"`
	Description string `yaml:"description"`
	Enabled     bool   `yaml:"enabled"`
}

type CheckpointConfig struct {
	Enabled bool `yaml:"enabled"`
}

type BandwidthConfig struct {
	Enabled           bool   `yaml:"enabled"`
	DefaultEgressMbit string `yaml:"default_egress_mbit"`
}

type VolumesConfig struct {
	Enabled              bool   `yaml:"enabled"`
	DefaultStorageClass  string `yaml:"default_storage_class"`
	DefaultSize          string `yaml:"default_size"`
	AutoCleanupOnDestroy bool   `yaml:"auto_cleanup_on_destroy"`
}

type NodeOpsConfig struct {
	HealthScoring struct {
		Enabled       bool   `yaml:"enabled"`
		LatencyWindow string `yaml:"latency_window"`
	} `yaml:"health_scoring"`
	Drain struct {
		Enabled                   bool `yaml:"enabled"`
		DefaultGracePeriodSeconds int  `yaml:"default_grace_period_seconds"`
		IgnoreDaemonSets          bool `yaml:"ignore_daemonsets"`
	} `yaml:"drain"`
	MultiArch struct {
		Enabled     bool   `yaml:"enabled"`
		DefaultArch string `yaml:"default_arch"`
	} `yaml:"multi_arch"`
}

type RuntimeConfig struct {
	DefaultBackend string `yaml:"default_backend"`
}

type DockerConfig struct {
	Host       string `yaml:"host"`
	APIVersion string `yaml:"api_version"`
}

type KubernetesConfig struct {
	Kubeconfig       string `yaml:"kubeconfig"`
	Context          string `yaml:"context"`
	DefaultNamespace string `yaml:"default_namespace"`
}

type PodmanConfig struct {
	Socket string `yaml:"socket"`
}

type RemoteConfig struct {
	SSHKeyPath     string `yaml:"ssh_key_path"`
	KnownHostsPath string `yaml:"known_hosts_path"`
}

type GPUConfig struct {
	Enabled          bool   `yaml:"enabled"`
	DefaultVendor    string `yaml:"default_vendor"` // nvidia | amd
	NvidiaDevicePath string `yaml:"nvidia_device_path,omitempty"`
	AMDDevicePath    string `yaml:"amd_device_path,omitempty"`
}

type EphemeralConfig struct {
	Enabled         bool   `yaml:"enabled"`
	DefaultTTL      string `yaml:"default_ttl"` // e.g., "2h"
	MaxEnvironments int32  `yaml:"max_environments"`
	NamespacePrefix string `yaml:"namespace_prefix"`
	AutoTeardown    bool   `yaml:"auto_teardown"`
	TeardownOnMerge bool   `yaml:"teardown_on_merge"`
	TeardownOnClose bool   `yaml:"teardown_on_close"`
	CleanupInterval string `yaml:"cleanup_interval"` // e.g., "5m"
}

type EdgeAgentConfig struct {
	Enabled           bool   `yaml:"enabled"`
	NodeID            string `yaml:"node_id,omitempty"`
	NodeName          string `yaml:"node_name,omitempty"`
	IPAddress         string `yaml:"ip_address,omitempty"`
	Port              int32  `yaml:"port"`
	HeartbeatInterval string `yaml:"heartbeat_interval"` // e.g., "30s"
	AuthToken         string `yaml:"auth_token,omitempty"`
}

type ImageCacheConfig struct {
	Enabled         bool     `yaml:"enabled"`
	CacheSizeGB     int32    `yaml:"cache_size_gb"`
	MaxCachedImages int32    `yaml:"max_cached_images"`
	TTL             string   `yaml:"ttl"` // e.g., "168h"
	PrepullImages   []string `yaml:"prepull_images"`
	RegistryMirrors []string `yaml:"registry_mirrors"`
}

type MetricsConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CollectionInterval string `yaml:"collection_interval"` // e.g., "30s"`
	RetentionPeriod    string `yaml:"retention_period"`    // e.g., "24h"
	ExposeEndpoint     bool   `yaml:"expose_endpoint"`
	MetricsPort        int32  `yaml:"metrics_port"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Runtime: RuntimeConfig{
			DefaultBackend: "kubernetes",
		},
		Docker: DockerConfig{
			Host:       "unix:///var/run/docker.sock",
			APIVersion: "1.45",
		},
		Kubernetes: KubernetesConfig{
			Kubeconfig:       "",
			Context:          "",
			DefaultNamespace: "default",
		},
		Podman: PodmanConfig{
			Socket: "unix:///run/user/1000/podman/podman.sock",
		},
		Remote: RemoteConfig{
			SSHKeyPath:     "~/.ssh/id_rsa",
			KnownHostsPath: "~/.ssh/known_hosts",
		},
		GPU: GPUConfig{
			Enabled:          false,
			DefaultVendor:    "nvidia",
			NvidiaDevicePath: "/dev/nvidia0",
			AMDDevicePath:    "/dev/kfd",
		},
		Ephemeral: EphemeralConfig{
			Enabled:         false,
			DefaultTTL:      "2h",
			MaxEnvironments: 10,
			NamespacePrefix: "ephem-",
			AutoTeardown:    true,
			TeardownOnMerge: true,
			TeardownOnClose: true,
			CleanupInterval: "5m",
		},
		EdgeAgent: EdgeAgentConfig{
			Enabled:           false,
			Port:              50052,
			HeartbeatInterval: "30s",
		},
		ImageCache: ImageCacheConfig{
			Enabled:         false,
			CacheSizeGB:     100,
			MaxCachedImages: 50,
			TTL:             "168h",
			PrepullImages:   []string{},
			RegistryMirrors: []string{},
		},
		Metrics: MetricsConfig{
			Enabled:            true,
			CollectionInterval: "30s",
			RetentionPeriod:    "24h",
			ExposeEndpoint:     true,
			MetricsPort:        9090,
		},
	}
}
