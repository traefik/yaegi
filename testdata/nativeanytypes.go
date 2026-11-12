package main

// Values of plugin types that have no native counterpart (generic
// instantiations, local types, unexported types) go through native code
// as any (sync.Pool, atomic.Value, sync.Map, context, channels of any,
// container/list, a map[any]any, an interface field of a native struct)
// and come back. As in Go, they must come back with their type: a type
// assertion succeeds and a type switch finds the right case.

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"mime"
	"sync"
	"sync/atomic"
)

type Box[T any] struct {
	V T
	N string
}

type Pair[K comparable, V any] struct {
	K K
	V V
}

type plain struct {
	a int
	b string
}

type Named struct{ X int }

type key[T any] struct{}

func describe(v any) string {
	switch x := v.(type) {
	case Box[int]:
		return fmt.Sprintf("Box[int] %d %s", x.V, x.N)
	case Box[string]:
		return fmt.Sprintf("Box[string] %s %s", x.V, x.N)
	case *Box[int]:
		return fmt.Sprintf("*Box[int] %d", x.V)
	case Pair[string, int]:
		return fmt.Sprintf("Pair[string,int] %s %d", x.K, x.V)
	case plain:
		return fmt.Sprintf("plain %d %s", x.a, x.b)
	case Named:
		return fmt.Sprintf("Named %d", x.X)
	case []Box[int]:
		return fmt.Sprintf("[]Box[int] %d", len(x))
	case map[string]Box[int]:
		return fmt.Sprintf("map[string]Box[int] %d", len(x))
	case [2]Box[int]:
		return fmt.Sprintf("[2]Box[int] %d", x[1].V)
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("other %T", v)
	}
}

