//go:build !unix

package contract

import (
	"os/exec"
	"time"
)

func defaultShell() []string {
	return []string{"sh", "-c"}
}

func configureCmd(cmd *exec.Cmd) {
	cmd.WaitDelay = 500 * time.Millisecond
}
