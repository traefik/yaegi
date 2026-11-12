package main

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"unsafe"
)

// Reading the bytes of strings that live in read-only memory (constants of
// the host, of the standard library) through unsafe works as in gc. The VM
// refuses only writes there (TestUnsafeHeaders), which stop a gc program.

func s2b(s string) []byte { return *(*[]byte)(unsafe.Pointer(&s)) }

func main() {
	for _, s := range []string{strconv.Itoa(7), http.StatusText(200), http.StatusText(404), "literal"} {
		sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
		p := unsafe.StringData(s)
		b := s2b(s)
		v := unsafe.Slice(p, len(s))
		w := unsafe.Slice((*byte)(unsafe.Pointer(sh.Data)), sh.Len)
		fmt.Println(s, sh.Len, *p, string(b), len(b), string(v), string(w),
			unsafe.String(p, len(s)) == s, sh.Data == uintptr(unsafe.Pointer(p)),
			unsafe.Pointer(sh.Data) == unsafe.Pointer(p))
		sh.Len = 1
		fmt.Println(s)
	}
	// reflect reads them too
	t := http.StatusText(200)
	e := reflect.ValueOf(unsafe.StringData(t)).Elem()
	fmt.Println(e.Uint(), e.Kind(), e.Interface())
}
