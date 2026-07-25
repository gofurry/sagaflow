package desktop

import (
	"fmt"
	"os/exec"
	"runtime"
)

func Open(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		command = exec.Command("open", target)
	case "linux":
		command = exec.Command("xdg-open", target)
	default:
		return fmt.Errorf("当前系统不支持自动打开：%s", target)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("打开 %s：%w", target, err)
	}
	return nil
}
