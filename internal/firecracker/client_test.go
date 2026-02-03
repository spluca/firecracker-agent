package firecracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUnixServer creates a test HTTP server listening on a Unix socket
func mockUnixServer(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()

	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "firecracker.sock")

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err, "Failed to create Unix socket listener")

	server := &http.Server{Handler: handler}

	go func() {
		_ = server.Serve(listener)
	}()

	cleanup := func() {
		_ = server.Close()
		_ = os.Remove(socketPath)
	}

	// Wait for server to be ready
	time.Sleep(10 * time.Millisecond)

	return socketPath, cleanup
}

func TestNewClient(t *testing.T) {
	socketPath := "/var/run/firecracker.sock"
	client := NewClient(socketPath)

	assert.NotNil(t, client)
	assert.Equal(t, socketPath, client.socketPath)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
}

func TestClient_SetBootSource(t *testing.T) {
	tests := []struct {
		name           string
		bootSource     BootSource
		mockStatusCode int
		mockResponse   string
		expectError    bool
		errorContains  string
	}{
		{
			name: "successful boot source configuration",
			bootSource: BootSource{
				KernelImagePath: "/path/to/kernel",
				BootArgs:        "console=ttyS0 reboot=k panic=1",
			},
			mockStatusCode: http.StatusNoContent,
			mockResponse:   "",
			expectError:    false,
		},
		{
			name: "boot source with minimal config",
			bootSource: BootSource{
				KernelImagePath: "/path/to/kernel",
			},
			mockStatusCode: http.StatusNoContent,
			mockResponse:   "",
			expectError:    false,
		},
		{
			name: "API returns error",
			bootSource: BootSource{
				KernelImagePath: "/invalid/path",
			},
			mockStatusCode: http.StatusBadRequest,
			mockResponse:   `{"fault_message": "Invalid kernel path"}`,
			expectError:    true,
			errorContains:  "400",
		},
		{
			name: "API returns internal error",
			bootSource: BootSource{
				KernelImagePath: "/path/to/kernel",
			},
			mockStatusCode: http.StatusInternalServerError,
			mockResponse:   `{"fault_message": "Internal error"}`,
			expectError:    true,
			errorContains:  "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/boot-source", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify request body
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var receivedBootSource BootSource
				err = json.Unmarshal(body, &receivedBootSource)
				require.NoError(t, err)
				assert.Equal(t, tt.bootSource.KernelImagePath, receivedBootSource.KernelImagePath)

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != "" {
					_, _ = w.Write([]byte(tt.mockResponse))
				}
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			// Test the client
			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.SetBootSource(ctx, tt.bootSource)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_SetMachineConfig(t *testing.T) {
	tests := []struct {
		name           string
		machineConfig  MachineConfig
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful machine configuration",
			machineConfig: MachineConfig{
				VcpuCount:  2,
				MemSizeMib: 512,
				HtEnabled:  false,
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "large machine configuration",
			machineConfig: MachineConfig{
				VcpuCount:  8,
				MemSizeMib: 4096,
				HtEnabled:  true,
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "API returns error",
			machineConfig: MachineConfig{
				VcpuCount:  0,
				MemSizeMib: 0,
			},
			mockStatusCode: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/machine-config", r.URL.Path)

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var receivedConfig MachineConfig
				err = json.Unmarshal(body, &receivedConfig)
				require.NoError(t, err)
				assert.Equal(t, tt.machineConfig.VcpuCount, receivedConfig.VcpuCount)
				assert.Equal(t, tt.machineConfig.MemSizeMib, receivedConfig.MemSizeMib)
				assert.Equal(t, tt.machineConfig.HtEnabled, receivedConfig.HtEnabled)

				w.WriteHeader(tt.mockStatusCode)
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.SetMachineConfig(ctx, tt.machineConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_AddDrive(t *testing.T) {
	tests := []struct {
		name           string
		drive          Drive
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful root drive addition",
			drive: Drive{
				DriveID:      "rootfs",
				PathOnHost:   "/path/to/rootfs.ext4",
				IsRootDevice: true,
				IsReadOnly:   false,
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "successful read-only drive addition",
			drive: Drive{
				DriveID:      "data",
				PathOnHost:   "/path/to/data.ext4",
				IsRootDevice: false,
				IsReadOnly:   true,
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "API returns error",
			drive: Drive{
				DriveID:    "invalid",
				PathOnHost: "/nonexistent/path",
			},
			mockStatusCode: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				expectedPath := fmt.Sprintf("/drives/%s", tt.drive.DriveID)
				assert.Equal(t, expectedPath, r.URL.Path)

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var receivedDrive Drive
				err = json.Unmarshal(body, &receivedDrive)
				require.NoError(t, err)
				assert.Equal(t, tt.drive.DriveID, receivedDrive.DriveID)
				assert.Equal(t, tt.drive.PathOnHost, receivedDrive.PathOnHost)
				assert.Equal(t, tt.drive.IsRootDevice, receivedDrive.IsRootDevice)
				assert.Equal(t, tt.drive.IsReadOnly, receivedDrive.IsReadOnly)

				w.WriteHeader(tt.mockStatusCode)
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.AddDrive(ctx, tt.drive)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_AddNetworkInterface(t *testing.T) {
	tests := []struct {
		name           string
		iface          NetworkInterface
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful network interface with MAC",
			iface: NetworkInterface{
				IfaceID:     "eth0",
				HostDevName: "tap0",
				GuestMAC:    "AA:FC:00:00:00:01",
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "successful network interface without MAC",
			iface: NetworkInterface{
				IfaceID:     "eth0",
				HostDevName: "tap0",
			},
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "API returns error",
			iface: NetworkInterface{
				IfaceID:     "eth0",
				HostDevName: "invalid-tap",
			},
			mockStatusCode: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				expectedPath := fmt.Sprintf("/network-interfaces/%s", tt.iface.IfaceID)
				assert.Equal(t, expectedPath, r.URL.Path)

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var receivedIface NetworkInterface
				err = json.Unmarshal(body, &receivedIface)
				require.NoError(t, err)
				assert.Equal(t, tt.iface.IfaceID, receivedIface.IfaceID)
				assert.Equal(t, tt.iface.HostDevName, receivedIface.HostDevName)
				assert.Equal(t, tt.iface.GuestMAC, receivedIface.GuestMAC)

				w.WriteHeader(tt.mockStatusCode)
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.AddNetworkInterface(ctx, tt.iface)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_StartInstance(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockResponse   string
		expectError    bool
	}{
		{
			name:           "successful instance start",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "instance already running",
			mockStatusCode: http.StatusBadRequest,
			mockResponse:   `{"fault_message": "Instance already started"}`,
			expectError:    true,
		},
		{
			name:           "invalid configuration",
			mockStatusCode: http.StatusBadRequest,
			mockResponse:   `{"fault_message": "Missing boot source"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/actions", r.URL.Path)

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var action InstanceActionInfo
				err = json.Unmarshal(body, &action)
				require.NoError(t, err)
				assert.Equal(t, "InstanceStart", action.ActionType)

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != "" {
					_, _ = w.Write([]byte(tt.mockResponse))
				}
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.StartInstance(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_SendCtrlAltDel(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockResponse   string
		expectError    bool
	}{
		{
			name:           "successful ctrl-alt-del",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "instance not running",
			mockStatusCode: http.StatusBadRequest,
			mockResponse:   `{"fault_message": "Instance not running"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/actions", r.URL.Path)

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var action InstanceActionInfo
				err = json.Unmarshal(body, &action)
				require.NoError(t, err)
				assert.Equal(t, "SendCtrlAltDel", action.ActionType)

				w.WriteHeader(tt.mockStatusCode)
				if tt.mockResponse != "" {
					_, _ = w.Write([]byte(tt.mockResponse))
				}
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.SendCtrlAltDel(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_GetInstanceInfo(t *testing.T) {
	tests := []struct {
		name           string
		mockStatusCode int
		mockResponse   string
		expectError    bool
		expectedData   map[string]interface{}
	}{
		{
			name:           "successful get instance info",
			mockStatusCode: http.StatusOK,
			mockResponse:   `{"id": "vm-123", "state": "Running", "vmm_version": "1.4.0"}`,
			expectError:    false,
			expectedData: map[string]interface{}{
				"id":          "vm-123",
				"state":       "Running",
				"vmm_version": "1.4.0",
			},
		},
		{
			name:           "empty response",
			mockStatusCode: http.StatusOK,
			mockResponse:   `{}`,
			expectError:    false,
			expectedData:   map[string]interface{}{},
		},
		{
			name:           "API returns error",
			mockStatusCode: http.StatusNotFound,
			mockResponse:   `{"fault_message": "Instance not found"}`,
			expectError:    true,
		},
		{
			name:           "invalid JSON response",
			mockStatusCode: http.StatusOK,
			mockResponse:   `{invalid json}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/", r.URL.Path)

				w.WriteHeader(tt.mockStatusCode)
				_, _ = w.Write([]byte(tt.mockResponse))
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			result, err := client.GetInstanceInfo(ctx)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedData, result)
			}
		})
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	// Create a handler that delays response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})

	socketPath, cleanup := mockUnixServer(t, handler)
	defer cleanup()

	client := NewClient(socketPath)

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to make a request with cancelled context
	err := client.SetBootSource(ctx, BootSource{
		KernelImagePath: "/path/to/kernel",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestClient_Timeout(t *testing.T) {
	// Create a handler that never responds
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than client timeout
		w.WriteHeader(http.StatusNoContent)
	})

	socketPath, cleanup := mockUnixServer(t, handler)
	defer cleanup()

	client := NewClient(socketPath)
	ctx := context.Background()

	// This should timeout
	err := client.SetBootSource(ctx, BootSource{
		KernelImagePath: "/path/to/kernel",
	})

	require.Error(t, err)
}

func TestClient_InvalidSocketPath(t *testing.T) {
	client := NewClient("/nonexistent/socket/path.sock")
	ctx := context.Background()

	err := client.SetBootSource(ctx, BootSource{
		KernelImagePath: "/path/to/kernel",
	})

	require.Error(t, err)
}

func TestClient_HTTPStatusCodes(t *testing.T) {
	statusCodes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	}

	for _, statusCode := range statusCodes {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(`{"fault_message": "Test error"}`))
			})

			socketPath, cleanup := mockUnixServer(t, handler)
			defer cleanup()

			client := NewClient(socketPath)
			ctx := context.Background()

			err := client.SetBootSource(ctx, BootSource{
				KernelImagePath: "/path/to/kernel",
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("%d", statusCode))
		})
	}
}
