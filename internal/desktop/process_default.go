//go:build !windows

package desktop

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
