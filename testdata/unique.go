package main

// unique.Make is a generic function of a native package: no native
// instance exists, the VM implements it (GOROOT/test fixedbugs/issue68415.go).

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unique"
)

type pt struct{ X, Y int }

// F has a float field: +0 and -0 are equal for ==, NaN is not equal to itself.
type F struct{ X float64 }

// W has a field of a native struct type, kept by the VM as a native value.
type W struct {
	T time.Time
	N int
}

type valuer interface{ Value() string }

func main() {
	a := unique.Make("hello")
	b := unique.Make("hel" + string([]byte("lo")))
	c := unique.Make("other")
	fmt.Println(a == b, a == c, a.Value(), c.Value())

	p := unique.Make(pt{1, 2})
	q := unique.Make(pt{1, 2})
	fmt.Println(p == q, p.Value().Y)

	var z unique.Handle[string]
	fmt.Println(z == a)

	hs := map[unique.Handle[string]]int{a: 1}
	hs[b]++
	fmt.Println(hs[a], len(hs))

	// Native values: each distinct time gets its own handle.
	t1 := unique.Make(time.Unix(1, 0).UTC())
	t2 := unique.Make(time.Unix(2, 0).UTC())
	t3 := unique.Make(time.Unix(2, 0).UTC())
	fmt.Println(t1 == t2, t2 == t3, t1.Value().Unix(), t2.Value().Unix())
	w1 := unique.Make(W{time.Unix(1, 0).UTC(), 1})
	w2 := unique.Make(W{time.Unix(2, 0).UTC(), 1})
	w3 := unique.Make(W{time.Unix(2, 0).UTC(), 1})
	fmt.Println(w1 == w2, w2 == w3, w2.Value().T.Unix())

	// Floats in structs and arrays are compared by value.
	negZero := math.Copysign(0, -1)
	fmt.Println(unique.Make(F{0}) == unique.Make(F{negZero}))
	fmt.Println(unique.Make([2]float64{0, 1}) == unique.Make([2]float64{negZero, 1}))
	m := map[F]int{{0}: 1}
	m[F{negZero}]++
	m[F{math.NaN()}] = 1
	m[F{math.NaN()}] = 2
	fmt.Println(m[F{0}], len(m))

	// Handle[T].Value through an interface.
	var v valuer = unique.Make("hi")
	fmt.Println(v.Value(), v == valuer(unique.Make("hi")))

	// unique.Make in defer and go statements.
	done := make(chan bool)
	go unique.Make(2)
	go func() { unique.Make(1); done <- true }()
	<-done
	func() {
		defer unique.Make("x")
		defer fmt.Println("deferred")
	}()
	fmt.Println(unique.Make("x").Value())

	// A handle that goes through native code keeps its identity.
	var sm sync.Map
	sm.Store("k", unique.Make("stored"))
	sm.Store(unique.Make(pt{3, 4}), "by handle")
	got, _ := sm.Load("k")
	fmt.Println(got.(unique.Handle[string]) == unique.Make("stored"), got.(unique.Handle[string]).Value())
	byKey, ok := sm.Load(unique.Make(pt{3, 4}))
	fmt.Println(byKey, ok)
	var av atomic.Value
	av.Store(unique.Make(W{time.Unix(5, 0).UTC(), 5}))
	fmt.Println(av.Load().(unique.Handle[W]) == unique.Make(W{time.Unix(5, 0).UTC(), 5}))

	// A handle kept only in native memory keeps its value alive.
	storeOnly(&sm)
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	only, _ := sm.Load("only")
	fmt.Println(only.(unique.Handle[string]) == unique.Make("only native"), only.(unique.Handle[string]).Value())
}

func storeOnly(sm *sync.Map) {
	sm.Store("only", unique.Make("only "+string([]byte("native"))))
}
