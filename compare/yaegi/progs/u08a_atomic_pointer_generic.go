package main

import (
	"fmt"
	"sync/atomic"
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
}
