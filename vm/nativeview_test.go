package vm

import (
	"runtime"
	"strings"
	"testing"
)

// TestNativeViewChecks checks the refusals of a view of a native value as
// another type (nativeview.go): a type that does not fit inside the value,
// a native value with a pointer word, and a target type with a pointer
// word. Each one must be a clear "unsupported", never a view: in gc the
// first one reads past the value, and the others could hide a pointer from
// the garbage collector. testdata/nativeprefixview.go has the views that
// work.
func TestNativeViewChecks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the program uses the unix types of syscall (Msghdr, Iovec)")
	}
	m, spkg := buildSrc(t, `package p

import (
	"io"
	"syscall"
	"unsafe"
)

type withPtr struct {
	P *int
	N int
}

func Larger() {
	var small syscall.RawSockaddrInet4
	big := (*syscall.RawSockaddrAny)(unsafe.Pointer(&small))
	big.Addr.Family = 1
}

func SrcPointer() {
	var h syscall.Msghdr // has *byte and *Iovec fields
	sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&h))
	sa.Port = 1
}

func DstPointer() {
	var sa syscall.RawSockaddrAny
	w := (*withPtr)(unsafe.Pointer(&sa))
	w.N = 1
}

func PastEnd() {
	b := make([]byte, 20)
	sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&b[8]))
	sa.Port = 1
}

func ReadOnly() {
	s := io.ErrClosedPipe.Error() // a string constant of the host: read-only memory
	sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(unsafe.StringData(s)))
	_ = sa.Port
}

func DstNativePointer() {
	var sa syscall.RawSockaddrAny
	h := (*syscall.Iovec)(unsafe.Pointer(&sa))
	h.Len = 1
}

// viaParam gets the pointer as an unsafe.Pointer: the SSA does not show the
// type of the value (viewOf).
func viaParam(p unsafe.Pointer) *syscall.RawSockaddrAny { return (*syscall.RawSockaddrAny)(p) }

func LargerParam() {
	var small syscall.RawSockaddrInet4
	big := viaParam(unsafe.Pointer(&small))
	big.Addr.Family = 1
}

func Unaligned() {
	b := make([]byte, 32)
	sa := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&b[1]))
	sa.Port = 1
}
`)
	m.realStd = true // the syscall types of the standard library as in Go
	for _, c := range []struct{ fn, want string }{
		{"Larger", "does not fit inside the value"},
		// before, the view kept the native type and the store panicked in
		// reflect (call of reflect.Value.Field on uint8 Value)
		{"LargerParam", "does not fit inside the value"},
		{"Unaligned", "not at an address aligned for it"},
		{"SrcPointer", "the native type syscall.Msghdr is not plain data"},
		{"DstPointer", "the type p.withPtr is not plain data"},
		{"DstNativePointer", "the type syscall.Iovec is not plain data"},
		{"PastEnd", "reaches outside the memory known to the VM"},
		{"ReadOnly", "read-only memory"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(errString(r), "unsupported") || !strings.Contains(errString(r), c.want) {
					t.Errorf("%s: panic %v, want unsupported with %q", c.fn, r, c.want)
				}
			}()
			m.Call(m.funcOf(spkg.Func(c.fn)), nil, nil)
		}()
	}
}
