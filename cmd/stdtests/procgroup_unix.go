//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// ownGroup runs cmd in a process group of its own and kills the whole
// group on timeout: also the programs that the test started.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
