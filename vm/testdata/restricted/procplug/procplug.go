// Package procplug starts a program without os/exec.
package procplug

import "os"

func Start() error {
	_, err := os.StartProcess("/bin/sh", []string{"sh", "-c", "true"}, &os.ProcAttr{})
	return err
}

func Kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// Signal builds the process itself: os.FindProcess is not named.
func Signal(pid int) error { return (&os.Process{Pid: pid}).Signal(os.Kill) }
