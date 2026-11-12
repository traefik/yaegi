package main

// Native code that appends in place into the capacity of an interpreted
// slice: h.Sum(m.ID[:0]) writes the hash into the [32]byte field, as Go
// does, even when the program does not use the result. wazero computes the
// ID of each module this way (wasm.Module.AssignModuleID); the field stayed
// zero in the VM, two modules got the same ID, and the second module ran the
// compiled code of the first.

import (
	"crypto/sha256"
	"fmt"
	"strconv"
)

type module struct {
	name string
	ID   [32]byte
}

func (m *module) assign(data []byte) {
	h := sha256.New()
	h.Write(data)
	m.ID[0] = 1
	h.Write(m.ID[:1])
	h.Sum(m.ID[:0])
}

func main() {
	a, b := &module{name: "a"}, &module{name: "b"}
	a.assign([]byte("first"))
	b.assign([]byte("second"))
	fmt.Printf("%x\n%x\n%v\n", a.ID[:8], b.ID[:8], a.ID == b.ID)

	// strconv.AppendInt into the capacity of a slice of an array field
	var buf struct{ b [8]byte }
	_ = strconv.AppendInt(buf.b[:0], 12345, 10)
	fmt.Println(string(buf.b[:5]))

	// a slice of int cells: copy and append in place by native code
	xs := make([]int, 2, 6)
	xs[0], xs[1] = 1, 2
	s := xs[:2]
	fmt.Println(len(s), cap(s), xs[:4])

	// only the capacity: the elements after it stay
	var arr [6]byte
	for i := range arr {
		arr[i] = 'z'
	}
	_ = strconv.AppendQuote(arr[1:1:4], "q")
	fmt.Println(string(arr[:]))
}
