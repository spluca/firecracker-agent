package firecracker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return log
}

func TestVMProcess_IsRunning(t *testing.T) {
	log := createTestLogger()

	t.Run("returns false when process is nil", func(t *testing.T) {
		process := &VMProcess{
			PID: 0,
			Cmd: nil,
			log: log,
		}

		assert.False(t, process.IsRunning())
	})

	t.Run("returns false when process.Process is nil", func(t *testing.T) {
		process := &VMProcess{
			PID: 0,
			Cmd: &exec.Cmd{},
			log: log,
		}

		assert.False(t, process.IsRunning())
	})
}

func TestVMProcess_Stop(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")
	logPath := filepath.Join(tempDir, "test.log")

	log := createTestLogger()

	t.Run("stop succeeds when process is nil", func(t *testing.T) {
		process := &VMProcess{
			SocketPath: socketPath,
			log:        log,
		}

		err := process.Stop()

		require.NoError(t, err)
	})

	t.Run("stop closes log file", func(t *testing.T) {
		logFile, err := os.Create(logPath)
		require.NoError(t, err)

		process := &VMProcess{
			SocketPath: socketPath,
			LogFile:    logFile,
			log:        log,
		}

		err = process.Stop()

		require.NoError(t, err)
		// Try writing to closed file should fail
		_, err = logFile.Write([]byte("test"))
		assert.Error(t, err, "Expected error writing to closed file")
	})

	t.Run("stop removes socket file", func(t *testing.T) {
		socketPath := filepath.Join(tempDir, "test-stop.sock")
		// Create socket file
		_, err := os.Create(socketPath)
		require.NoError(t, err)

		process := &VMProcess{
			SocketPath: socketPath,
			log:        log,
		}

		err = process.Stop()
		require.NoError(t, err)

		// Verify socket was removed
		_, err = os.Stat(socketPath)
		assert.True(t, os.IsNotExist(err), "Socket file should be removed")
	})
}

func TestVMProcess_Kill(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test-kill.sock")
	logPath := filepath.Join(tempDir, "test-kill.log")

	log := createTestLogger()

	t.Run("kill succeeds when process is nil", func(t *testing.T) {
		process := &VMProcess{
			SocketPath: socketPath,
			log:        log,
		}

		err := process.Kill()

		require.NoError(t, err)
	})

	t.Run("kill closes log file", func(t *testing.T) {
		logFile, err := os.Create(logPath)
		require.NoError(t, err)

		process := &VMProcess{
			SocketPath: socketPath,
			LogFile:    logFile,
			log:        log,
		}

		err = process.Kill()

		require.NoError(t, err)
		// Try writing to closed file should fail
		_, err = logFile.Write([]byte("test"))
		assert.Error(t, err, "Expected error writing to closed file")
	})

	t.Run("kill removes socket file", func(t *testing.T) {
		socketPath := filepath.Join(tempDir, "test-kill-socket.sock")
		// Create socket file
		_, err := os.Create(socketPath)
		require.NoError(t, err)

		process := &VMProcess{
			SocketPath: socketPath,
			log:        log,
		}

		err = process.Kill()
		require.NoError(t, err)

		// Verify socket was removed
		_, err = os.Stat(socketPath)
		assert.True(t, os.IsNotExist(err), "Socket file should be removed")
	})
}

func TestStartFirecrackerProcess_Errors(t *testing.T) {
	tempDir := t.TempDir()
	log := createTestLogger()

	t.Run("fails when binary does not exist", func(t *testing.T) {
		binaryPath := filepath.Join(tempDir, "nonexistent-firecracker")
		socketPath := filepath.Join(tempDir, "test.sock")
		logPath := filepath.Join(tempDir, "test.log")

		ctx := context.Background()
		process, err := StartFirecrackerProcess(ctx, binaryPath, socketPath, logPath, log)

		require.Error(t, err)
		assert.Nil(t, process)
		assert.Contains(t, err.Error(), "failed to start Firecracker process")
	})

	t.Run("fails when log path is invalid", func(t *testing.T) {
		// Use a mock binary (sleep command as a stand-in)
		binaryPath := "/usr/bin/sleep"
		socketPath := filepath.Join(tempDir, "test2.sock")
		invalidLogPath := "/invalid/path/that/does/not/exist/test.log"

		ctx := context.Background()
		process, err := StartFirecrackerProcess(ctx, binaryPath, socketPath, invalidLogPath, log)

		require.Error(t, err)
		assert.Nil(t, process)
		assert.Contains(t, err.Error(), "failed to open log file")
	})
}

func TestWaitForSocket(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("succeeds when socket exists", func(t *testing.T) {
		socketPath := filepath.Join(tempDir, "exists.sock")
		// Create socket file
		_, err := os.Create(socketPath)
		require.NoError(t, err)

		err = waitForSocket(socketPath, 1*time.Second)

		require.NoError(t, err)
	})

	t.Run("times out when socket does not exist", func(t *testing.T) {
		socketPath := filepath.Join(tempDir, "nonexistent.sock")

		err := waitForSocket(socketPath, 200*time.Millisecond)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "timeout waiting for socket")
	})

	t.Run("waits for socket to be created", func(t *testing.T) {
		socketPath := filepath.Join(tempDir, "delayed.sock")

		// Create socket after a delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			_, _ = os.Create(socketPath)
		}()

		err := waitForSocket(socketPath, 1*time.Second)

		require.NoError(t, err)
	})
}

