package firecracker

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	pb "github.com/spluca/firecracker-agent/api/proto/firecracker/v1"
	"github.com/spluca/firecracker-agent/internal/network"
	"github.com/spluca/firecracker-agent/internal/storage"
	"github.com/spluca/firecracker-agent/pkg/config"
	"github.com/sirupsen/logrus"
)

// VMManager defines the interface for managing Firecracker VMs.
type VMManager interface {
	CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.VMInfo, error)
	StartVM(ctx context.Context, vmID string) error
	StopVM(ctx context.Context, vmID string, force bool) error
	DeleteVM(ctx context.Context, vmID string) error
	GetVM(vmID string) (*pb.VMInfo, error)
	ListVMs() []*pb.VMInfo
}

// Compile-time check that Manager implements VMManager.
var _ VMManager = (*Manager)(nil)

// Manager manages Firecracker VMs
type Manager struct {
	cfg            *config.Config
	log            *logrus.Logger
	networkManager *network.Manager
	storageManager *storage.Manager
	vms            map[string]*VM
	mu             sync.RWMutex
}

// VM represents a Firecracker microVM
type VM struct {
	Info       *pb.VMInfo
	Process    *VMProcess
	SocketPath string
	TAPDevice  string
	CreatedAt  time.Time
}

// NewManager creates a new Firecracker manager
func NewManager(cfg *config.Config, log *logrus.Logger) (*Manager, error) {
	log.Info("Initializing Firecracker manager")

	// Create network manager
	networkMgr := network.NewManager(cfg.Network.BridgeName, cfg.Network.BridgeIP, cfg.Network.TapPrefix, log)

	// Ensure bridge exists
	if err := networkMgr.EnsureBridgeExists(); err != nil {
		return nil, fmt.Errorf("failed to ensure bridge exists: %w", err)
	}

	// Create storage manager
	storageMgr := storage.NewManager(cfg.Storage.VMsDir, cfg.Storage.UseOverlay, log)

	// Ensure VMs directory exists
	if err := storageMgr.EnsureVMsDir(); err != nil {
		return nil, fmt.Errorf("failed to ensure VMs directory: %w", err)
	}

	return &Manager{
		cfg:            cfg,
		log:            log,
		networkManager: networkMgr,
		storageManager: storageMgr,
		vms:            make(map[string]*VM),
	}, nil
}

