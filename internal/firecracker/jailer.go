package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apardo/firecracker-agent/internal/storage"
	"github.com/sirupsen/logrus"
)

// StartJailedProcess starts a new Firecracker process using the native firecracker jailer
// Native firecracker jailer implementation:
// - chroot-base-dir: /var/lib/firecracker/vms
// - jail structure: firecracker/<vm_id>/root/
// - socket: /run/firecracker.socket (inside chroot)
// - kernel: /root/vmlinux (inside chroot)
// - rootfs: /root/rootfs.ext4 (inside chroot)
func StartJailedProcess(
	ctx context.Context,
	jailerPath string,
	vmID string,
	jailPaths *storage.JailPaths,
	uid, gid int,
	log *logrus.Logger,
) (*VMProcess, error) {
	log.WithFields(logrus.Fields{
		"jailer": jailerPath,
		"vm_id":  vmID,
		"uid":    uid,
		"gid":    gid,
	}).Info("Starting Firecracker with native jailer")

	// Verify source files exist
	if err := verifyFileExists(jailPaths.FirecrackerBinary, "firecracker binary"); err != nil {
		return nil, err
	}
	if err := verifyFileExists(jailPaths.KernelPath, "kernel"); err != nil {
		return nil, err
	}
	if err := verifyFileExists(jailPaths.RootfsPath, "rootfs"); err != nil {
		return nil, err
	}

	// Structure: <chroot-base-dir>/firecracker/<vm_id>/root/
	chrootBaseDir := filepath.Dir(jailPaths.JailDir)
	jailIdDir := filepath.Join(chrootBaseDir, "firecracker", vmID)
	jailRootDir := filepath.Join(jailIdDir, "root")

	// Socket path: <chroot-base-dir>/firecracker/<vm_id>/root/run/firecracker.socket
	chrootSocketPath := filepath.Join(jailRootDir, "run", "firecracker.socket")

	log.WithFields(logrus.Fields{
		"chroot_base_dir": chrootBaseDir,
		"jail_dir":        jailIdDir,
		"root_dir":        jailRootDir,
		"socket":          chrootSocketPath,
	}).Debug("Jail paths")

	// STEP 1: Create jail directories
	if err := os.MkdirAll(jailRootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create jail root directory: %w", err)
	}
	runDir := filepath.Join(jailRootDir, "run")
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run directory: %w", err)
	}

	// STEP 2: Copy files to jail before starting jailer
	// The jailer will chroot and drop privileges, so we must copy files now
	jailedFirecrackerPath := filepath.Join(jailRootDir, "firecracker")
	jailedKernelPath := filepath.Join(jailRootDir, "vmlinux")
	jailedRootfsPath := filepath.Join(jailRootDir, "rootfs.ext4")

	if err := copyFile(jailPaths.FirecrackerBinary, jailedFirecrackerPath); err != nil {
		return nil, fmt.Errorf("failed to copy firecracker binary: %w", err)
	}
	if err := copyFile(jailPaths.KernelPath, jailedKernelPath); err != nil {
		return nil, fmt.Errorf("failed to copy kernel: %w", err)
	}
	if err := copyFile(jailPaths.RootfsPath, jailedRootfsPath); err != nil {
		return nil, fmt.Errorf("failed to copy rootfs: %w", err)
	}

	// STEP 3: Set ownership so jailer can access files after dropping privileges
	if err := os.Chown(jailRootDir, uid, gid); err != nil {
		log.WithError(err).Warn("Failed to chown root dir")
	}
	if err := os.Chown(runDir, uid, gid); err != nil {
		log.WithError(err).Warn("Failed to chown run dir")
	}
	if err := os.Chown(jailedFirecrackerPath, uid, gid); err != nil {
		log.WithError(err).Warn("Failed to chown firecracker")
	}
	if err := os.Chown(jailedKernelPath, uid, gid); err != nil {
		log.WithError(err).Warn("Failed to chown kernel")
	}
	if err := os.Chown(jailedRootfsPath, uid, gid); err != nil {
		log.WithError(err).Warn("Failed to chown rootfs")
	}

	// Remove existing socket
	os.Remove(chrootSocketPath)

	// STEP 4: Build jailer command
	// The jailer creates chroot, drops privileges, and runs firecracker
	// We pass the HOST path to the firecracker binary inside the jail
	args := []string{
		"--id", vmID,
		"--uid", fmt.Sprintf("%d", uid),
		"--gid", fmt.Sprintf("%d", gid),
		"--chroot-base-dir", chrootBaseDir,
		"--exec-file", jailedFirecrackerPath, // Host path to firecracker inside jail
		"--cgroup-version", "2",
		"--",
		"--api-sock", "/run/firecracker.socket",
		"--boot-timer",
	}

	log.WithFields(logrus.Fields{
		"jailer": jailerPath,
		"args":   args,
	}).Info("Starting jailer")

	cmd := exec.CommandContext(context.Background(), jailerPath, args...)

	// Create log file and redirect output
	logFile, err := os.OpenFile(jailPaths.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil // /dev/null

	// Set process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the jailer
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start jailer: %w", err)
	}

	log.WithFields(logrus.Fields{
		"pid":   cmd.Process.Pid,
		"vm_id": vmID,
	}).Info("Jailer started, waiting for socket...")

	// Wait for jailer to initialize
	time.Sleep(500 * time.Millisecond)

	// Check if process exited immediately
	if !isProcessRunning(cmd.Process.Pid) {
		logContent, _ := os.ReadFile(jailPaths.LogPath)
		return nil, fmt.Errorf("jailer exited immediately. Log: %s", string(logContent))
	}

	// Start goroutine to monitor jailer
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.WithError(err).WithField("pid", cmd.Process.Pid).Warn("Jailer exited with error")
		} else {
			log.WithField("pid", cmd.Process.Pid).Info("Jailer exited cleanly")
		}
	}()

	// Wait for socket
	if err := waitForSocket(chrootSocketPath, 20*time.Second); err != nil {
		logContent, _ := os.ReadFile(jailPaths.LogPath)
		cmd.Process.Kill()
		return nil, fmt.Errorf("socket not ready: %w. Jailer log: %s", err, string(logContent))
	}

	log.WithField("socket", chrootSocketPath).Info("Socket ready, jailer initialized successfully")

	// Create API client
	client := NewClient(chrootSocketPath)

	// Update paths for Firecracker API calls (paths inside chroot)
	jailPaths.KernelPath = "/vmlinux"
	jailPaths.RootfsPath = "/rootfs.ext4"
	jailPaths.SocketPath = chrootSocketPath
	jailPaths.JailDir = jailIdDir

	return &VMProcess{
		PID:        cmd.Process.Pid,
		Cmd:        cmd,
		SocketPath: chrootSocketPath,
		LogFile:    logFile,
		Client:     client,
		log:        log,
		Mode:       ModeJailer,
		JailPath:   jailIdDir,
	}, nil
}

// verifyFileExists checks if a file exists
func verifyFileExists(path, description string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s not found at %s: %w", description, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s at %s is a directory, not a file", description, path)
	}
	return nil
}

// isProcessRunning checks if a process is still running
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// copyFile copies a file using cp command
func copyFile(src, dst string) error {
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	cmd := exec.Command("cp", "-p", src, dst)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp failed: %w (output: %s)", err, string(output))
	}
	return nil
}
