package main

// A zero-size value of a plugin type (type Empty struct{}, [0]int) given to
// native code in an interface comes back with its type. Its native copy is
// a struct {} that other zero-size types share, so it was not remembered
// (rememberValue): sync.Pool's New returning Empty{} gave a struct {} to
// Get, and Get().(Empty) failed with no error. Such a value is now an
// opaque handle, one per type, so values of the same type stay equal and
// values of two types do not. (TODO.md section 1: "a zero-size value still
// comes back as struct {}".)
//
// An unnamed struct (struct{}{}, struct{ X int }{1}) came back with its
// native type, so a type assertion of the program on it failed: it now
// becomes a value of the program's type, as unnamed slices and arrays.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type Empty struct{}

func (Empty) String() string { return "empty!" }

type Empty2 struct{}

type Arr [0]int

type Gen[T any] struct{ _ [0]T }

type key struct{}

func main() {
	p := sync.Pool{New: func() any { return Empty{} }}
	v := p.Get()
	_, ok := v.(Empty)
	fmt.Printf("%T %v %v %v\n", v, ok, v == any(Empty{}), v == any(Empty2{}))
	fmt.Println(v, fmt.Sprint(v), fmt.Sprintf("%v %+v %#v", v, v, v))

	for _, f := range []func() any{
		func() any { return Empty2{} },
		func() any { return Arr{} },
		func() any { return Gen[int]{} },
		func() any { return struct{}{} },
	} {
		p := sync.Pool{New: f}
		v := p.Get()
		_, e2 := v.(Empty2)
		_, a := v.(Arr)
		_, g := v.(Gen[int])
		_, s := v.(struct{})
		fmt.Printf("%T %v %v %v %v\n", v, e2, a, g, s)
	}

	// as a map key of native code, and compared by native code
	var m sync.Map
	m.Store(Empty{}, 1)
	m.Store(Empty2{}, 2)
	m.Store(Empty{}, 3)
	a, _ := m.Load(Empty{})
	b, _ := m.Load(Empty2{})
	c, okc := m.Load(struct{}{})
	n := 0
	m.Range(func(k, v any) bool { n++; return true })
	fmt.Println(n, a, b, c, okc)
	fmt.Println(m.CompareAndSwap(Empty{}, 3, 4), m.CompareAndDelete(Empty2{}, 2))

	var av atomic.Value
	av.Store(Arr{})
	_, ok = av.Load().(Arr)
	fmt.Println(ok, av.CompareAndSwap(Arr{}, Arr{}))

	ctx := context.WithValue(context.Background(), key{}, Empty{})
	_, ok = ctx.Value(key{}).(Empty)
	fmt.Println(ok, ctx.Value(struct{}{}))

	// printers and json see the value
	out, err := json.Marshal(map[string]any{"e": v, "a": Arr{}})
	fmt.Println(string(out), err)

	unnamed()
}

type pair = struct {
	S []string
	E Empty
}

func unnamed() {
	for _, f := range []func() any{
		func() any { return struct{}{} },
		func() any { return struct{ X int }{1} },
		func() any { return pair{[]string{"a"}, Empty{}} },
		func() any { return [2]int{1, 2} },
		func() any { return []int{1} },
		func() any { return map[string]int{"a": 1} },
	} {
		p := sync.Pool{New: f}
		v := p.Get()
		_, a := v.(struct{})
		_, b := v.(struct{ X int })
		_, c := v.([2]int)
		_, d := v.([]int)
		_, e := v.(map[string]int)
		s, g := v.(pair)
		fmt.Printf("%T %v %v %v %v %v %v %v\n", v, a, b, c, d, e, g, s.S)
	}
	var m sync.Map
	m.Store("k", struct{ X int }{1})
	v, _ := m.Load("k")
	x, ok := v.(struct{ X int })
	fmt.Println(x.X, ok)
}
