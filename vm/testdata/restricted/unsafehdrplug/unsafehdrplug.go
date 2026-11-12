// Package unsafehdrplug uses unsafe (useUnsafe) to forge string and slice
// headers that reach outside their memory. The VM must refuse each one:
// a plugin cannot read or write host memory through a forged header.
package unsafehdrplug

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"unsafe"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// far is an offset far outside any allocation, and hostAddr an address in
// the Go heap of the host: 1<<40 and 0xc000010000 on 64-bit platforms,
// 1<<24 and 0xc0000100 on 32-bit ones (where the 64-bit values overflow
// uintptr).
const (
	far      = uintptr(1) << (strconv.IntSize/2 + 8)
	hostAddr = uintptr(0xc0000100) << ((strconv.IntSize - 32) / 4)
)

// withPtr and withStr are not plain data: a view of bytes must never
// carry these types (native code would follow a pointer, or a string
// header, made of bytes the plugin wrote). pair is plain data.
type withPtr struct {
	N int
	P *int
}

type withStr struct {
	N int
	S string
}

type pair struct{ A, B uint32 }

// asPtr, asStr and asPair hide the source type from the SSA: the
// conversion sees an unsafe.Pointer of unknown origin.
func asPtr(p unsafe.Pointer) *withPtr { return (*withPtr)(p) }
func asStr(p unsafe.Pointer) *withStr { return (*withStr)(p) }
func asPair(p unsafe.Pointer) *pair   { return (*pair)(p) }

