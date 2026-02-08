package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spluca/firecracker-agent/pkg/fileutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return log
}

func createTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func TestNewManager(t *testing.T) {
	vmsDir := "/var/lib/firecracker/vms"
	log := createTestLogger()

	tests := []struct {
		name       string
		useOverlay bool
	}{
		{
			name:       "manager with overlay enabled",
			useOverlay: true,
		},
		{
			name:       "manager with overlay disabled",
			useOverlay: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(vmsDir, tt.useOverlay, log)

			assert.NotNil(t, manager)
			assert.Equal(t, vmsDir, manager.vmsDir)
			assert.Equal(t, tt.useOverlay, manager.useOverlay)
			assert.Equal(t, log, manager.log)
		})
	}
}

func TestManager_EnsureVMsDir(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "successful directory creation",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			vmsDir := filepath.Join(tempDir, "vms")
			manager := NewManager(vmsDir, false, createTestLogger())

			err := manager.EnsureVMsDir()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify directory was created
				info, err := os.Stat(vmsDir)
				require.NoError(t, err)
				assert.True(t, info.IsDir())
			}
		})
	}
}

func TestManager_PrepareVMStorage_WithCopy(t *testing.T) {
	tempDir := t.TempDir()
	vmsDir := filepath.Join(tempDir, "vms")

	// Create test kernel and rootfs files
	kernelPath := createTestFile(t, tempDir, "vmlinux.bin", "kernel content")
	rootfsPath := createTestFile(t, tempDir, "rootfs.ext4", "rootfs content")

	manager := NewManager(vmsDir, false, createTestLogger())

	t.Run("successful storage preparation with copy", func(t *testing.T) {
		vmID := "test-vm-1"

		storage, err := manager.PrepareVMStorage(vmID, kernelPath, rootfsPath)

		require.NoError(t, err)
		assert.NotNil(t, storage)

		// Verify VM directory was created
		assert.DirExists(t, storage.VMDir)
		assert.Equal(t, filepath.Join(vmsDir, vmID), storage.VMDir)

		// Verify kernel was copied
		assert.FileExists(t, storage.KernelPath)
		assert.Equal(t, filepath.Join(vmsDir, vmID, "vmlinux.bin"), storage.KernelPath)
		kernelContent, err := os.ReadFile(storage.KernelPath)
		require.NoError(t, err)
		assert.Equal(t, "kernel content", string(kernelContent))

		// Verify rootfs was copied
		assert.FileExists(t, storage.RootfsPath)
		assert.Equal(t, filepath.Join(vmsDir, vmID, "rootfs.ext4"), storage.RootfsPath)
		rootfsContent, err := os.ReadFile(storage.RootfsPath)
		require.NoError(t, err)
		assert.Equal(t, "rootfs content", string(rootfsContent))

		// Verify socket and log paths
		assert.Equal(t, filepath.Join(vmsDir, vmID, "firecracker.socket"), storage.SocketPath)
		assert.Equal(t, filepath.Join(vmsDir, vmID, "firecracker.log"), storage.LogPath)
	})

	t.Run("failure when kernel does not exist", func(t *testing.T) {
		vmID := "test-vm-invalid-kernel"
		invalidKernelPath := filepath.Join(tempDir, "nonexistent-kernel.bin")

		storage, err := manager.PrepareVMStorage(vmID, invalidKernelPath, rootfsPath)

		require.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "failed to copy kernel")
	})

	t.Run("failure when rootfs does not exist", func(t *testing.T) {
		vmID := "test-vm-invalid-rootfs"
		invalidRootfsPath := filepath.Join(tempDir, "nonexistent-rootfs.ext4")

		storage, err := manager.PrepareVMStorage(vmID, kernelPath, invalidRootfsPath)

		require.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "failed to copy rootfs")
	})
}

