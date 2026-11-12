// Package flagforgeplug writes the flag field of a reflect.Value with
// unsafe, as go-spew's bypass.go does: it clears the read-only bits and sets
// the addressable bit. On a value of the plugin the bypass works as in gc.
// On a value of a type of the host, and on a byte of a string in read-only
// memory, it must give nothing more than before: the host decides.
package flagforgeplug

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unsafe"
)

const (
	flagRO    = 1<<5 | 1<<6
	flagAddr  = 1 << 8
	flagROMem = 1 << 10 // a bit of the VM, not of gc
)

var flagOff = func() uintptr {
	f, ok := reflect.TypeOf(reflect.Value{}).FieldByName("flag")
	if !ok {
		panic("reflect.Value has no flag field")
	}
	return f.Offset
}()

// forge clears the read-only bits of v and sets its addressable bit.
func forge(v reflect.Value) reflect.Value {
	p := (*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&v)) + flagOff))
	*p &^= flagRO | flagROMem
	*p |= flagAddr
	return v
}

// try runs f and returns "ok" or the panic.
func try(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = "panic"
		}
	}()
	f()
	return "ok"
}

type own struct{ secret string }

// Probe returns what the bypass gives: on a value of the plugin, on the
// unexported field of a bytes.Buffer of the host (by value and through a
// pointer), and on a byte of a string of the host in read-only memory.
func Probe() string {
	var out []string
	add := func(name string, v reflect.Value) {
		out = append(out, fmt.Sprintf("%s: %v %v %v %s", name, v.CanInterface(), v.CanAddr(), v.CanSet(),
			try(func() { _ = v.Interface() })))
	}

	mine := forge(reflect.ValueOf(own{"s"}).Field(0))
	add("own", mine)

	b := bytes.NewBufferString("host")
	fv := forge(reflect.ValueOf(*b).Field(0))
	add("field", fv)
	out = append(out, "set field: "+try(func() { fv.SetBytes([]byte("X")) }))
	pv := forge(reflect.ValueOf(b).Elem().Field(0))
	add("ptr field", pv)
	out = append(out, "set ptr field: "+try(func() { pv.Set(reflect.ValueOf([]byte("Y"))) }))
	out = append(out, "addr: "+try(func() { pv.Addr() }))
	out = append(out, "buffer: "+b.String())

	// A string of the host that lives in read-only memory (a literal of
	// net/http): a write there would stop the process.
	s := http.StatusText(http.StatusOK)
	rv := forge(reflect.ValueOf(unsafe.StringData(s)).Elem())
	out = append(out, "byte: "+try(func() { rv.SetUint('X') })+" "+s+" "+http.StatusText(http.StatusOK))
	return strings.Join(out, "; ")
}
