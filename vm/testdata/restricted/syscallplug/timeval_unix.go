//go:build unix

package syscallplug

import "syscall"

// timevalOK checks the conversions of a Timeval (syscall.TimevalToNsec is
// only on unix).
func timevalOK() bool {
	return syscall.TimevalToNsec(syscall.NsecToTimeval(2_000_000_000)) == 2_000_000_000
}
