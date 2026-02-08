package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spluca/firecracker-agent/pkg/fileutil"
	"github.com/sirupsen/logrus"
)

// StorageManager defines the interface for VM storage management.
type StorageManager interface {
	PrepareVMStorage(vmID, kernelPath, rootfsPath string) (*VMStorage, error)
	CleanupVMStorage(vmID string) error
	SetupJailDirectory(vmID, kernelPath, rootfsPath string) (*JailPaths, error)
	CleanupJail(vmID string) error
	EnsureVMsDir() error
}

// Compile-time check that Manager implements StorageManager.
var _ StorageManager = (*Manager)(nil)

// Manager handles storage configuration for VMs
type Manager struct {
	vmsDir     string
	useOverlay bool
	log        *logrus.Logger
}

// NewManager creates a new storage manager
func NewManager(vmsDir string, useOverlay bool, log *logrus.Logger) *Manager {
	return &Manager{
		vmsDir:     vmsDir,
		useOverlay: useOverlay,
		log:        log,
	}
}

// VMStorage represents VM storage paths
type VMStorage struct {
	VMDir      string
	RootfsPath string
	KernelPath string
	SocketPath string
	LogPath    string
}

// JailPaths represents paths for a jailed VM
type JailPaths struct {
	JailDir           string // Base chroot directory
	FirecrackerBinary string // Path to firecracker binary
	KernelPath        string // Path to kernel
	RootfsPath        string // Path to rootfs
	SocketPath        string // Path to socket
	LogPath           string // Path to logs
}

// PrepareVMStorage prepares storage directories and files for a VM
func (m *Manager) PrepareVMStorage(vmID, kernelPath, rootfsPath string) (*VMStorage, error) {
	vmDir := filepath.Join(m.vmsDir, vmID)

	m.log.WithFields(logrus.Fields{
		"vm_id":  vmID,
		"vm_dir": vmDir,
	}).Info("Preparing VM storage")

	// Create VM directory
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create VM directory: %w", err)
	}

	storage := &VMStorage{
		VMDir:      vmDir,
		SocketPath: filepath.Join(vmDir, "firecracker.socket"),
		LogPath:    filepath.Join(vmDir, "firecracker.log"),
	}

	// Handle kernel
	if m.useOverlay {
		// Create symlink to shared kernel
		storage.KernelPath = kernelPath
	} else {
		// Copy kernel to VM directory
		storage.KernelPath = filepath.Join(vmDir, "vmlinux.bin")
		if err := fileutil.CopyFile(kernelPath, storage.KernelPath); err != nil {
			return nil, fmt.Errorf("failed to copy kernel: %w", err)
		}
	}

	// Handle rootfs
	if m.useOverlay {
		// Create overlay filesystem for rootfs
		overlayPath := filepath.Join(vmDir, "rootfs.ext4")
		if err := m.createOverlay(rootfsPath, overlayPath, vmDir); err != nil {
			return nil, fmt.Errorf("failed to create overlay: %w", err)
		}
		storage.RootfsPath = overlayPath
	} else {
		// Copy rootfs to VM directory
		storage.RootfsPath = filepath.Join(vmDir, "rootfs.ext4")
		if err := fileutil.CopyFile(rootfsPath, storage.RootfsPath); err != nil {
			return nil, fmt.Errorf("failed to copy rootfs: %w", err)
		}
	}

	m.log.WithFields(logrus.Fields{
		"vm_id":       vmID,
		"kernel_path": storage.KernelPath,
		"rootfs_path": storage.RootfsPath,
	}).Info("VM storage prepared")

	return storage, nil
}

// CleanupVMStorage removes VM storage
func (m *Manager) CleanupVMStorage(vmID string) error {
	vmDir := filepath.Join(m.vmsDir, vmID)

	m.log.WithFields(logrus.Fields{
		"vm_id":  vmID,
		"vm_dir": vmDir,
	}).Info("Cleaning up VM storage")

	// Remove VM directory
	if err := os.RemoveAll(vmDir); err != nil {
		return fmt.Errorf("failed to remove VM directory: %w", err)
	}

	m.log.WithField("vm_id", vmID).Info("VM storage cleaned up")
	return nil
}

