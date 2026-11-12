package main

// runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name() on interpreted
// functions (GOROOT/test fixedbugs/issue58300.go, issue58300b.go): the
// names of gc for plain functions, closures, method values (-fm), method
// expressions, promoted methods, generic functions and methods of generic
// types; and the *runtime.Func used through a method value, an interface
// and native code.

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"text/template"
)

type T struct{}

func (T) Value()    {}
func (*T) Pointer() {}

type U struct{ T }

type G[X any] struct{ x X }

func (g G[X]) Get() X   { return g.x }
func (g *G[X]) Set(x X) { g.x = x }

func gen[X any](x X) X { return x }

type namer interface{ Name() string }

func plain(n int) int { return n }

func name(fn any) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func main() {
	fmt.Println(name(plain))
	fmt.Println(name(T.Value))
	fmt.Println(name((*T).Pointer))
	inner := func() {}
	fmt.Println(name(inner))
	fmt.Println(reflect.ValueOf(plain).Pointer() == reflect.ValueOf(plain).Pointer())
	fmt.Println(name(strings.ToUpper))
	f := runtime.FuncForPC(reflect.ValueOf(plain).Pointer())
	fmt.Println(f.Entry() == reflect.ValueOf(plain).Pointer())

	// Synthetic wrappers and generics.
	var t T
	var u U
	g := G[int]{}
	fmt.Println(name(t.Value), name(t.Pointer), name(u.Value))
	fmt.Println(name((*T).Value), name(U.Value), name((*U).Pointer))
	fmt.Println(name(G[int].Get), name((*G[string]).Set), name(g.Get), name(gen[int]))
	var i interface{ Value() } = t
	fmt.Println(name(i.Value), name(interface{ Value() }.Value))

	// The *runtime.Func through a method value, an interface, and native
	// code that holds it (text/template calls its Name method).
	nm := f.Name
	fmt.Println(nm())
	var n namer = f
	fmt.Println(n.Name())
	file, line := f.FileLine(f.Entry())
	fmt.Println(strings.HasSuffix(file, "funcname.go"), line > 0)
	tmpl := template.Must(template.New("").Parse("{{.Name}}\n"))
	tmpl.Execute(os.Stdout, runtime.FuncForPC(reflect.ValueOf(gen[int]).Pointer()))
	names := []func() string{runtime.FuncForPC(reflect.ValueOf(inner).Pointer()).Name}
	fmt.Println(names[0]())
}
