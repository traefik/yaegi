package method38

import "sync"

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
