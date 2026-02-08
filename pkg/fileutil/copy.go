package fileutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CopyFile copies a file from src to dst using the cp command.
// It creates parent directories if they don't exist.
func CopyFile(src, dst string) error {
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
