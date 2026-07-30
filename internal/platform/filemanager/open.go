package filemanager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Reveal asks the host file manager to select a local file. Linux file
// managers do not share a portable select-file contract, so its parent folder
// is opened instead.
func Reveal(filename string) error {
	absolute, err := existingAbsolutePath(filename, false)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", "/select,", absolute)
	case "darwin":
		command = exec.Command("open", "-R", absolute)
	case "linux":
		command = exec.Command("xdg-open", filepath.Dir(absolute))
	default:
		return fmt.Errorf("unsupported desktop platform %s", runtime.GOOS)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open local file manager: %w", err)
	}
	return nil
}

func OpenDirectory(directory string) error {
	absolute, err := existingAbsolutePath(directory, true)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("explorer.exe", absolute)
	case "darwin":
		command = exec.Command("open", absolute)
	case "linux":
		command = exec.Command("xdg-open", absolute)
	default:
		return fmt.Errorf("unsupported desktop platform %s", runtime.GOOS)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open local file manager: %w", err)
	}
	return nil
}

func existingAbsolutePath(value string, directory bool) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if info.IsDir() != directory {
		return "", fmt.Errorf("unexpected local path type")
	}
	return absolute, nil
}
