package interp

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestUpstream1632BinaryPackageVar exercises the assignment forms of upstream
// issue #1632 against a variable exported by a host binary package. Every
// form must propagate: a later read (in any position) must observe the
// written value, and the host variable must reflect it.
func TestUpstream1632BinaryPackageVar(t *testing.T) {
	tests := []struct {
		name     string
		pre      string // setup statements run before stmt
		stmt     string // statements under test
		read     string // final read expression, its value must equal want
		want     int
		hostWant int // expected final host backing var value (defaults to want)
	}{
		{name: "assign literal", stmt: `pkg.J = 1`, read: `pkg.J`, want: 1},
		{name: "assign literal passed as call argument", stmt: `pkg.J = 2`, read: `f(pkg.J)`, want: 20, hostWant: 2},
		{name: "plus equal", stmt: `pkg.J += 1`, read: `pkg.J`, want: 1},
		{name: "increment", stmt: `pkg.J++`, read: `pkg.J`, want: 1},
		{name: "decrement", pre: `pkg.J = 5`, stmt: `pkg.J--`, read: `pkg.J`, want: 4},
		{name: "assign self plus", stmt: `pkg.J = pkg.J + 1`, read: `pkg.J`, want: 1},
		{name: "assign self double", pre: `pkg.J = 1`, stmt: `pkg.J = pkg.J + pkg.J`, read: `pkg.J`, want: 2},
		{name: "assign call result", stmt: `pkg.J = f(1)`, read: `pkg.J`, want: 10},
		{name: "assign call result from local", stmt: `k := 2; pkg.J = f(k)`, read: `pkg.J`, want: 20},
		{name: "assign through address", stmt: `*&pkg.J = 3`, read: `pkg.J`, want: 3},
		{name: "read in closure return", stmt: `pkg.J = 4; g := func() int { return pkg.J }`, read: `g()`, want: 4},
		{name: "read in if condition", stmt: `pkg.J = 5; if pkg.J != 5 { _ = 0 }`, read: `pkg.J`, want: 5},
		{name: "read in composite literal", stmt: `pkg.J = 6; s := []int{pkg.J}`, read: `s[0]`, want: 6},
		{name: "read in loop condition", stmt: `pkg.J = 3; n := 0; for i := 0; i < pkg.J; i++ { n++ }`, read: `n`, want: 3},
		{name: "double increment", stmt: `pkg.J++; pkg.J++`, read: `pkg.J`, want: 2},
		{name: "self assignment", pre: `pkg.J = 7`, stmt: `pkg.J = pkg.J`, read: `pkg.J`, want: 7},
		{name: "read and write mixed", stmt: `pkg.J = f(pkg.J) + 1`, read: `pkg.J`, want: 1},
		{name: "read in switch condition", stmt: `pkg.J = 8; switch pkg.J { case 8: default: }`, read: `pkg.J`, want: 8},
		{name: "read as builtin argument", stmt: `pkg.J = 9; a := make([]int, pkg.J)`, read: `len(a)`, want: 9},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			j := 0
			i := New(Options{})
			if err := i.Use(Exports{
				"pkg/pkg":     {"J": reflect.ValueOf(&j).Elem()},
				"up163/up163": {"F": reflect.ValueOf(func(i int) int { return i * 10 })},
			}); err != nil {
				t.Fatal(err)
			}
			i.ImportUsed()

			hostWant := test.want
			if test.hostWant != 0 {
				hostWant = test.hostWant
			}
			var b strings.Builder
			fmt.Fprintf(&b, `package main

func f(i int) int { return up163.F(i) }

func main() {
`)
			if test.pre != "" {
				fmt.Fprintf(&b, "\t%s\n", test.pre)
			}
			fmt.Fprintf(&b, "\t%s\n", test.stmt)
			fmt.Fprintf(&b, "\tv := %s\n", test.read)
			fmt.Fprintf(&b, "\tif v != %d {\n\t\tpanic(\"upstream 1632: stale package variable value\")\n\t}\n", test.want)
			b.WriteString("}\n")

			if _, err := i.Eval(b.String()); err != nil {
				t.Fatalf("Eval error: %v", err)
			}
			if j != hostWant {
				t.Fatalf("host variable = %d, want %d", j, hostWant)
			}
		})
	}
}

