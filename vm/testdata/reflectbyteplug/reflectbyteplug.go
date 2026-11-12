// Package reflectbyteplug reads the same reflect.Value of a byte of a Go
// []byte from several goroutines at the same time. In gc a read of a
// reflect.Value is a read of memory: goroutines can share the value. The VM
// reflect must not write a shared cell on each read (a race).
package reflectbyteplug

import "reflect"

var b = []byte("hello")
var e = reflect.ValueOf(b).Index(1)

// Read returns the sum of 4000 concurrent reads of e, plus one read after
// a write into b: 4000*'e' + 7.
func Read() int {
	done := make(chan uint64)
	for range 4 {
		go func() {
			var s uint64
			for range 1000 {
				s += e.Uint()
			}
			done <- s
		}()
	}
	var sum uint64
	for range 4 {
		sum += <-done
	}
	b[1] = 7
	return int(sum) + int(e.Uint())
}
