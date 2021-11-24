package main

type Pooler interface {
	Get() string
}

type baseClient struct {
	connPool Pooler
}

type connPool struct {
	name string
}

func (c *connPool) Get() string { return c.name }

func newBaseClient(i int, p Pooler) *baseClient {
	return &baseClient{connPool: p}
}

func newConnPool() *connPool { return &connPool{name: "connPool"} }

func main() {
	b := newBaseClient(0, newConnPool())
	println(b.connPool.(*connPool).name)
}

// Output:
// connPool