func main() {
	type local struct{ Z int }
	type localGen = Box[local]

	vals := []any{
		Box[int]{V: 1, N: "a"},
		Box[string]{V: "s", N: "b"},
		Pair[string, int]{K: "k", V: 2},
		plain{a: 3, b: "c"},
		Named{X: 4},
		[]Box[int]{{V: 5}},
		map[string]Box[int]{"x": {V: 6}},
		[2]Box[int]{{V: 7}, {V: 8}},
		local{Z: 9},
		localGen{V: local{Z: 10}},
	}
	check := func(where string, v any) {
		s := describe(v)
		if l, ok := v.(local); ok {
			s = fmt.Sprintf("local %d", l.Z)
		}
		if l, ok := v.(localGen); ok {
			s = fmt.Sprintf("Box[local] %d", l.V.Z)
		}
		fmt.Println(where, s)
	}

	// sync.Pool whose New returns values (not pointers).
	for _, v := range vals {
		v := v
		p := sync.Pool{New: func() any { return v }}
		check("pool.New", p.Get())
	}
	if b, ok := (&sync.Pool{New: func() any { return Box[int]{V: 42} }}).Get().(Box[int]); ok {
		fmt.Println("pool assert", b.V)
	} else {
		fmt.Println("pool assert failed")
	}

	// atomic.Value.
	for i, v := range vals {
		var a atomic.Value
		a.Store(v)
		check("atomic.Load", a.Load())
		check("atomic.Swap", a.Swap(v))
		if i != 5 && i != 6 { // a slice and a map are not comparable
			fmt.Println("atomic.CAS", a.CompareAndSwap(v, v))
		}
		check("atomic.Load2", a.Load())
	}

	// CompareAndSwap converts its new value (it is not only stored: the old
	// one is compared).
	for i, v := range vals {
		if i == 5 || i == 6 {
			continue
		}
		var a atomic.Value
		fmt.Println("atomic.CAS nil", a.CompareAndSwap(nil, v))
		check("atomic.CAS Load", a.Load())
	}

	// A type with a method that no wrapper knows: native code sees its
	// fields, the program gets its method back.
	mp := sync.Pool{New: func() any { return withMethod[int]{v: 14} }}
	if w, ok := mp.Get().(interface{ Get() int }); ok {
		fmt.Println("method back", w.Get())
	} else {
		fmt.Println("method lost")
	}

	// Values put into a pool and taken out (the pool may drop them).
	var vp sync.Pool
	vp.Put(Box[int]{V: 15})
	if v := vp.Get(); v != nil {
		if _, ok := v.(Box[int]); !ok {
			fmt.Println("pool Put/Get lost the type")
		}
	}

	// sync.Map.
	var m sync.Map
	for i, v := range vals {
		m.Store(i, v)
	}
	for i := range vals {
		v, _ := m.Load(i)
		check("sync.Map", v)
	}
	m.Store(Box[int]{V: 1}, "generic key")
	if v, ok := m.Load(Box[int]{V: 1}); ok {
		fmt.Println("sync.Map key", v)
	}
	m.Range(func(k, v any) bool {
		if b, ok := k.(Box[int]); ok {
			fmt.Println("sync.Map range key", b.V, v)
		}
		return true
	})
	act, loaded := m.LoadOrStore(100, Box[int]{V: 100})
	check("sync.Map LoadOrStore", act)
	fmt.Println(loaded)
	fmt.Println("sync.Map CAS", m.CompareAndSwap(100, Box[int]{V: 100}, Pair[string, int]{K: "n", V: 101}))
	prev, _ := m.Swap(100, Box[string]{V: "w"})
	check("sync.Map Swap", prev)
	prev, _ = m.LoadAndDelete(100)
	check("sync.Map LoadAndDelete", prev)

	// context.WithValue, with a generic key type.
	ctx := context.Background()
	for i, v := range vals {
		ctx = context.WithValue(ctx, i, v)
	}
	ctx = context.WithValue(ctx, key[int]{}, Box[int]{V: 11})
	for i := range vals {
		check("context", ctx.Value(i))
	}
	check("context generic key", ctx.Value(key[int]{}))
	fmt.Println("context other key", ctx.Value(key[string]{}))

	// Channels of any.
	ch := make(chan any, len(vals))
	for _, v := range vals {
		ch <- v
	}
	close(ch)
	for v := range ch {
		check("chan", v)
	}

	// container/list.
	l := list.New()
	for _, v := range vals {
		l.PushBack(v)
	}
	for e := l.Front(); e != nil; e = e.Next() {
		check("list", e.Value)
	}

	// map[any]any given to native code (fmt reads it) and kept.
	mm := map[any]any{}
	for i, v := range vals {
		mm[i] = v
	}
	_ = fmt.Sprint(len(mm))
	for i := range vals {
		check("map", mm[i])
	}

	// An interface field of a native struct.
	for _, v := range vals {
		e := &list.Element{Value: v}
		check("field", e.Value)
	}

	// errors.Join keeps its errors; an error of a generic type comes back.
	err := errors.Join(genErr[int]{v: 12})
	var ge genErr[int]
	fmt.Println("errors.As", errors.As(err, &ge), ge.v)

	// A pointer still keeps its identity.
	pb := &Box[int]{V: 13}
	var a atomic.Value
	a.Store(pb)
	fmt.Println("pointer identity", a.Load().(*Box[int]) == pb)

	// The value in the interface is a copy: the variable it came from can
	// change after, and the value taken out can change too.
	b := Box[int]{V: 16}
	cp := sync.Pool{New: func() any { return b }}
	got := cp.Get().(Box[int])
	b.V = 17
	got2 := cp.Get().(Box[int])
	got.V = 18
	fmt.Println("copies", got.V, got2.V, b.V)

	// A slice in an interface shares its array, as in Go.
	sl := []Box[int]{{V: 19}}
	sp := sync.Pool{New: func() any { return sl }}
	gs := sp.Get().([]Box[int])
	sl[0].V = 20
	fmt.Println("shared array", gs[0].V)

	// A native map (from mime.ParseMediaType) held in a map type of the
	// plugin is the same map as the unnamed one: the unnamed one must come
	// back unnamed, also after the named one went to native code. (The
	// named one comes back unnamed in default mode: a known limit, see
	// rememberValue in vm/native.go.)
	_, params, _ := mime.ParseMediaType("text/plain; charset=utf-8")
	np := sync.Pool{New: func() any { return Params(params) }}
	up := sync.Pool{New: func() any { return params }}
	_ = np.Get()
	um, isUnnamed := up.Get().(map[string]string)
	fmt.Println("native map", isUnnamed, len(um))
	params["x"] = "y"
	um, _ = up.Get().(map[string]string)
	fmt.Println("native map shared", um["x"])
}

type Params map[string]string

type withMethod[T any] struct{ v T }

func (w withMethod[T]) Get() T { return w.v }

type genErr[T any] struct{ v T }

func (e genErr[T]) Error() string { return fmt.Sprint("genErr ", e.v) }
