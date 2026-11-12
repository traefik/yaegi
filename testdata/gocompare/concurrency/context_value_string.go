// String of the contexts made by context.WithValue: keys and values are
// shown with their String method, as a string, or by the name of their type.
package main

import (
	"context"
	"fmt"
	"time"
)

type key string

type named struct{ n int }

type strKey struct{ s string }

func (k strKey) String() string { return "strKey(" + k.s + ")" }

type ptrStr struct{}

func (*ptrStr) String() string { return "ptrStr" }

func main() {
	bg := context.Background()
	fmt.Println(context.WithValue(bg, key("s"), "v"))
	fmt.Println(context.WithValue(bg, 3, 4))
	fmt.Println(context.WithValue(bg, "k", nil))
	fmt.Println(context.WithValue(bg, named{1}, []int{1}))
	fmt.Println(context.WithValue(bg, strKey{"a"}, strKey{"b"}))
	fmt.Println(context.WithValue(bg, &ptrStr{}, ptrStr{}))
	fmt.Println(context.WithValue(bg, &named{}, time.Second))
	ctx := context.WithValue(context.WithValue(context.TODO(), key("a"), 1), key("b"), 2)
	fmt.Println(ctx)
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fmt.Println(context.WithValue(cctx, key("c"), "x"))
}
