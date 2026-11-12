// An interpreted type with a SyscallConn method, given to native code as a
// syscall.Conn. The VM boxes it in the syscall.Conn wrapper of the
// unrestricted table (stdsyms.Unrestricted), so native code can call the
// method and get the interpreted value back.
package main

import (
	"errors"
	"fmt"
	"reflect"
	"syscall"
)

type fakeConn struct{ id int }

func (c fakeConn) SyscallConn() (syscall.RawConn, error) {
	return nil, fmt.Errorf("fake conn %d", c.id)
}

func main() {
	conns := []syscall.Conn{fakeConn{1}, fakeConn{2}}
	// reflect is native: it reads the elements as syscall.Conn values.
	rv := reflect.ValueOf(conns)
	for i := 0; i < rv.Len(); i++ {
		out := rv.Index(i).MethodByName("SyscallConn").Call(nil)
		err, _ := out[1].Interface().(error)
		fmt.Println(out[0].IsNil(), err)
	}
	c := rv.Index(1).Interface().(syscall.Conn)
	fc, ok := c.(fakeConn)
	fmt.Println(ok, fc.id)
	_, err := c.SyscallConn()
	fmt.Println(errors.Unwrap(err) == nil, err)
}
