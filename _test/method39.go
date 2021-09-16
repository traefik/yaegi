package main

import (
	"fmt"
	"sync"

	"github.com/traefik/yaegi/_test/method38"
)

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
	fmt.Println(Get())
}

// Error:
// 17:11: undefined selector Get
