// An interpreted struct with an embedded unexported field, stored by native
// code (sync.Pool, sync.Map, a context). reflect.StructOf cannot build its
// native counterpart: the VM gave the panic of reflect to the program
// ("field is anonymous but has PkgPath set"). Now native code gets the VM
// value and gives it back intact. The Encoder of encoding/json/jsontext
// (Go 1.27) is such a struct, in a sync.Pool.
package main

import (
	"context"
	"fmt"
	"sync"
)

type state struct{ n int }

type Encoder struct {
	state
	Name string
}

type withPtr struct {
	*state
	Name string
}

type key int

var pool = sync.Pool{New: func() any { return new(Encoder) }}

func main() {
	e := pool.Get().(*Encoder)
	e.n = 3
	e.Name = "pool"
	pool.Put(e)
	fmt.Println("pool:", e.n, e.Name)

	var m sync.Map
	v := Encoder{state{4}, "map"}
	m.Store("value", v)
	v.n = 40 // the stored value is a copy
	got, ok := m.Load("value")
	fmt.Println("sync.Map value:", ok, got.(Encoder).n, got.(Encoder).Name)

	m.Store("ptr", withPtr{&state{5}, "ptr"})
	gp, _ := m.Load("ptr")
	fmt.Println("sync.Map embedded pointer:", gp.(withPtr).n, gp.(withPtr).Name)

	ctx := context.WithValue(context.Background(), key(1), Encoder{state{6}, "ctx"})
	c := ctx.Value(key(1)).(Encoder)
	fmt.Println("context:", c.n, c.Name)
	fmt.Printf("%T %T\n", ctx.Value(key(1)), gp)
}
