package main

import (
	"fmt"
	"sync"
)

func (p Pool) Get() *Buffer { return &Buffer{} }

func NewPool() Pool { return Pool{} }

type Buffer struct {
	bs   []byte
	pool Pool
}

type Pool struct {
	p *sync.Pool
}

var (
	_pool = NewPool()
	Get   = _pool.Get
)

func main() {
	fmt.Println(_pool)
	fmt.Println(Get())
}

// Output:
// {<nil>}
// &{[] {<nil>}}
