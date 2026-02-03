package network

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// Manager handles network configuration for VMs
type Manager struct {
	bridgeName string
	bridgeIP   string
	tapPrefix  string
	log        *logrus.Logger
}

// NewManager creates a new network manager
func NewManager(bridgeName, bridgeIP, tapPrefix string, log *logrus.Logger) *Manager {
	return &Manager{
		bridgeName: bridgeName,
		bridgeIP:   bridgeIP,
		tapPrefix:  tapPrefix,
		log:        log,
	}
}

// CreateTAPDevice creates a TAP device for a VM
func (m *Manager) CreateTAPDevice(vmID string) (string, error) {
	tapName := fmt.Sprintf("%s-%s", m.tapPrefix, vmID[:8])

	m.log.WithField("tap_device", tapName).Info("Creating TAP device")

	// Create TAP device
	cmd := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to create TAP device: %w (output: %s)", err, string(output))
	}

	// Bring up the TAP device
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		m.DeleteTAPDevice(tapName) // Cleanup on error
		return "", fmt.Errorf("failed to bring up TAP device: %w (output: %s)", err, string(output))
	}

	// Add TAP device to bridge
	cmd = exec.Command("ip", "link", "set", tapName, "master", m.bridgeName)
	if output, err := cmd.CombinedOutput(); err != nil {
		m.DeleteTAPDevice(tapName) // Cleanup on error
		return "", fmt.Errorf("failed to add TAP to bridge: %w (output: %s)", err, string(output))
	}

	m.log.WithFields(logrus.Fields{
		"tap_device": tapName,
		"bridge":     m.bridgeName,
	}).Info("TAP device created and attached to bridge")

	return tapName, nil
}

// DeleteTAPDevice removes a TAP device
func (m *Manager) DeleteTAPDevice(tapName string) error {
	m.log.WithField("tap_device", tapName).Info("Deleting TAP device")

	cmd := exec.Command("ip", "link", "delete", tapName)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if device doesn't exist (not an error)
		if strings.Contains(string(output), "Cannot find device") {
			m.log.WithField("tap_device", tapName).Warn("TAP device not found, already deleted")
			return nil
		}
		return fmt.Errorf("failed to delete TAP device: %w (output: %s)", err, string(output))
	}

	m.log.WithField("tap_device", tapName).Info("TAP device deleted")
	return nil
}

// EnsureBridgeExists checks if bridge exists and creates it if needed
func (m *Manager) EnsureBridgeExists() error {
	m.log.WithField("bridge", m.bridgeName).Info("Ensuring bridge exists")

	// Check if bridge exists
	cmd := exec.Command("ip", "link", "show", m.bridgeName)
	if err := cmd.Run(); err != nil {
		// Create bridge
		m.log.WithField("bridge", m.bridgeName).Info("Creating bridge")
		cmd = exec.Command("ip", "link", "add", "name", m.bridgeName, "type", "bridge")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create bridge: %w (output: %s)", err, string(output))
		}
	} else {
		m.log.WithField("bridge", m.bridgeName).Info("Bridge already exists")
	}

	// Bring up bridge
	cmd = exec.Command("ip", "link", "set", m.bridgeName, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w (output: %s)", err, string(output))
	}

	// Assign IP address if configured
	if m.bridgeIP != "" {
		// Check if IP is already assigned
		cmd = exec.Command("ip", "addr", "show", m.bridgeName)
		output, err := cmd.CombinedOutput()
		if err == nil && !strings.Contains(string(output), m.bridgeIP) {
			m.log.WithFields(logrus.Fields{
				"bridge": m.bridgeName,
				"ip":     m.bridgeIP,
			}).Info("Assigning IP to bridge")

			cmd = exec.Command("ip", "addr", "add", m.bridgeIP, "dev", m.bridgeName)
			if output, err := cmd.CombinedOutput(); err != nil {
				// Ignore "File exists" error which means IP is already assigned but maybe not detected by string match
				if !strings.Contains(string(output), "File exists") {
					return fmt.Errorf("failed to assign IP to bridge: %w (output: %s)", err, string(output))
				}
			}
		}
	}

	m.log.WithField("bridge", m.bridgeName).Info("Bridge created/configured successfully")
	return nil
}

// GenerateMAC generates a MAC address for a VM
func (m *Manager) GenerateMAC(vmID string) string {
	// Generate MAC address from VM ID (simple deterministic approach)
	// Format: 02:XX:XX:XX:XX:XX (locally administered unicast)

	// Take first 10 chars of vmID and convert to hex bytes
	mac := "02:FC"
	for i := 0; i < 4; i++ {
		if i*2+1 < len(vmID) {
			mac += fmt.Sprintf(":%02x", vmID[i*2]^vmID[i*2+1])
		} else {
			mac += ":00"
		}
	}

	return mac
}

// ConfigureIPTables configures iptables rules for NAT (optional)
func (m *Manager) ConfigureIPTables(tapName, vmIP string) error {
	m.log.WithFields(logrus.Fields{
		"tap_device": tapName,
		"vm_ip":      vmIP,
	}).Info("Configuring iptables rules")

	// Enable IP forwarding (if not already enabled)
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if _, err := cmd.CombinedOutput(); err != nil {
		m.log.WithError(err).Warn("Failed to enable IP forwarding")
	}

	// Add MASQUERADE rule for NAT
	cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "eth0", "-j", "MASQUERADE")
	if output, err := cmd.CombinedOutput(); err != nil {
		m.log.WithError(err).WithField("output", string(output)).Warn("Failed to add iptables rule (may already exist)")
	}

	return nil
}

// RemoveIPTablesRules removes iptables rules for a VM
func (m *Manager) RemoveIPTablesRules(tapName, vmIP string) error {
	m.log.WithFields(logrus.Fields{
		"tap_device": tapName,
		"vm_ip":      vmIP,
	}).Info("Removing iptables rules")

	// Remove specific rules if needed
	// For simplicity, we're using general MASQUERADE which is shared

	return nil
}
