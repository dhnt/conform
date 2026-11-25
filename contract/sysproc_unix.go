//go:build unix

package contract

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func defaultShell() []string {
	if _, err := os.Stat("/bin/sh"); err == nil {
		return []string{"/bin/sh", "-c"}
	}
	return []string{"sh", "-c"}
}

func configureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 500 * time.Millisecond
}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