// CreateVM creates and starts a new VM
func (m *Manager) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.VMInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if VM already exists
	if _, exists := m.vms[req.VmId]; exists {
		return nil, fmt.Errorf("VM %s already exists", req.VmId)
	}

	m.log.WithField("vm_id", req.VmId).Info("Creating VM")

	// Determine kernel and rootfs paths
	kernelPath := req.KernelPath
	if kernelPath == "" {
		kernelPath = m.cfg.Firecracker.KernelPath
	}

	rootfsPath := req.RootfsPath
	if rootfsPath == "" {
		rootfsPath = m.cfg.Firecracker.RootfsPath
	}

	// Deferred cleanup stack: on error, run cleanups in reverse order
	var cleanups []func()
	committed := false
	defer func() {
		if !committed {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}
	}()

	var vmStorage *storage.VMStorage
	var process *VMProcess
	var tapDevice string
	var macAddr string

	useJailer := m.cfg.Firecracker.UseJailer != nil && *m.cfg.Firecracker.UseJailer

	if useJailer {
		m.log.WithField("vm_id", req.VmId).Info("Using Firecracker jailer for security isolation")

		// Setup jail directory
		jailPaths, err := m.storageManager.SetupJailDirectory(req.VmId, kernelPath, rootfsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to setup jail directory: %w", err)
		}
		cleanups = append(cleanups, func() {
			m.storageManager.CleanupJail(req.VmId)
			m.storageManager.CleanupVMStorage(req.VmId)
		})

		jailPaths.FirecrackerBinary = m.cfg.Firecracker.BinaryPath

		// Create TAP device
		tapDevice, err = m.networkManager.CreateTAPDevice(req.VmId)
		if err != nil {
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}
		cleanups = append(cleanups, func() { m.networkManager.DeleteTAPDevice(tapDevice) })

		macAddr = m.networkManager.GenerateMAC(req.VmId)

		// Start jailed Firecracker process
		process, err = StartJailedProcess(
			ctx,
			m.cfg.Firecracker.JailerPath,
			req.VmId,
			jailPaths,
			m.cfg.Firecracker.JailUID,
			m.cfg.Firecracker.JailGID,
			m.log,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to start jailed Firecracker process: %w", err)
		}
		cleanups = append(cleanups, func() { process.Kill() })

		vmStorage = &storage.VMStorage{
			VMDir:      jailPaths.JailDir,
			RootfsPath: jailPaths.RootfsPath,
			KernelPath: jailPaths.KernelPath,
			SocketPath: jailPaths.SocketPath,
			LogPath:    jailPaths.LogPath,
		}
	} else {
		m.log.WithField("vm_id", req.VmId).Warn("Running Firecracker without jailer (security risk)")

		// Prepare storage (traditional mode without jailer)
		var err error
		vmStorage, err = m.storageManager.PrepareVMStorage(req.VmId, kernelPath, rootfsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare storage: %w", err)
		}
		cleanups = append(cleanups, func() { m.storageManager.CleanupVMStorage(req.VmId) })

		// Create TAP device
		tapDevice, err = m.networkManager.CreateTAPDevice(req.VmId)
		if err != nil {
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}
		cleanups = append(cleanups, func() { m.networkManager.DeleteTAPDevice(tapDevice) })

		macAddr = m.networkManager.GenerateMAC(req.VmId)

		// Start Firecracker process directly
		process, err = StartFirecrackerProcess(
			ctx,
			m.cfg.Firecracker.BinaryPath,
			vmStorage.SocketPath,
			vmStorage.LogPath,
			m.log,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to start Firecracker process: %w", err)
		}
		cleanups = append(cleanups, func() { process.Kill() })
	}

	// Configure Firecracker via API
	client := process.Client

	// Determine Gateway IP from Bridge CIDR
	gatewayIP := m.extractGatewayIP(m.cfg.Network.BridgeIP)

	// Build boot arguments
	bootArgs := "console=ttyS0 reboot=k panic=1 pci=off"
	if req.IpAddress != "" {
		bootArgs = fmt.Sprintf("%s ip=%s:%s:%s:255.255.255.0::eth0:off",
			bootArgs, req.IpAddress, gatewayIP, gatewayIP)
	}

	if err := client.SetBootSource(ctx, BootSource{
		KernelImagePath: vmStorage.KernelPath,
		BootArgs:        bootArgs,
	}); err != nil {
		return nil, fmt.Errorf("failed to set boot source: %w", err)
	}

	if err := client.SetMachineConfig(ctx, MachineConfig{
		VcpuCount:  req.VcpuCount,
		MemSizeMib: req.MemoryMb,
		Smt:        false,
	}); err != nil {
		return nil, fmt.Errorf("failed to set machine config: %w", err)
	}

	if err := client.AddDrive(ctx, Drive{
		DriveID:      "rootfs",
		PathOnHost:   vmStorage.RootfsPath,
		IsRootDevice: true,
		IsReadOnly:   false,
	}); err != nil {
		return nil, fmt.Errorf("failed to add drive: %w", err)
	}

	if err := client.AddNetworkInterface(ctx, NetworkInterface{
		IfaceID:     "eth0",
		HostDevName: tapDevice,
		GuestMAC:    macAddr,
	}); err != nil {
		return nil, fmt.Errorf("failed to add network interface: %w", err)
	}

	if err := client.StartInstance(ctx); err != nil {
		return nil, fmt.Errorf("failed to start instance: %w", err)
	}

	// Create VM info
	vmInfo := &pb.VMInfo{
		VmId:       req.VmId,
		State:      pb.VMState_VM_STATE_RUNNING,
		VcpuCount:  req.VcpuCount,
		MemoryMb:   req.MemoryMb,
		IpAddress:  req.IpAddress,
		SocketPath: vmStorage.SocketPath,
		CreatedAt:  time.Now().Unix(),
		Metadata:   req.Metadata,
	}

	m.vms[req.VmId] = &VM{
		Info:       vmInfo,
		Process:    process,
		SocketPath: vmStorage.SocketPath,
		TAPDevice:  tapDevice,
		CreatedAt:  time.Now(),
	}

	committed = true

	m.log.WithFields(logrus.Fields{
		"vm_id":      req.VmId,
		"vcpus":      req.VcpuCount,
		"memory":     req.MemoryMb,
		"ip":         req.IpAddress,
		"tap_device": tapDevice,
	}).Info("VM created successfully")

	return vmInfo, nil
}

// extractGatewayIP extracts the IP address from a CIDR string (e.g. "172.16.0.1/24" -> "172.16.0.1").
func (m *Manager) extractGatewayIP(cidr string) string {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		m.log.WithError(err).WithField("cidr", cidr).Warn("Failed to parse bridge CIDR, using raw value")
		return cidr
	}
	return ip.String()
}

