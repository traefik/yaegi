package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type Config struct {
	Name string
	N    int
}

func main() {
	var p atomic.Pointer[Config]
	fmt.Println(p.Load() == nil)
	p.Store(&Config{"a", 1})
	old := p.Swap(&Config{"b", 2})
	fmt.Println(old.Name, p.Load().Name)
	c := p.Load()
	fmt.Println(p.CompareAndSwap(c, &Config{"c", 3}), p.Load().Name)

	var v atomic.Value
	v.Store(Config{"v", 4})
	fmt.Println(v.Load().(Config).N)

	var n atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n.Add(2) }()
	}
	wg.Wait()
	fmt.Println(n.Load())

	var up unsafe.Pointer
	atomic.StorePointer(&up, unsafe.Pointer(&Config{"raw", 5}))
	fmt.Println((*Config)(atomic.LoadPointer(&up)).Name)
}
