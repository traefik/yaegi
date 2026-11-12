package main

import (
	"fmt"
	"sync"
)

func NewPool() Pool { return Pool{} }

type Pool struct {
	p *sync.Pool
}

var _pool = NewPool()

func main() {
	fmt.Println(_pool)
}

// Output:
// {<nil>}
