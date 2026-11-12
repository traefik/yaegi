// A value of a native type of plain data (syscall.RawSockaddrAny) seen
// through unsafe as another native type that fits inside it
// (syscall.RawSockaddrInet4, RawSockaddrInet6), or as an interpreted struct
// of plain data: the views share the memory of the value, as in gc. Before,
// the VM refused the conversion ("needs byte-level memory"). A native type
// of plain data over the bytes of a []byte or of a local [16]byte is the
// same memory too.
package main

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

// family is an interpreted struct laid over the start of a sockaddr.
type family struct {
	Family uint16
	Port   [2]byte
}

// asInet6 gets the pointer as an unsafe.Pointer: the SSA does not show the
// type of the value.
func asInet6(p unsafe.Pointer) *syscall.RawSockaddrInet6 {
	return (*syscall.RawSockaddrInet6)(p)
}

func main() {
	var any syscall.RawSockaddrAny
	in4 := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&any))
	in4.Family = syscall.AF_INET
	in4.Port = 0x5000
	in4.Addr = [4]byte{127, 0, 0, 1}
	fmt.Println(any.Addr.Family, any.Addr.Data[:6])

	any.Addr.Data[5] = 2
	fmt.Println(in4.Addr, *in4 == syscall.RawSockaddrInet4{Family: syscall.AF_INET, Port: 0x5000, Addr: [4]byte{127, 0, 0, 2}})

	f := (*family)(unsafe.Pointer(&any))
	fmt.Println(f.Family, f.Port)
	f.Port = [2]byte{0x1f, 0x90}
	fmt.Println(in4.Port == 0x901f || in4.Port == 0x1f90, any.Addr.Data[:2])

	in6 := asInet6(unsafe.Pointer(&any))
	in6.Family = syscall.AF_INET6
	in6.Addr[15] = 1
	fmt.Println(any.Addr.Family, any.Addr.Data[:2], in6.Addr[15] == 1)

	p := new(syscall.RawSockaddrAny)
	(*syscall.RawSockaddrUnix)(unsafe.Pointer(p)).Path[0] = 'x'
	fmt.Println(p.Addr.Data[0])

	// a native type of plain data over real bytes, as golang.org/x/net
	// parses socket messages
	b := make([]byte, 32)
	binary.LittleEndian.PutUint16(b[4:], syscall.AF_INET)
	b[6], b[7] = 0x1f, 0x90
	copy(b[8:], []byte{10, 0, 0, 1})
	sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&b[4]))
	fmt.Println(sa.Family, sa.Port, sa.Addr)
	sa.Addr[3] = 9
	fmt.Println(b[8:12])
	var raw [16]byte
	sa2 := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&raw))
	sa2.Family = 2
	sa2.Addr = [4]byte{1, 2, 3, 4}
	fmt.Println(raw[:2], raw[4:8])
}