var attacks = map[string]func() any{
	// the zero-copy idioms stay allowed
	"/ok": func() any {
		b := make([]byte, 5)
		copy(b, "hello")
		s := *(*string)(unsafe.Pointer(&b))
		b[0] = 'J'
		return s + " " + unsafe.String(unsafe.SliceData(b), len(b))
	},
	// A view of plain data over a Go []byte (byteview.go), retyped
	// through an unsafe.Pointer of unknown origin to a struct with a
	// pointer field, then given to native code (json): refused at the
	// conversion. Without the check, json followed the bytes as a
	// pointer (a fault that ends the process).
	"/viewptr": func() any {
		b := make([]byte, 64)
		w := (*uint64)(unsafe.Pointer(&b[0]))
		*w = 0xc000010000
		*(*uint64)(unsafe.Pointer(&b[8])) = 0xc000010000 // the bytes of the pointer field
		out, err := json.Marshal(asPtr(unsafe.Pointer(w)))
		return string(out) + fmt.Sprint(err)
	},
	// The same with a string field: a load would read a string header
	// out of the bytes.
	"/viewstr": func() any {
		b := make([]byte, 64)
		return asStr(unsafe.Pointer((*uint64)(unsafe.Pointer(&b[0])))).S
	},
	// A view retyped to a type of the same size that is not plain data
	// (the layouts agree, the SSA shows both types).
	"/viewnative": func() any {
		b := make([]byte, 64)
		w := (*uint64)(unsafe.Pointer(&b[0]))
		return *(*unsafe.Pointer)(unsafe.Pointer(w)) == nil
	},
	// The type word of an interface read as a pointer to a runtime type:
	// there is no type data behind it in the VM.
	"/typeword": func() any {
		var v any = 3
		e := (*struct {
			typ  *struct{ size, ptrdata uintptr }
			data unsafe.Pointer
		})(unsafe.Pointer(&v))
		return e.typ.size
	},
	// The words of an interface built again: allowed.
	"/ifacewords": func() any {
		var v, w any = "ab", nil
		vs := (*[2]unsafe.Pointer)(unsafe.Pointer(&v))
		ws := (*[2]unsafe.Pointer)(unsafe.Pointer(&w))
		ws[0], ws[1] = vs[0], vs[1]
		return w
	},
	// A type word made from an integer: refused.
	"/forgedtype": func() any {
		var w any
		ws := (*[2]uintptr)(unsafe.Pointer(&w))
		ws[0], ws[1] = 0x10000, 0x10000
		return w
	},
	// Plain data retyped through an unknown origin stays allowed, and
	// native code gets the real bytes.
	"/viewok": func() any {
		b := make([]byte, 64)
		w := (*uint64)(unsafe.Pointer(&b[0]))
		p := asPair(unsafe.Pointer(w))
		p.A, p.B = 5, 7
		out, _ := json.Marshal(p)
		return string(out) + " " + fmt.Sprint(*w != 0)
	},
	// Strings of the host in read-only memory (constants): a write would
	// stop the process with a fault. A []byte view of them is a copy, and
	// a write through their data pointer is refused.
	"/rodata": func() any {
		s, t := strconv.Itoa(7), http.StatusText(200)
		b := *(*[]byte)(unsafe.Pointer(&s))
		b[0] = 'x'
		sh := (*reflect.StringHeader)(unsafe.Pointer(&t))
		c := unsafe.Slice((*byte)(unsafe.Pointer(sh.Data)), sh.Len)
		c[0] = 'x'
		d := unsafe.Slice(unsafe.StringData(t), len(t))
		d[1] = 'x'
		return s + " " + t + " " + unsafe.String(unsafe.StringData(t), sh.Len) + " " + string(b) + string(c) + string(d)
	},
	"/roreflect": func() any {
		t := http.StatusText(200)
		reflect.ValueOf(unsafe.StringData(t)).Elem().SetUint(65)
		return t
	},
	// native code (encoding/binary) would write through the pointer
	"/ronative": func() any {
		t := http.StatusText(200)
		_ = binary.Read(bytes.NewReader([]byte{'x'}), binary.LittleEndian, unsafe.StringData(t))
		return t
	},
	"/rowrite": func() any {
		t := http.StatusText(200)
		*unsafe.StringData(t) = 'x'
		return t
	},
	"/read": func() any {
		s := fmt.Sprint("hello")
		sh := *(*reflect.StringHeader)(unsafe.Pointer(&s))
		return sh.Len
	},
	"/cap": func() any {
		b := make([]byte, 4)
		h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
		h.Cap = 1 << 20
		h.Len = 1 << 20
		return len(b)
	},
	"/datafirst": func() any {
		// the data moves first (allowed while the old length and capacity
		// reach outside), then the length is written outside the array
		b := make([]byte, 8, 16)
		h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
		h.Data += 2
		h.Len = 1 << 20
		return len(b)
	},
	"/strlen": func() any {
		s := fmt.Sprint("abc")
		h := (*reflect.StringHeader)(unsafe.Pointer(&s))
		h.Len = 4096
		return len(s)
	},
	"/data": func() any {
		var b []byte
		h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
		h.Data = 0x10000
		h.Len, h.Cap = 8, 8
		return b[0]
	},
	"/dataoff": func() any {
		b := make([]byte, 4)
		h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
		h.Data = uintptr(unsafe.Pointer(&b[0])) + far
		return b[0]
	},
	"/built": func() any {
		hdr := reflect.SliceHeader{Data: 0x10000, Len: 8, Cap: 8}
		return (*(*[]byte)(unsafe.Pointer(&hdr)))[0]
	},
	"/slice": func() any {
		b := make([]byte, 4)
		return len(unsafe.Slice(&b[0], 1<<20))
	},
	"/string": func() any {
		b := make([]byte, 4)
		return len(unsafe.String(&b[0], 1<<20))
	},
	"/cells": func() any {
		x := []int{1, 2, 3}
		return len(unsafe.Slice(&x[1], 3))
	},
	"/add": func() any {
		b := make([]byte, 4)
		return *(*byte)(unsafe.Add(unsafe.Pointer(&b[0]), 4096))
	},
	"/deref": func() any {
		return *(*int)(unsafe.Pointer(hostAddr))
	},
	"/native": func() any {
		return fmt.Sprint(unsafe.Pointer(hostAddr))
	},
}

func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := attacks[r.URL.Path]
		if f == nil {
			next.ServeHTTP(w, r)
			return
		}
		defer func() {
			if e := recover(); e != nil {
				fmt.Fprint(w, "refused: ", e)
			}
		}()
		fmt.Fprint(w, "allowed: ", f())
	}), nil
}
