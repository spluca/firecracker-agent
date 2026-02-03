package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the agent configuration
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Firecracker FirecrackerConfig `yaml:"firecracker"`
	Network     NetworkConfig     `yaml:"network"`
	Storage     StorageConfig     `yaml:"storage"`
	Monitoring  MonitoringConfig  `yaml:"monitoring"`
	Log         LogConfig         `yaml:"log"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type FirecrackerConfig struct {
	BinaryPath string `yaml:"binary_path"`
	JailerPath string `yaml:"jailer_path"`
	KernelPath string `yaml:"kernel_path"`
	RootfsPath string `yaml:"rootfs_path"`
	// NEW: Jailer configuration (enabled by default for security)
	UseJailer bool `yaml:"use_jailer"`
	JailUID   int  `yaml:"jail_uid"`
	JailGID   int  `yaml:"jail_gid"`
}

type NetworkConfig struct {
	BridgeName string `yaml:"bridge_name"`
	TapPrefix  string `yaml:"tap_prefix"`
}

type StorageConfig struct {
	VMsDir     string `yaml:"vms_dir"`
	UseOverlay bool   `yaml:"use_overlay"`
}

type MonitoringConfig struct {
	Enabled     bool `yaml:"enabled"`
	MetricsPort int  `yaml:"metrics_port"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load reads config from file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 50051
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
	if cfg.Network.BridgeName == "" {
		cfg.Network.BridgeName = "br0"
	}
	if cfg.Network.TapPrefix == "" {
		cfg.Network.TapPrefix = "vmtap"
	}
	if cfg.Storage.VMsDir == "" {
		cfg.Storage.VMsDir = "/srv/firecracker/vms"
	}
	if cfg.Monitoring.MetricsPort == 0 {
		cfg.Monitoring.MetricsPort = 9090
	}
	// NEW: Default jailer configuration - enabled by default for security
	cfg.Firecracker.UseJailer = true // Default to using jailer
	if cfg.Firecracker.JailUID == 0 {
		cfg.Firecracker.JailUID = 1000 // Default to non-privileged user
	}
	if cfg.Firecracker.JailGID == 0 {
		cfg.Firecracker.JailGID = 1000 // Default to non-privileged group
	}

	return &cfg, nil
}
