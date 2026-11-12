package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var n int64
	var u uint32
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				atomic.AddInt64(&n, 1)
				atomic.AddUint32(&u, 1)
			}
		}()
	}
	wg.Wait()
	fmt.Println(atomic.LoadInt64(&n), atomic.LoadUint32(&u))
}
