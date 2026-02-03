package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// VMProcess represents a running Firecracker process
type VMProcess struct {
	PID        int
	Cmd        *exec.Cmd
	SocketPath string
	LogFile    *os.File
	Client     *Client
	log        *logrus.Logger
}

// StartFirecrackerProcess starts a new Firecracker process
func StartFirecrackerProcess(ctx context.Context, binaryPath, socketPath, logPath string, log *logrus.Logger) (*VMProcess, error) {
	log.WithFields(logrus.Fields{
		"binary": binaryPath,
		"socket": socketPath,
		"log":    logPath,
	}).Info("Starting Firecracker process")

	// Remove socket if it exists
	os.Remove(socketPath)

	// Open log file
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create Firecracker command
	cmd := exec.CommandContext(ctx, binaryPath,
		"--api-sock", socketPath,
		"--log-path", logPath,
		"--level", "Info",
	)

	// Set stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Set process group (for easier cleanup)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("failed to start Firecracker process: %w", err)
	}

	log.WithFields(logrus.Fields{
		"pid":    cmd.Process.Pid,
		"socket": socketPath,
	}).Info("Firecracker process started")

	// Wait for socket to be ready
	if err := waitForSocket(socketPath, 5*time.Second); err != nil {
		cmd.Process.Kill()
		logFile.Close()
		return nil, fmt.Errorf("socket not ready: %w", err)
	}

	// Create API client
	client := NewClient(socketPath)

	return &VMProcess{
		PID:        cmd.Process.Pid,
		Cmd:        cmd,
		SocketPath: socketPath,
		LogFile:    logFile,
		Client:     client,
		log:        log,
	}, nil
}

// Stop gracefully stops the Firecracker process
func (p *VMProcess) Stop() error {
	p.log.WithField("pid", p.PID).Info("Stopping Firecracker process")

	if p.Cmd != nil && p.Cmd.Process != nil {
		// Send SIGTERM for graceful shutdown
		if err := p.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
			p.log.WithError(err).Warn("Failed to send SIGTERM")
		}

		// Wait for process to exit (with timeout)
		done := make(chan error, 1)
		go func() {
			done <- p.Cmd.Wait()
		}()

		select {
		case <-done:
			p.log.WithField("pid", p.PID).Info("Firecracker process stopped gracefully")
		case <-time.After(5 * time.Second):
			// Force kill if it doesn't stop
			p.log.WithField("pid", p.PID).Warn("Forcing kill of Firecracker process")
			p.Cmd.Process.Kill()
		}
	}

	// Close log file
	if p.LogFile != nil {
		p.LogFile.Close()
	}

	// Remove socket
	os.Remove(p.SocketPath)

	return nil
}

// Kill forcefully kills the Firecracker process
func (p *VMProcess) Kill() error {
	p.log.WithField("pid", p.PID).Info("Killing Firecracker process")

	if p.Cmd != nil && p.Cmd.Process != nil {
		if err := p.Cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
	}

	// Close log file
	if p.LogFile != nil {
		p.LogFile.Close()
	}

	// Remove socket
	os.Remove(p.SocketPath)

	return nil
}

// IsRunning checks if the process is still running
func (p *VMProcess) IsRunning() bool {
	if p.Cmd == nil || p.Cmd.Process == nil {
		return false
	}

	// Send signal 0 to check if process exists
	err := p.Cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// waitForSocket waits for a Unix socket to be created
func waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for socket: %s", socketPath)
}
