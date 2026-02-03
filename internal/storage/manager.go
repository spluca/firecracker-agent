package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

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
		if err := m.copyFile(kernelPath, storage.KernelPath); err != nil {
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
		if err := m.copyFile(rootfsPath, storage.RootfsPath); err != nil {
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

// copyFile copies a file from src to dst
func (m *Manager) copyFile(src, dst string) error {
	m.log.WithFields(logrus.Fields{
		"src": src,
		"dst": dst,
	}).Debug("Copying file")

	// Use cp command for efficient copy
	cmd := exec.Command("cp", src, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to copy file: %w (output: %s)", err, string(output))
	}

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
