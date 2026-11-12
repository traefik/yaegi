// Package ctxaliasplug has a Context whose Value gives its own map, slice,
// struct with a pointer field, or pointer, wrapped by context.WithCancel:
// native code calls Value and gives the result back in an any. The result
// must alias the program's value as in Go: a write through it changes the
// program's value, and a later write by the program is seen through it. A
// plain struct or array of scalars is a copy in Go too.
package ctxaliasplug

import (
	"context"
	"fmt"
	"strings"
)

// box is unnamed: a named type of the program is never a copy.
type box = struct {
	P *int
	N int
}

type pair struct {
	A int
	B string
}

type myCtx struct {
	context.Context
	m map[string]string
	s []int
	b box
	p *pair
	v pair
	a [2]int
}

func (c *myCtx) Value(key any) any {
	switch key {
	case "m":
		return c.m
	case "s":
		return c.s
	case "b":
		return c.b
	case "p":
		return c.p
	case "v":
		return c.v
	case "a":
		return c.a
	}
	return c.Context.Value(key)
}

// Run returns what the program sees, one line per kind of value.
func Run() string {
	var out strings.Builder
	n := 1
	c := &myCtx{
		Context: context.Background(),
		m:       map[string]string{"a": "1"},
		s:       []int{1, 2},
		b:       box{P: &n, N: 1},
		p:       &pair{1, "x"},
		v:       pair{1, "x"},
		a:       [2]int{1, 2},
	}
	cc, cancel := context.WithCancel(c)
	defer cancel()

	m := cc.Value("m").(map[string]string)
	m["b"] = "2"
	c.m["c"] = "3"
	fmt.Fprintln(&out, "map:", c.m, m)

	s := cc.Value("s").([]int)
	s[0] = 10
	c.s[1] = 20
	fmt.Fprintln(&out, "slice:", c.s, s)

	b := cc.Value("b").(box)
	*b.P = 5
	b.N = 5
	fmt.Fprintln(&out, "struct with pointer:", n, *c.b.P, c.b.N)

	p := cc.Value("p").(*pair)
	p.A = 7
	c.p.B = "y"
	fmt.Fprintln(&out, "pointer:", *c.p, *p)

	v := cc.Value("v").(pair)
	v.A = 9
	fmt.Fprintln(&out, "plain struct:", c.v, v)

	a := cc.Value("a").([2]int)
	a[0] = 9
	fmt.Fprintln(&out, "array:", c.a, a)
	return out.String()
}