func TestManager_PrepareVMStorage_WithOverlay(t *testing.T) {
	// Check if qemu-img is available
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping overlay tests")
	}

	tempDir := t.TempDir()
	vmsDir := filepath.Join(tempDir, "vms")

	// Create test kernel and rootfs files
	kernelPath := createTestFile(t, tempDir, "vmlinux.bin", "kernel content")
	rootfsPath := createTestFile(t, tempDir, "rootfs.ext4", "rootfs content")

	manager := NewManager(vmsDir, true, createTestLogger())

	t.Run("successful storage preparation with overlay", func(t *testing.T) {
		vmID := "test-vm-overlay"

		storage, err := manager.PrepareVMStorage(vmID, kernelPath, rootfsPath)

		require.NoError(t, err)
		assert.NotNil(t, storage)

		// Verify VM directory was created
		assert.DirExists(t, storage.VMDir)

		// Verify kernel path points to shared kernel (no copy)
		assert.Equal(t, kernelPath, storage.KernelPath)

		// Verify overlay rootfs was created
		assert.FileExists(t, storage.RootfsPath)
		assert.Equal(t, filepath.Join(vmsDir, vmID, "rootfs.ext4"), storage.RootfsPath)

		// Verify overlay directories were created
		upperDir := filepath.Join(storage.VMDir, "upper")
		workDir := filepath.Join(storage.VMDir, "work")
		assert.DirExists(t, upperDir)
		assert.DirExists(t, workDir)

		// Verify socket and log paths
		assert.Equal(t, filepath.Join(vmsDir, vmID, "firecracker.socket"), storage.SocketPath)
		assert.Equal(t, filepath.Join(vmsDir, vmID, "firecracker.log"), storage.LogPath)
	})

	t.Run("overlay creation fails with invalid base image", func(t *testing.T) {
		vmID := "test-vm-overlay-invalid"
		invalidRootfsPath := filepath.Join(tempDir, "nonexistent-rootfs.ext4")

		storage, err := manager.PrepareVMStorage(vmID, kernelPath, invalidRootfsPath)

		require.Error(t, err)
		assert.Nil(t, storage)
		assert.Contains(t, err.Error(), "failed to create overlay")
	})
}

func TestManager_CleanupVMStorage(t *testing.T) {
	tempDir := t.TempDir()
	vmsDir := filepath.Join(tempDir, "vms")
	manager := NewManager(vmsDir, false, createTestLogger())

	t.Run("successful cleanup of existing VM storage", func(t *testing.T) {
		vmID := "test-vm-cleanup"
		vmDir := filepath.Join(vmsDir, vmID)

		// Create VM directory with some files
		err := os.MkdirAll(vmDir, 0755)
		require.NoError(t, err)
		createTestFile(t, vmDir, "test-file.txt", "test content")

		// Verify directory exists
		assert.DirExists(t, vmDir)

		// Cleanup
		err = manager.CleanupVMStorage(vmID)

		require.NoError(t, err)
		// Verify directory was removed
		assert.NoDirExists(t, vmDir)
	})

	t.Run("cleanup of non-existent VM storage succeeds", func(t *testing.T) {
		vmID := "test-vm-nonexistent"

		err := manager.CleanupVMStorage(vmID)

		// os.RemoveAll does not return error if path doesn't exist
		require.NoError(t, err)
	})

	t.Run("cleanup removes nested directories and files", func(t *testing.T) {
		vmID := "test-vm-nested"
		vmDir := filepath.Join(vmsDir, vmID)

		// Create nested directory structure
		nestedDir := filepath.Join(vmDir, "subdir", "nested")
		err := os.MkdirAll(nestedDir, 0755)
		require.NoError(t, err)
		createTestFile(t, nestedDir, "nested-file.txt", "nested content")
		createTestFile(t, vmDir, "root-file.txt", "root content")

		// Verify structure exists
		assert.DirExists(t, vmDir)
		assert.DirExists(t, nestedDir)

		// Cleanup
		err = manager.CleanupVMStorage(vmID)

		require.NoError(t, err)
		// Verify everything was removed
		assert.NoDirExists(t, vmDir)
	})
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("successful file copy", func(t *testing.T) {
		srcPath := createTestFile(t, tempDir, "source.txt", "test content")
		dstPath := filepath.Join(tempDir, "destination.txt")

		err := fileutil.CopyFile(srcPath, dstPath)

		require.NoError(t, err)
		assert.FileExists(t, dstPath)

		// Verify content matches
		content, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
	})

	t.Run("copy fails when source does not exist", func(t *testing.T) {
		srcPath := filepath.Join(tempDir, "nonexistent.txt")
		dstPath := filepath.Join(tempDir, "destination2.txt")

		err := fileutil.CopyFile(srcPath, dstPath)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "cp failed")
	})

	t.Run("copy large file", func(t *testing.T) {
		// Create a larger file (1MB)
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}
		srcPath := filepath.Join(tempDir, "large-source.txt")
		err := os.WriteFile(srcPath, largeContent, 0644)
		require.NoError(t, err)

		dstPath := filepath.Join(tempDir, "large-destination.txt")

		err = fileutil.CopyFile(srcPath, dstPath)

		require.NoError(t, err)
		assert.FileExists(t, dstPath)

		// Verify size matches
		srcInfo, err := os.Stat(srcPath)
		require.NoError(t, err)
		dstInfo, err := os.Stat(dstPath)
		require.NoError(t, err)
		assert.Equal(t, srcInfo.Size(), dstInfo.Size())
	})
}

