package vm

import (
	"context"
	"fmt"
	"testing"
)

// dotlessModule is a module whose path has no dot ("test", as the GOROOT
// tests of rundir use), with several packages: a, its internal package c,
// b that imports both, a plugin package and a main package. The go command
// takes these packages from the main module, not from the standard library
// (there is no GOROOT/src/test). yaegi v2 took them for standard library
// packages ("no dot in the first path element") and called them natively:
// "native func test/a.F not in registry".
var dotlessModule = map[string]string{
	"go.mod": "module test\n\ngo 1.22\n",
	"a/a.go": `package a

var X = 41

type T struct{ N int }

func (t T) Get() int { return t.N }

func F() int { return X + 1 }

func Max[E int | string](x, y E) E {
	if x > y {
		return x
	}
	return y
}
`,
	"internal/c/c.go": `package c

func Twice(n int) int { return 2 * n }
`,
	"b/b.go": `package b

import (
	"test/a"
	"test/internal/c"
)

func Sum() int { return a.F() + a.T{N: 3}.Get() + c.Twice(a.Max(2, 5)) }
`,
	"plug/plug.go": `package plug

import "test/b"

func Sum() int { return b.Sum() }
`,
	"main.go": `package main

import "test/b"

func main() {
	if got := b.Sum(); got != 55 {
		panic(got)
	}
}
`,
}

// TestDotlessModuleRunMain runs a program of a module "test" with RunMain
// (go list), with and without AOT and the restrictions of plugins.
func TestDotlessModuleRunMain(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, dotlessModule)
	for _, aot := range []bool{false, true} {
		for _, restricted := range []bool{false, true} {
			if _, err := RunMain(RunOptions{Dir: dir, AOT: aot, Restricted: restricted}, "."); err != nil {
				t.Errorf("aot=%v restricted=%v: %v", aot, restricted, err)
			}
		}
	}
}

// TestDotlessModuleLoad loads a plugin package of a module "test" with the
// default loader (no toolchain) and with go list, with and without AOT.
func TestDotlessModuleLoad(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, dotlessModule)
	for _, goList := range []bool{false, true} {
		for _, aot := range []bool{false, true} {
			name := fmt.Sprintf("goList=%v aot=%v", goList, aot)
			p, err := Load(Options{Dir: dir, GoList: goList, AOT: aot}, "test/plug")
			if err != nil {
				t.Errorf("%s: load: %v", name, err)
				continue
			}
			res, err := p.Call(context.Background(), "Sum")
			if err != nil {
				t.Errorf("%s: call: %v", name, err)
				continue
			}
			if v, err := res[0].Interface(); err != nil || v != 55 {
				t.Errorf("%s: Sum() = %v, %v, want 55", name, v, err)
			}
		}
	}
}
