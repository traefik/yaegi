package main

import "sync"

type T struct {
	mu   sync.RWMutex
	name string
}

func (t *T) get() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.name
}

var d = T{name: "test"}

func main() {
	println(d.get())
}

// Output:
// test
