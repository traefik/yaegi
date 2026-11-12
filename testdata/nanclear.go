package main

import (
	"fmt"
	"math"
	"reflect"
)

// reflect.Value.Clear on a map during a range over it stops the loop, also
// for NaN keys (the VM counts clears in MapV.clear). Run with -aot: the
// interpreted reflect works on the VM map itself.

func main() {
	m := map[float64]int{}
	for i := 0; i < 4; i++ {
		m[math.NaN()] = i
	}
	n := 0
	for range m {
		n++
		reflect.ValueOf(m).Clear()
	}
	fmt.Println(n, len(m))

	// a range that does not clear sees every NaN key
	for i := 0; i < 3; i++ {
		m[math.NaN()] = i
	}
	n = 0
	for range m {
		n++
		_ = reflect.ValueOf(m).Len()
	}
	fmt.Println(n, len(m))
}
