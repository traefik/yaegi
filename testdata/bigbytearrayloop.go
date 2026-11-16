package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// reflect walks a big [n]byte in real bytes (bigByteArray in byteview.go)
// element by element: Len and Index must not decode the whole array at
// each call. They did, and DeepEqual on two *[1 << 20]byte, a loop of
// Index or json.Marshal of the array were quadratic (hours).

func main() {
	start := time.Now()

	p, q := new([1 << 20]byte), new([1 << 20]byte)
	p[12345], q[12345] = 7, 7
	fmt.Println("deepequal:", reflect.DeepEqual(p, q))
	q[1<<20-1] = 1
	fmt.Println("deepequal last:", reflect.DeepEqual(p, q))

	rv := reflect.ValueOf(p).Elem()
	sum := uint64(0)
	for i := 0; i < rv.Len(); i++ {
		sum += rv.Index(i).Uint()
	}
	fmt.Println("index loop:", sum, rv.Len(), rv.Cap())

	small := new([1 << 16]byte)
	small[100] = 200
	j, err := json.Marshal(small)
	fmt.Println("json:", len(j), err, string(j[:12]))
	var back [1 << 16]byte
	err = json.Unmarshal(j, &back)
	fmt.Println("json back:", err, back[100], back == *small)

	fmt.Println("fast:", time.Since(start) < 15*time.Second)
}
