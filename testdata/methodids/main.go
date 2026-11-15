// The unexported methods of different packages are different methods, even
// with the same name: a.Call(v) runs the method m of package a, and an
// interface of package main runs the m of package main (methods keyed by
// package path and name, as types.Id).
package main

import (
	"fmt"
	"strings"

	"github.com/traefik/yaegi/v2/testdata/methodids/a"
	"github.com/traefik/yaegi/v2/testdata/methodids/b"
)

type I interface{ m() string }

// Y embeds a.T and has its own m: the m of main.
type Y struct{ a.T }

func (Y) m() string { return "main.Y.m" }

// Both embeds a.T (through b.U) and b.U: the m of a and the m of b.
type Both struct {
	b.U
}

// Two embeds a.P and b.U at the same depth: a.m comes from *a.P, b.m
// from b.U.
type Two struct {
	*a.P
	b.U
}

// SB embeds a native type with an unexported method (copyCheck) and has
// its own copyCheck.
type SB struct{ *strings.Builder }

func (SB) copyCheck() string { return "main.SB.copyCheck" }

type checker interface{ copyCheck() string }

// reset is a common helper name.
type R struct{ a.T }

func (R) reset() string { return "main.R.reset" }

// Gen calls the method m of package main on a type parameter.
func Gen[X I](x X) string { return x.m() }

func main() {
	var i I = Y{}
	fmt.Println("main.I(Y):", i.m())
	fmt.Println("a.Call(Y):", a.Call(Y{}))
	fmt.Println("a.Call(&Y):", a.Call(&Y{}))
	f := i.m
	fmt.Println("main value:", f())
	fmt.Println("a.Value(Y):", a.Value(Y{})())
	fmt.Println("a.Defer(Y):", a.Defer(Y{}))
	fmt.Println("a.Go(Y):", a.Go(Y{}))
	fmt.Println("a.Expr(Y):", a.Expr(Y{}))
	fmt.Println("I.m(Y):", I.m(Y{}))
	fmt.Println("a.Switch(Y):", a.Switch(Y{}))
	fmt.Println("a.Gen(Y):", a.Gen(Y{}), a.Gen(&Y{}), a.Gen[a.I](Y{}))
	fmt.Println("Gen(Y):", Gen(Y{}), Gen[I](Y{}))

	fmt.Println("a.Call(b.U):", a.Call(b.U{}))
	fmt.Println("b.Call(b.U):", b.Call(b.U{}))
	fmt.Println("a.Value(b.U):", a.Value(b.U{})())
	fmt.Println("b.Value(b.U):", b.Value(b.U{})())

	fmt.Println("a.Call(Both):", a.Call(Both{}))
	fmt.Println("b.Call(Both):", b.Call(Both{}))
	fmt.Println("a.Switch(Both):", a.Switch(Both{}))

	two := Two{P: &a.P{N: 1}}
	fmt.Println("a.Call(Two):", a.Call(two))
	fmt.Println("a.Gen(Two):", a.Gen(two), a.Gen(&two))
	fmt.Println("b.Call(Two):", b.Call(two))
	fmt.Println("a.Go(Two):", a.Go(two))

	var v any = Y{}
	switch x := v.(type) {
	case I:
		fmt.Println("switch main.I:", x.m())
	}
	if x, ok := v.(a.I); ok {
		fmt.Println("assert a.I:", a.Call(x))
	}

	var c checker = SB{&strings.Builder{}}
	fmt.Println("checker(SB):", c.copyCheck())
	g := c.copyCheck
	fmt.Println("checker value:", g())

	var r interface{ reset() string } = R{}
	fmt.Println("R.reset:", r.reset(), a.Call(R{}))
}
