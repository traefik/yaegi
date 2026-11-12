//go:build !unix

package syscallplug

// timevalOK: syscall.TimevalToNsec is only on unix, there is nothing to
// check here.
func timevalOK() bool { return true }
