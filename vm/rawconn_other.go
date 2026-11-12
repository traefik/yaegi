//go:build !unix

package vm

import "reflect"

// lentRead, lentRecvfrom, lentSetsockoptInt and lentGetsockoptInt are the functions of the
// syscall subset that take a lent descriptor (rawconn.go). They are not
// given outside unix: the descriptor is a Handle there, and the signatures
// of syscall are not the same.
func lentRead(m *Machine) reflect.Value          { return reflect.Value{} }
func lentRecvfrom(m *Machine) reflect.Value      { return reflect.Value{} }
func lentSetsockoptInt(m *Machine) reflect.Value { return reflect.Value{} }
func lentGetsockoptInt(m *Machine) reflect.Value { return reflect.Value{} }

// checkSockopt refuses every socket option outside unix: the subsets that
// call it (golang.org/x/sys/unix, xsysunix.go) exist only on unix.
func (m *Machine) checkSockopt(fn string, fd, level, opt int) {
	panic(&RefusedError{What: fn + " outside unix"})
}

// checkRecvfrom refuses Recvfrom outside unix, as checkSockopt.
func (m *Machine) checkRecvfrom(fn string, fd, flags int) {
	panic(&RefusedError{What: fn + " outside unix"})
}