// TestUpstream1632InterpretedPackageVar exercises the same assignment forms
// with a fully interpreted imported package.
func TestUpstream1632InterpretedPackageVar(t *testing.T) {
	tests := []struct {
		name string
		pre  string
		stmt string
		read string
		want int
	}{
		{name: "assign literal", stmt: `pkg.J = 1`, read: `pkg.J`, want: 1},
		{name: "assign literal passed as call argument", stmt: `pkg.J = 2`, read: `f(pkg.J)`, want: 20},
		{name: "plus equal", stmt: `pkg.J += 1`, read: `pkg.J`, want: 1},
		{name: "increment", stmt: `pkg.J++`, read: `pkg.J`, want: 1},
		{name: "decrement", pre: `pkg.J = 5`, stmt: `pkg.J--`, read: `pkg.J`, want: 4},
		{name: "assign self plus", stmt: `pkg.J = pkg.J + 1`, read: `pkg.J`, want: 1},
		{name: "assign self double", pre: `pkg.J = 1`, stmt: `pkg.J = pkg.J + pkg.J`, read: `pkg.J`, want: 2},
		{name: "assign call result", stmt: `pkg.J = f(1)`, read: `pkg.J`, want: 10},
		{name: "assign call result from local", stmt: `k := 2; pkg.J = f(k)`, read: `pkg.J`, want: 20},
		{name: "assign through address", stmt: `*&pkg.J = 3`, read: `pkg.J`, want: 3},
		{name: "read in closure return", stmt: `pkg.J = 4; g := func() int { return pkg.J }`, read: `g()`, want: 4},
		{name: "read in if condition", stmt: `pkg.J = 5; if pkg.J != 5 { _ = 0 }`, read: `pkg.J`, want: 5},
		{name: "read in composite literal", stmt: `pkg.J = 6; s := []int{pkg.J}`, read: `s[0]`, want: 6},
		{name: "read in loop condition", stmt: `pkg.J = 3; n := 0; for i := 0; i < pkg.J; i++ { n++ }`, read: `n`, want: 3},
		{name: "double increment", stmt: `pkg.J++; pkg.J++`, read: `pkg.J`, want: 2},
		{name: "self assignment", pre: `pkg.J = 7`, stmt: `pkg.J = pkg.J`, read: `pkg.J`, want: 7},
		{name: "read and write mixed", stmt: `pkg.J = f(pkg.J) + 1`, read: `pkg.J`, want: 1},
		{name: "read in switch condition", stmt: `pkg.J = 8; switch pkg.J { case 8: default: }`, read: `pkg.J`, want: 8},
		{name: "read as builtin argument", stmt: `pkg.J = 9; a := make([]int, pkg.J)`, read: `len(a)`, want: 9},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			i := New(Options{})
			if _, err := i.Eval(`package pkg; var J int`); err != nil {
				t.Fatal(err)
			}

			var b strings.Builder
			b.WriteString(`package main

import "pkg"

func f(i int) int { return i * 10 }

func main() {
`)
			if test.pre != "" {
				fmt.Fprintf(&b, "\t%s\n", test.pre)
			}
			fmt.Fprintf(&b, "\t%s\n", test.stmt)
			fmt.Fprintf(&b, "\tv := %s\n", test.read)
			fmt.Fprintf(&b, "\tif v != %d {\n\t\tpanic(\"upstream 1632: stale package variable value\")\n\t}\n", test.want)
			b.WriteString("}\n")

			if _, err := i.Eval(b.String()); err != nil {
				t.Fatalf("Eval error: %v", err)
			}
		})
	}
}

// TestUpstream1632BinaryConstNotAssignable ensures a binary package constant
// stays read-only.
func TestUpstream1632BinaryConstNotAssignable(t *testing.T) {
	i := New(Options{})
	if err := i.Use(Exports{
		"pkg/pkg": {"C": reflect.ValueOf(42)},
	}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	_, err := i.Eval(`package main

func main() {
	pkg.C = 1
}
`)
	if err == nil {
		t.Fatal("assignment to binary package constant was accepted")
	}
}

// TestUpstream1632HostVarLiveInView ensures reads from the host side and from
// a later evaluation observe the interpreted writes.
func TestUpstream1632HostVarLiveInView(t *testing.T) {
	j := 0
	i := New(Options{})
	if err := i.Use(Exports{
		"pkg/pkg": {"J": reflect.ValueOf(&j).Elem()},
	}); err != nil {
		t.Fatal(err)
	}
	i.ImportUsed()
	if _, err := i.Eval(`package main

func set() { pkg.J = 11 }

func main() {
	set()
	if pkg.J != 11 {
		panic("stale read in main")
	}
}
`); err != nil {
		t.Fatal(err)
	}
	if j != 11 {
		t.Fatalf("host variable = %d, want 11", j)
	}
	if v, err := i.Eval(`pkg.J`); err != nil {
		t.Fatal(err)
	} else if v.Int() != 11 {
		t.Fatalf("Eval(\"pkg.J\") = %v, want 11", v)
	}
}
