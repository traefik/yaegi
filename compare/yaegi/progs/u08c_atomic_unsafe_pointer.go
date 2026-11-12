package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

type Config struct{ Name string }

func main() {
	var up unsafe.Pointer
	atomic.StorePointer(&up, unsafe.Pointer(&Config{"raw"}))
	fmt.Println((*Config)(atomic.LoadPointer(&up)).Name)
	fmt.Println(atomic.CompareAndSwapPointer(&up, up, unsafe.Pointer(&Config{"new"})), (*Config)(up).Name)
}
