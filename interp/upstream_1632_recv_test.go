package interp

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/stdlib"
)

// TestUpstream1632RecvIntoBinaryPackageVar checks that a channel receive
// redirected to a binary package variable stores into the live host cell
// instead of frame index -1 (index out of range panic).
func TestUpstream1632RecvIntoBinaryPackageVar(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{"pkg/pkg": {
		"J": reflect.New(reflect.TypeOf("")).Elem(),
	}}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	if _, err := i.Eval(`package main

import "pkg"

func main() {
	ch := make(chan string, 1)
	ch <- "v"
	pkg.J = <-ch
}
`); err != nil {
		t.Fatal(err)
	}
	v, err := i.Eval("pkg.J")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "v" {
		t.Fatalf("got %q, want %q", got, "v")
	}
}

// TestUpstream1632SortSliceInPlace checks that in-place mutation contracts on
// slices passed to binary code (sort.Slice) are preserved: interface-element
// containers are passed untouched rather than copied.
func TestUpstream1632SortSliceInPlace(t *testing.T) {
	i := New(Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Eval(`package main

import (
	"fmt"
	"sort"
)

type S struct{ K int }

func (s S) String() string { return fmt.Sprint(s.K) }

func main() {
	ss := []fmt.Stringer{S{3}, S{1}, S{2}}
	sort.Slice(ss, func(i, j int) bool { return ss[i].(S).K < ss[j].(S).K })
	for _, x := range ss {
		fmt.Print(x, " ")
	}
	fmt.Println()
}
`); err != nil {
		t.Fatal(err)
	}
}
