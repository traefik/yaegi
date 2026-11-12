// Per-site caches of type assertions to interface types and of interface
// method calls (vm/exec.go: assertHit, invokeHit), and the lock-free
// implements cache. A site that sees several dynamic types, with and
// without the methods, must keep giving the right answer; so must the
// same sites run by several goroutines at once.
package main

import (
	"fmt"
	"sort"
	"sync"
)

type Shape interface{ Area() int }
type Named interface{ Name() string }

type Sq struct{ S int }
type Rect struct{ W, H int }
type Circ struct{ R int }

func (s Sq) Area() int       { return s.S * s.S }
func (s Sq) Name() string    { return "sq" }
func (r Rect) Area() int     { return r.W * r.H }
func (c *Circ) Area() int    { return 3 * c.R * c.R }
func (c *Circ) Name() string { return "circ" }

type Plain struct{ X int }

func kinds(vs []any) (areas, named, other int) {
	for _, v := range vs {
		if s, ok := v.(Shape); ok { // one site, four dynamic types
			areas += s.Area() // one invoke site, three dynamic types
		} else {
			other++
		}
		if n, ok := v.(Named); ok {
			named += len(n.Name())
		}
	}
	return
}

func main() {
	vs := []any{Sq{2}, Rect{2, 3}, &Circ{1}, Plain{4}, 7, "s", Sq{3}, &Circ{2}, nil, Rect{1, 1}}
	a, n, o := kinds(vs)
	fmt.Println(a, n, o)
	// The same sites, from several goroutines, each with its own order.
	var wg sync.WaitGroup
	results := make([]string, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			rot := append(append([]any{}, vs[g%len(vs):]...), vs[:g%len(vs)]...)
			total := 0
			for i := 0; i < 200; i++ {
				a, n, o := kinds(rot)
				total += a + n + o
			}
			results[g] = fmt.Sprint(total)
		}(g)
	}
	wg.Wait()
	sort.Strings(results)
	fmt.Println(results)
	// A site that only ever sees one type, then a nil interface.
	var s Shape
	for i := 0; i < 3; i++ {
		s = Sq{i}
		if _, ok := s.(Named); !ok {
			fmt.Println("missing Name")
		}
	}
	var e any
	_, ok := e.(Shape)
	fmt.Println(ok, s.Area())
}