// StartVM starts an existing VM (not applicable for Firecracker - VMs start on creation)
func (m *Manager) StartVM(ctx context.Context, vmID string) error {
	m.mu.RLock()
	vm, exists := m.vms[vmID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	// Check if already running
	if vm.Process != nil && vm.Process.IsRunning() {
		m.log.WithField("vm_id", vmID).Info("VM is already running")
		return nil
	}

	// For Firecracker, starting a stopped VM requires recreating it
	// This is a limitation of Firecracker architecture
	return fmt.Errorf("VM %s cannot be restarted - Firecracker VMs must be recreated", vmID)
}

// StopVM stops a running VM
func (m *Manager) StopVM(ctx context.Context, vmID string, force bool) error {
	m.mu.RLock()
	vm, exists := m.vms[vmID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	m.log.WithFields(logrus.Fields{
		"vm_id": vmID,
		"force": force,
	}).Info("Stopping VM")

	if vm.Process != nil {
		if force {
			if err := vm.Process.Kill(); err != nil {
				m.log.WithError(err).Warn("Failed to kill VM process")
			}
		} else {
			// Try graceful shutdown via Ctrl+Alt+Del
			if err := vm.Process.Client.SendCtrlAltDel(ctx); err != nil {
				m.log.WithError(err).Warn("Failed to send Ctrl+Alt+Del, forcing kill")
				vm.Process.Kill()
			} else {
				// Wait a bit then stop
				time.Sleep(2 * time.Second)
				vm.Process.Stop()
			}
		}
	}

	m.mu.Lock()
	vm.Info.State = pb.VMState_VM_STATE_STOPPED
	m.mu.Unlock()

	return nil
}

// DeleteVM deletes a VM and cleans up resources
func (m *Manager) DeleteVM(ctx context.Context, vmID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	m.log.WithField("vm_id", vmID).Info("Deleting VM")

	// Stop process if running
	if vm.Process != nil {
		if err := vm.Process.Kill(); err != nil {
			m.log.WithError(err).Warn("Failed to kill VM process")
		}
	}

	// Delete TAP device
	if vm.TAPDevice != "" {
		if err := m.networkManager.DeleteTAPDevice(vm.TAPDevice); err != nil {
			m.log.WithError(err).Warn("Failed to delete TAP device")
		}
	}

	// Cleanup jail directory if using jailer
	if m.cfg.Firecracker.UseJailer != nil && *m.cfg.Firecracker.UseJailer {
		if err := m.storageManager.CleanupJail(vmID); err != nil {
			m.log.WithError(err).Warn("Failed to cleanup jail directory")
		}
	}

	// Cleanup storage
	if err := m.storageManager.CleanupVMStorage(vmID); err != nil {
		m.log.WithError(err).Warn("Failed to cleanup VM storage")
	}

	// Remove from map
	delete(m.vms, vmID)

	m.log.WithField("vm_id", vmID).Info("VM deleted successfully")

	return nil
}

// resolveVMState returns the current state of a VM, checking process status.
// This does not mutate the VM, making it safe to call under RLock.
func (m *Manager) resolveVMState(vm *VM) pb.VMState {
	if vm.Process != nil && !vm.Process.IsRunning() {
		return pb.VMState_VM_STATE_STOPPED
	}
	return vm.Info.State
}

// copyVMInfo returns a copy of VMInfo with the current resolved state.
func (m *Manager) copyVMInfo(vm *VM) *pb.VMInfo {
	return &pb.VMInfo{
		VmId:       vm.Info.VmId,
		State:      m.resolveVMState(vm),
		VcpuCount:  vm.Info.VcpuCount,
		MemoryMb:   vm.Info.MemoryMb,
		IpAddress:  vm.Info.IpAddress,
		SocketPath: vm.Info.SocketPath,
		CreatedAt:  vm.Info.CreatedAt,
		Metadata:   vm.Info.Metadata,
	}
}

// GetVM retrieves VM information
func (m *Manager) GetVM(vmID string) (*pb.VMInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return nil, fmt.Errorf("VM %s not found", vmID)
	}

	return m.copyVMInfo(vm), nil
}

// ListVMs lists all VMs
func (m *Manager) ListVMs() []*pb.VMInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vms := make([]*pb.VMInfo, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, m.copyVMInfo(vm))
	}

	return vms
}
