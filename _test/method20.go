package main

import (
	"fmt"
	"sync"
)

type Hello struct {
	mu sync.Mutex
}

func (h *Hello) Hi() string {
	h.mu.Lock()
	h.mu.Unlock()
	return "hi"
}

func main() {
	a := &Hello{}

	fmt.Println(a.Hi())
}

// Output:
// hi
