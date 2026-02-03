package firecracker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/apardo/firecracker-agent/api/proto/firecracker/v1"
	"github.com/apardo/firecracker-agent/internal/network"
	"github.com/apardo/firecracker-agent/internal/storage"
	"github.com/apardo/firecracker-agent/pkg/config"
	"github.com/sirupsen/logrus"
)

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

	var vmStorage *storage.VMStorage
	var jailPaths *storage.JailPaths
	var process *VMProcess
	var tapDevice string
	var macAddr string
	var err error

	// Check if we should use jailer (enabled by default for security)
	useJailer := m.cfg.Firecracker.UseJailer

	if useJailer {
		m.log.WithField("vm_id", req.VmId).Info("Using Firecracker jailer for security isolation")

		// Setup jail directory
		jailPaths, err = m.storageManager.SetupJailDirectory(req.VmId, kernelPath, rootfsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to setup jail directory: %w", err)
		}

		// Update jail paths with firecracker binary path
		jailPaths.FirecrackerBinary = m.cfg.Firecracker.BinaryPath

		// Create TAP device
		tapDevice, err = m.networkManager.CreateTAPDevice(req.VmId)
		if err != nil {
			m.storageManager.CleanupJail(req.VmId)
			m.storageManager.CleanupVMStorage(req.VmId)
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}

		// Generate MAC address
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
			m.networkManager.DeleteTAPDevice(tapDevice)
			m.storageManager.CleanupJail(req.VmId)
			m.storageManager.CleanupVMStorage(req.VmId)
			return nil, fmt.Errorf("failed to start jailed Firecracker process: %w", err)
		}

		// Use jail paths for configuration
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
		vmStorage, err = m.storageManager.PrepareVMStorage(req.VmId, kernelPath, rootfsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare storage: %w", err)
		}

		// Create TAP device
		tapDevice, err = m.networkManager.CreateTAPDevice(req.VmId)
		if err != nil {
			m.storageManager.CleanupVMStorage(req.VmId)
			return nil, fmt.Errorf("failed to create TAP device: %w", err)
		}

		// Generate MAC address
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
			m.networkManager.DeleteTAPDevice(tapDevice)
			m.storageManager.CleanupVMStorage(req.VmId)
			return nil, fmt.Errorf("failed to start Firecracker process: %w", err)
		}
	}

	// Configure Firecracker via API
	client := process.Client

	// Determine Gateway IP (Bridge IP) - remove CIDR for gateway
	gatewayIP := m.cfg.Network.BridgeIP
	if idx := strings.Index(gatewayIP, "/"); idx != -1 {
		gatewayIP = gatewayIP[:idx]
	}

	// Build boot arguments
	bootArgs := "console=ttyS0 reboot=k panic=1 pci=off"
	if req.IpAddress != "" {
		// Format: ip=<client-ip>:<server-ip>:<gw-ip>:<netmask>:<hostname>:<device>:<autoconf>
		// We use bridge IP as server and gateway
		// Assuming /24 netmask (255.255.255.0) for simplicity as it matches default bridge config
		bootArgs = fmt.Sprintf("%s ip=%s:%s:%s:255.255.255.0::eth0:off",
			bootArgs, req.IpAddress, gatewayIP, gatewayIP)
	}

	// Set boot source
	bootSource := BootSource{
		KernelImagePath: vmStorage.KernelPath,
		BootArgs:        bootArgs,
	}
	if err := client.SetBootSource(ctx, bootSource); err != nil {
		process.Kill()
		m.networkManager.DeleteTAPDevice(tapDevice)
		m.storageManager.CleanupVMStorage(req.VmId)
		return nil, fmt.Errorf("failed to set boot source: %w", err)
	}

	// Set machine config
	machineConfig := MachineConfig{
		VcpuCount:  req.VcpuCount,
		MemSizeMib: req.MemoryMb,
		Smt:        false, // Disable SMT/Hyper-Threading
	}
	if err := client.SetMachineConfig(ctx, machineConfig); err != nil {
		process.Kill()
		m.networkManager.DeleteTAPDevice(tapDevice)
		m.storageManager.CleanupVMStorage(req.VmId)
		return nil, fmt.Errorf("failed to set machine config: %w", err)
	}

	// Add rootfs drive
	drive := Drive{
		DriveID:      "rootfs",
		PathOnHost:   vmStorage.RootfsPath,
		IsRootDevice: true,
		IsReadOnly:   false,
	}
	if err := client.AddDrive(ctx, drive); err != nil {
		process.Kill()
		m.networkManager.DeleteTAPDevice(tapDevice)
		m.storageManager.CleanupVMStorage(req.VmId)
		return nil, fmt.Errorf("failed to add drive: %w", err)
	}

	// Add network interface
	netIface := NetworkInterface{
		IfaceID:     "eth0",
		HostDevName: tapDevice,
		GuestMAC:    macAddr,
	}
	if err := client.AddNetworkInterface(ctx, netIface); err != nil {
		process.Kill()
		m.networkManager.DeleteTAPDevice(tapDevice)
		m.storageManager.CleanupVMStorage(req.VmId)
		return nil, fmt.Errorf("failed to add network interface: %w", err)
	}

	// Start the VM
	if err := client.StartInstance(ctx); err != nil {
		process.Kill()
		m.networkManager.DeleteTAPDevice(tapDevice)
		m.storageManager.CleanupVMStorage(req.VmId)
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

	vm := &VM{
		Info:       vmInfo,
		Process:    process,
		SocketPath: vmStorage.SocketPath,
		TAPDevice:  tapDevice,
		CreatedAt:  time.Now(),
	}

	m.vms[req.VmId] = vm

	m.log.WithFields(logrus.Fields{
		"vm_id":      req.VmId,
		"vcpus":      req.VcpuCount,
		"memory":     req.MemoryMb,
		"ip":         req.IpAddress,
		"tap_device": tapDevice,
	}).Info("VM created successfully")

	return vmInfo, nil
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
	if m.cfg.Firecracker.UseJailer {
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

// GetVM retrieves VM information
func (m *Manager) GetVM(vmID string) (*pb.VMInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return nil, fmt.Errorf("VM %s not found", vmID)
	}

	// Update state based on process status
	if vm.Process != nil && !vm.Process.IsRunning() {
		vm.Info.State = pb.VMState_VM_STATE_STOPPED
	}

	return vm.Info, nil
}

// ListVMs lists all VMs
func (m *Manager) ListVMs() []*pb.VMInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vms := make([]*pb.VMInfo, 0, len(m.vms))
	for _, vm := range m.vms {
		// Update state based on process status
		if vm.Process != nil && !vm.Process.IsRunning() {
			vm.Info.State = pb.VMState_VM_STATE_STOPPED
		}
		vms = append(vms, vm.Info)
	}

	return vms
}