// createOverlay creates an overlay filesystem for copy-on-write
func (m *Manager) createOverlay(basePath, overlayPath, workDir string) error {
	m.log.WithFields(logrus.Fields{
		"base":    basePath,
		"overlay": overlayPath,
	}).Debug("Creating overlay filesystem")

	upperDir := filepath.Join(workDir, "upper")
	workDirPath := filepath.Join(workDir, "work")

	// Create directories
	if err := os.MkdirAll(upperDir, 0755); err != nil {
		return fmt.Errorf("failed to create upper dir: %w", err)
	}
	if err := os.MkdirAll(workDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create work dir: %w", err)
	}

	// For ext4 images, we'll use a copy-on-write approach with qemu-img
	// Create a qcow2 overlay with the base image as backing file
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", "-b", basePath, "-F", "raw", overlayPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create overlay: %w (output: %s)", err, string(output))
	}

	m.log.WithField("overlay", overlayPath).Info("Overlay filesystem created")
	return nil
}

// EnsureVMsDir ensures the VMs directory exists
func (m *Manager) EnsureVMsDir() error {
	m.log.WithField("vms_dir", m.vmsDir).Info("Ensuring VMs directory exists")

	if err := os.MkdirAll(m.vmsDir, 0755); err != nil {
		return fmt.Errorf("failed to create VMs directory: %w", err)
	}

	return nil
}

// SetupJailDirectory creates the chroot jail structure for a VM
// For the native firecracker jailer, we only prepare the base directory
// The jailer creates: <chroot-base-dir>/firecracker/<vm_id>/root/
// We return paths pointing to the source files (kernel, rootfs) which will be
// copied into the jail AFTER the jailer creates the chroot structure
func (m *Manager) SetupJailDirectory(vmID, kernelPath, rootfsPath string) (*JailPaths, error) {
	vmDir := filepath.Join(m.vmsDir, vmID)

	m.log.WithFields(logrus.Fields{
		"vm_id":  vmID,
		"vm_dir": vmDir,
	}).Info("Setting up jail directory")

	// Create VM directory (this will be the base for jailer operations)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create VM directory: %w", err)
	}

	// For native jailer:
	// - JailDir is the VM directory (jailer uses parent as chroot-base-dir)
	// - KernelPath and RootfsPath point to source files (will be copied to jail later)
	// - SocketPath will be set after jailer creates the chroot
	jailPaths := &JailPaths{
		JailDir:           vmDir,      // jailer uses parent of this as chroot-base-dir
		FirecrackerBinary: "",         // Set by manager
		KernelPath:        kernelPath, // Source kernel path (will be copied to jail)
		RootfsPath:        rootfsPath, // Source rootfs path (will be copied to jail)
		SocketPath:        "",         // Will be set after jailer creates chroot
		LogPath:           filepath.Join(vmDir, "firecracker.log"),
	}

	m.log.WithFields(logrus.Fields{
		"vm_id":      vmID,
		"vm_dir":     vmDir,
		"kernel_src": kernelPath,
		"rootfs_src": rootfsPath,
	}).Debug("Jail paths configured for native jailer")

	return jailPaths, nil
}

// CleanupJail removes the jail directory created by the native firecracker jailer
// The jailer creates: <vms_dir>/firecracker/<vm_id>/
func (m *Manager) CleanupJail(vmID string) error {
	// For native jailer: <vms_dir>/firecracker/<vm_id>/
	jailDir := filepath.Join(m.vmsDir, "firecracker", vmID)

	m.log.WithFields(logrus.Fields{
		"vm_id":    vmID,
		"jail_dir": jailDir,
	}).Info("Cleaning up jail directory")

	if err := os.RemoveAll(jailDir); err != nil {
		// Don't fail if directory doesn't exist
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to remove jail directory: %w", err)
	}

	return nil
}
