package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `
server:
  host: "127.0.0.1"
  port: 50051

firecracker:
  binary_path: "/usr/bin/firecracker"
  jailer_path: "/usr/bin/jailer"
  kernel_path: "/var/lib/firecracker/vmlinux"
  rootfs_path: "/var/lib/firecracker/rootfs.ext4"

network:
  bridge_name: "testbr0"
  tap_prefix: "testtap"

storage:
  vms_dir: "/tmp/vms"
  use_overlay: true

monitoring:
  enabled: true
  metrics_port: 9090

log:
  level: "debug"
  format: "text"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config
	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Assert values
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 50051, cfg.Server.Port)
	assert.Equal(t, "/usr/bin/firecracker", cfg.Firecracker.BinaryPath)
	assert.Equal(t, "/usr/bin/jailer", cfg.Firecracker.JailerPath)
	assert.Equal(t, "/var/lib/firecracker/vmlinux", cfg.Firecracker.KernelPath)
	assert.Equal(t, "/var/lib/firecracker/rootfs.ext4", cfg.Firecracker.RootfsPath)
	assert.Equal(t, "testbr0", cfg.Network.BridgeName)
	assert.Equal(t, "testtap", cfg.Network.TapPrefix)
	assert.Equal(t, "/tmp/vms", cfg.Storage.VMsDir)
	assert.True(t, cfg.Storage.UseOverlay)
	assert.True(t, cfg.Monitoring.Enabled)
	assert.Equal(t, 9090, cfg.Monitoring.MetricsPort)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
}

func TestLoad_MinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal-config.yaml")

	// Minimal config - should use defaults
	configContent := `
firecracker:
  binary_path: "/usr/bin/firecracker"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Check defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 50051, cfg.Server.Port)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
	assert.Equal(t, "br0", cfg.Network.BridgeName)
	assert.Equal(t, "vmtap", cfg.Network.TapPrefix)
	assert.Equal(t, "/srv/firecracker/vms", cfg.Storage.VMsDir)
	assert.Equal(t, 9090, cfg.Monitoring.MetricsPort)
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid-config.yaml")

	invalidContent := `
server:
  host: "127.0.0.1"
  port: invalid_port
  bad_indentation
`

	err := os.WriteFile(configPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	_, err = Load(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config")
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoad_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty-config.yaml")

	err := os.WriteFile(configPath, []byte(""), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Should have defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 50051, cfg.Server.Port)
}

func TestLoad_PartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial-config.yaml")

	configContent := `
server:
  port: 9999

network:
  bridge_name: "custombr"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(configPath)
	require.NoError(t, err)

	// Custom values
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "custombr", cfg.Network.BridgeName)

	// Defaults for missing values
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, "vmtap", cfg.Network.TapPrefix)
	assert.Equal(t, "info", cfg.Log.Level)
}
