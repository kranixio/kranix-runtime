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
}

type RuntimeConfig struct {
	DefaultBackend string `yaml:"default_backend"`
}

type DockerConfig struct {
	Host       string `yaml:"host"`
	APIVersion string `yaml:"api_version"`
}

type KubernetesConfig struct {
	Kubeconfig        string `yaml:"kubeconfig"`
	Context           string `yaml:"context"`
	DefaultNamespace  string `yaml:"default_namespace"`
}

type PodmanConfig struct {
	Socket string `yaml:"socket"`
}

type RemoteConfig struct {
	SSHKeyPath     string `yaml:"ssh_key_path"`
	KnownHostsPath string `yaml:"known_hosts_path"`
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
	}
}