func TestManager_createOverlay(t *testing.T) {
	// Check if qemu-img is available
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available, skipping overlay tests")
	}

	tempDir := t.TempDir()
	manager := NewManager(tempDir, true, createTestLogger())

	t.Run("successful overlay creation", func(t *testing.T) {
		basePath := createTestFile(t, tempDir, "base.ext4", "base image content")
		overlayPath := filepath.Join(tempDir, "overlay.qcow2")
		workDir := filepath.Join(tempDir, "work")

		err := manager.createOverlay(basePath, overlayPath, workDir)

		require.NoError(t, err)
		assert.FileExists(t, overlayPath)

		// Verify upper and work directories were created
		upperDir := filepath.Join(workDir, "upper")
		workDirPath := filepath.Join(workDir, "work")
		assert.DirExists(t, upperDir)
		assert.DirExists(t, workDirPath)
	})

	t.Run("overlay creation fails with invalid base image", func(t *testing.T) {
		basePath := filepath.Join(tempDir, "nonexistent-base.ext4")
		overlayPath := filepath.Join(tempDir, "overlay2.qcow2")
		workDir := filepath.Join(tempDir, "work2")

		// Create work dir
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		err = manager.createOverlay(basePath, overlayPath, workDir)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create overlay")
	})
}

func TestManager_PrepareVMStorage_ConcurrentCreation(t *testing.T) {
	tempDir := t.TempDir()
	vmsDir := filepath.Join(tempDir, "vms")

	kernelPath := createTestFile(t, tempDir, "vmlinux.bin", "kernel content")
	rootfsPath := createTestFile(t, tempDir, "rootfs.ext4", "rootfs content")

	manager := NewManager(vmsDir, false, createTestLogger())

	// Create multiple VMs concurrently
	done := make(chan bool, 3)
	errors := make(chan error, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			vmID := filepath.Join("test-vm-concurrent-", string(rune('a'+id)))
			_, err := manager.PrepareVMStorage(vmID, kernelPath, rootfsPath)
			if err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 3; i++ {
		<-done
	}
	close(errors)

	// Verify no errors occurred
	for err := range errors {
		t.Errorf("Concurrent VM creation failed: %v", err)
	}
}

func TestManager_PermissionHandling(t *testing.T) {
	tempDir := t.TempDir()
	vmsDir := filepath.Join(tempDir, "vms")
	manager := NewManager(vmsDir, false, createTestLogger())

	kernelPath := createTestFile(t, tempDir, "vmlinux.bin", "kernel content")
	rootfsPath := createTestFile(t, tempDir, "rootfs.ext4", "rootfs content")

	t.Run("VM directory has correct permissions", func(t *testing.T) {
		vmID := "test-vm-permissions"

		storage, err := manager.PrepareVMStorage(vmID, kernelPath, rootfsPath)

		require.NoError(t, err)

		// Check VM directory permissions
		info, err := os.Stat(storage.VMDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
	})
}
