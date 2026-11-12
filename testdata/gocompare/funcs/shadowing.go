// Scopes and shadowing: package names and builtins shadowed by locals,
// variables shadowed in nested blocks, if/for/switch scopes, := with one new
// variable re-using an old one, closures seeing the right variable, and
// shadowed named results.
package main

import (
	"fmt"
	"strings"
)

var x = "package x"

func usesPackageX() string { return x }

func shadowResult() (err error) {
	if true {
		err := fmt.Errorf("inner err")
		_ = err
	}
	return
}

func redecl() (int, int) {
	a, b := 1, 2
	a, c := 10, 3 // a reused, c new
	return a + c, b
}

func main() {
	fmt.Println(x, usesPackageX())
	x := "local x"
	fmt.Println(x, usesPackageX())
	{
		x := 42
		fmt.Println("block x", x)
		{
			x := []string{"deep"}
			fmt.Println("deeper x", x)
		}
		x++
		fmt.Println("block x after", x)
	}
	fmt.Println(x)
	for x := 0; x < 2; x++ {
		x := x * 10
		fmt.Println("for body x", x)
	}
	switch x := 5; x {
	case 5:
		x := "case x"
		fmt.Println(x)
	}
	fmt.Println(x)
	{
		len := func(s string) int { return 100 }
		fmt.Println("shadowed len", len("ab"))
	}
	fmt.Println("builtin len", len("ab"))
	{
		true := false
		fmt.Println("shadowed true", true)
	}
	{
		strings := []string{"not", "the", "package"}
		fmt.Println(len(strings), strings[2])
	}
	fmt.Println(strings.ToUpper("pkg ok"))
	{
		type int = string
		var v int = "int is a string here"
		fmt.Println(v)
	}
	{
		nil := 5
		fmt.Println("nil is", nil)
	}
	fmt.Println(shadowResult())
	fmt.Println(redecl())
	y := 1
	f := func() int { return y }
	{
		y := 2
		_ = y
		fmt.Println("closure sees outer y", f())
	}
	y = 3
	fmt.Println("closure after change", f())
	err := fmt.Errorf("outer")
	if _, err := fmt.Sscan("12", new(int)); err == nil {
		fmt.Println("inner err nil")
	}
	fmt.Println(err)
	var n int
	for i := 0; i < 3; i++ {
		n, i := n+i, i // shadow both
		_ = i
		fmt.Print(n, " ")
	}
	fmt.Println("n", n)
	type T struct{ x int }
	t := T{1}
	{
		t := T{2}
		t.x++
		fmt.Println(t)
	}
	fmt.Println(t)
	print := func(a ...interface{}) { fmt.Println(append([]interface{}{"custom print:"}, a...)...) }
	print(1, 2)
	const c = 1
	{
		const c = "two"
		fmt.Println(c)
	}
	fmt.Println(c)
	const (
		k0 = iota * 10
		k1
	)
	{
		iota := 9
		fmt.Println(iota, k0, k1)
	}
}
