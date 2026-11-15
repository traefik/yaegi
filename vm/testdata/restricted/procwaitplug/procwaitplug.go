// Package procwaitplug, loaded with Options.Exec = ExecMock, sets the
// Process of its own exec.Cmd and calls Wait: the native Wait would call
// the real Wait of that process, which reaps a child of the host.
package procwaitplug

import (
	"os"
	"os/exec"
)

func Wait(pid int) error {
	cmd := exec.Command("true")
	cmd.Process = &os.Process{Pid: pid}
	return cmd.Wait()
}
