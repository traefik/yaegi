package main

import (
	"fmt"
	"sync"
)

// Methods of sync.Mutex, sync.RWMutex and sync.WaitGroup called on every
// kind of receiver: struct field, embedded field, package variable, new(T),
// local variable and method value (GOROOT test fixedbugs/issue79186.go calls
// them in a loop: they take a direct path instead of reflect).

type counter struct {
	mu sync.RWMutex
	m  map[int]int
}

func (c *counter) get(k int) (int, bool) {
	c.mu.RLock()
	v, ok := c.m[k]
	c.mu.RUnlock()
	return v, ok
}

func (c *counter) inc(k int) {
	c.mu.Lock()
	c.m[k]++
	c.mu.Unlock()
}

type embedded struct {
	sync.Mutex
	n int
}

var global sync.Mutex
var globalN int

func main() {
	c := &counter{m: map[int]int{}}
	e := &embedded{}
	p := new(sync.Mutex)
	pn := 0
	var local sync.Mutex
	localN := 0
	lock, unlock := local.Lock, local.Unlock

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				c.inc(i & 3)
				_, _ = c.get(i & 3)
				e.Lock()
				e.n++
				e.Unlock()
				global.Lock()
				globalN++
				global.Unlock()
				p.Lock()
				pn++
				p.Unlock()
				lock()
				localN++
				unlock()
			}
		}(g)
	}
	wg.Wait()
	v, ok := c.get(0)
	fmt.Println(v, ok, c.m[1]+c.m[2]+c.m[3], e.n, globalN, pn, localN)

	// TryLock and the WaitGroup through a pointer.
	fmt.Println(global.TryLock(), global.TryLock())
	global.Unlock()
	wp := &sync.WaitGroup{}
	wp.Add(2)
	go wp.Done()
	go wp.Done()
	wp.Wait()
	fmt.Println("waited")
}
