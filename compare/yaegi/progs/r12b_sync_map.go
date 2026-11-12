package main

import (
	"fmt"
	"sync"
)

type Session struct{ User string }

func main() {
	var m sync.Map
	m.Store("k", Session{"x"})
	m.Store("p", &Session{"y"})
	v, _ := m.Load("k")
	fmt.Println(v.(Session).User)
	p, _ := m.Load("p")
	fmt.Println(p.(*Session).User)
	n := 0
	m.Range(func(k, v any) bool { n++; return true })
	fmt.Println("range", n)
}
