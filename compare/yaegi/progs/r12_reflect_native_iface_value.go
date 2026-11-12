package main

import (
	"context"
	"fmt"
	"sync"
)

type key struct{}

type Session struct{ User string }

func (s *Session) Name() string { return s.User }

func main() {
	ctx := context.WithValue(context.Background(), key{}, &Session{"bob"})
	s := ctx.Value(key{}).(*Session)
	fmt.Println(s.Name())
	var m sync.Map
	m.Store(key{}, Session{"x"})
	v, _ := m.Load(key{})
	fmt.Println(v.(Session).User)
	p := sync.Pool{New: func() any { return &Session{"pool"} }}
	o := p.Get().(*Session)
	p.Put(o)
	fmt.Println(o.User)
}
