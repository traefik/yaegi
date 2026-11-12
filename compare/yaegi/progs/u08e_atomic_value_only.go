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
	var v atomic.Value
	v.Store(Config{"v", 4})
	fmt.Println(v.Load().(Config).N)
	v.Store(Config{"w", 5})
	c, ok := v.Load().(Config)
	fmt.Println(c.Name, ok)
}