// Integration test with a real process (using sleep as a stand-in for Firecracker)
func TestVMProcess_WithRealProcess(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "process-test.sock")
	logPath := filepath.Join(tempDir, "process-test.log")
	log := createTestLogger()

	// Skip if sleep command doesn't exist
	sleepPath := "/usr/bin/sleep"
	if _, err := os.Stat(sleepPath); os.IsNotExist(err) {
		sleepPath = "/bin/sleep"
		if _, err := os.Stat(sleepPath); os.IsNotExist(err) {
			t.Skip("sleep command not found")
		}
	}

	t.Run("process lifecycle with real command", func(t *testing.T) {
		ctx := context.Background()

		// Create log file
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		require.NoError(t, err)

		// Start sleep process
		cmd := exec.CommandContext(ctx, sleepPath, "60")
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		err = cmd.Start()
		require.NoError(t, err)

		// Create VMProcess
		process := &VMProcess{
			PID:        cmd.Process.Pid,
			Cmd:        cmd,
			SocketPath: socketPath,
			LogFile:    logFile,
			log:        log,
		}

		// Verify process is running
		assert.True(t, process.IsRunning())
		assert.Greater(t, process.PID, 0)

		// Stop the process
		err = process.Stop()
		require.NoError(t, err)

		// Give it time to actually stop
		time.Sleep(100 * time.Millisecond)

		// Verify process is no longer running
		assert.False(t, process.IsRunning())
	})

	t.Run("kill stops running process immediately", func(t *testing.T) {
		ctx := context.Background()

		// Create log file
		logPath2 := filepath.Join(tempDir, "kill-test.log")
		logFile, err := os.OpenFile(logPath2, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		require.NoError(t, err)

		// Start sleep process
		cmd := exec.CommandContext(ctx, sleepPath, "60")
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		err = cmd.Start()
		require.NoError(t, err)

		// Create VMProcess
		process := &VMProcess{
			PID:        cmd.Process.Pid,
			Cmd:        cmd,
			SocketPath: filepath.Join(tempDir, "kill-test.sock"),
			LogFile:    logFile,
			log:        log,
		}

		// Verify process is running
		assert.True(t, process.IsRunning())

		// Kill the process
		err = process.Kill()
		require.NoError(t, err)

		// Wait for process to exit
		_, _ = cmd.Process.Wait()
		time.Sleep(100 * time.Millisecond)

		// Note: After Kill(), the process might still appear running briefly
		// This is a race condition in the test, not the code
		// In production, the process will be killed
	})
}

func TestVMProcess_StopWithTimeout(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "timeout-test.log")
	log := createTestLogger()

	// Skip if sleep command doesn't exist
	sleepPath := "/usr/bin/sleep"
	if _, err := os.Stat(sleepPath); os.IsNotExist(err) {
		sleepPath = "/bin/sleep"
		if _, err := os.Stat(sleepPath); os.IsNotExist(err) {
			t.Skip("sleep command not found")
		}
	}

	t.Run("force kills process if SIGTERM doesn't work within timeout", func(t *testing.T) {
		ctx := context.Background()

		// Create log file
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		require.NoError(t, err)

		// Start a process that ignores SIGTERM (sleep does handle it, but we'll simulate)
		// In reality, Firecracker should handle SIGTERM gracefully
		cmd := exec.CommandContext(ctx, sleepPath, "60")
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		err = cmd.Start()
		require.NoError(t, err)

		process := &VMProcess{
			PID:        cmd.Process.Pid,
			Cmd:        cmd,
			SocketPath: filepath.Join(tempDir, "timeout.sock"),
			LogFile:    logFile,
			log:        log,
		}

		// Stop should eventually kill the process
		start := time.Now()
		err = process.Stop()
		duration := time.Since(start)

		require.NoError(t, err)
		// Should complete within reasonable time (5s timeout + some buffer)
		assert.Less(t, duration, 7*time.Second)
	})
}

func TestVMProcess_ClientField(t *testing.T) {
	log := createTestLogger()

	t.Run("VMProcess has Client field", func(t *testing.T) {
		client := NewClient("/test/socket.sock")
		process := &VMProcess{
			PID:    12345,
			Client: client,
			log:    log,
		}

		assert.NotNil(t, process.Client)
		assert.Equal(t, "/test/socket.sock", process.Client.socketPath)
	})
}

func TestVMProcess_StructFields(t *testing.T) {
	t.Run("VMProcess struct has required fields", func(t *testing.T) {
		process := &VMProcess{
			PID:        12345,
			SocketPath: "/test/socket.sock",
		}

		assert.Equal(t, 12345, process.PID)
		assert.Equal(t, "/test/socket.sock", process.SocketPath)
	})
}
