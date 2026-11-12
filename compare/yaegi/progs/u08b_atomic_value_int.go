package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Config struct {
	Name string
	N    int
}

func main() {
	var v atomic.Value
	v.Store(Config{"v", 4})
	fmt.Println(v.Load().(Config).N)
	var n atomic.Int64
	var old int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); n.Add(2); atomic.AddInt64(&old, 1) }()
	}
	wg.Wait()
	fmt.Println(n.Load(), atomic.LoadInt64(&old))
}
