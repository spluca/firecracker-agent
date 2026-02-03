package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client is an HTTP client for Firecracker API over Unix socket
type Client struct {
	socketPath string
	httpClient *http.Client
}

// NewClient creates a new Firecracker API client
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// BootSource represents the boot source configuration
type BootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args,omitempty"`
}

// Drive represents a block device configuration
type Drive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

// MachineConfig represents machine configuration
type MachineConfig struct {
	VcpuCount  int32 `json:"vcpu_count"`
	MemSizeMib int32 `json:"mem_size_mib"`
	HtEnabled  bool  `json:"ht_enabled"`
}

// NetworkInterface represents a network interface configuration
type NetworkInterface struct {
	IfaceID     string `json:"iface_id"`
	HostDevName string `json:"host_dev_name"`
	GuestMAC    string `json:"guest_mac,omitempty"`
}

// InstanceActionInfo represents an action to perform on the VM
type InstanceActionInfo struct {
	ActionType string `json:"action_type"` // "FlushMetrics", "InstanceStart", "SendCtrlAltDel"
}

// SetBootSource configures the boot source
func (c *Client) SetBootSource(ctx context.Context, bootSource BootSource) error {
	return c.put(ctx, "/boot-source", bootSource)
}

// SetMachineConfig configures the machine (vCPU and memory)
func (c *Client) SetMachineConfig(ctx context.Context, machineConfig MachineConfig) error {
	return c.put(ctx, "/machine-config", machineConfig)
}

// AddDrive adds a block device
func (c *Client) AddDrive(ctx context.Context, drive Drive) error {
	return c.put(ctx, fmt.Sprintf("/drives/%s", drive.DriveID), drive)
}

// AddNetworkInterface adds a network interface
func (c *Client) AddNetworkInterface(ctx context.Context, iface NetworkInterface) error {
	return c.put(ctx, fmt.Sprintf("/network-interfaces/%s", iface.IfaceID), iface)
}

// StartInstance starts the VM
func (c *Client) StartInstance(ctx context.Context) error {
	action := InstanceActionInfo{ActionType: "InstanceStart"}
	return c.put(ctx, "/actions", action)
}

// SendCtrlAltDel sends Ctrl+Alt+Del to the VM
func (c *Client) SendCtrlAltDel(ctx context.Context) error {
	action := InstanceActionInfo{ActionType: "SendCtrlAltDel"}
	return c.put(ctx, "/actions", action)
}

// GetInstanceInfo retrieves VM instance information
func (c *Client) GetInstanceInfo(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.get(ctx, "/")
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// put sends a PUT request to the Firecracker API
func (c *Client) put(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// get sends a GET request to the Firecracker API
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
