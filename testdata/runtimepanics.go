package main

// Run-time panics must have the type (runtime.Error) and the message of the
// gc runtime, and defer/recover must follow the gc rules.

import (
	"bytes"
	"fmt"
	mrand "math/rand"
	randv2 "math/rand/v2"
	"runtime"
	"sort"
	"unsafe"
)

func try(name string, f func()) {
	defer func() {
		r := recover()
		_, isRT := r.(runtime.Error)
		fmt.Printf("%s: %v (runtime.Error: %v)\n", name, r, isRT)
	}()
	f()
}

type T struct {
	a int
	s []int
}

type V int

func (V) M() {}

type I interface{ M() }

type J interface{ N() }

func nilArray() *[4]int { return nil }

// Two local types with the same name.
func scopeA() interface{}  { type L int; return L(1) }
func scopeB(x interface{}) { type L int; _ = x.(L) }

type G[T any] struct{ v T }

func (G[T]) M() {}

var (
	pv   *V
	nilV I = pv
	pg   *G[int]
	nilG I = pg
)

func main() {
	s := []int{1, 2, 3}
	arr := [3]int{1, 2, 3}
	str := "abc"
	var p *[4]int
	neg, big := -1, 9876543210
	var u64 uint64 = 1<<64 - 1
	var u32 uint32 = 1<<32 - 1
	var lo, hi = 4, 2

	// Index errors: signed and unsigned indexes.
	try("slice[-1]", func() { _ = s[neg] })
	try("slice[big]", func() { _ = s[big] })
	try("slice[u64]", func() { _ = s[u64] })
	try("array[u32]", func() { _ = arr[u32] })
	try("string[u64]", func() { _ = str[u64] })
	try("store[-1]", func() { s[neg] = 1 })
	try("nil slice", func() { var ns []int; _ = ns[0] })

	// Slice bounds errors.
	try("s[:big]", func() { _ = s[:big] })
	try("s[:-1]", func() { _ = s[:neg] })
	try("s[-1:]", func() { _ = s[neg:] })
	try("s[4:2]", func() { _ = s[lo:hi] })
	try("s[:u64]", func() { _ = s[:u64] })
	try("arr[:4]", func() { _ = arr[:lo] })
	try("str[:4]", func() { _ = str[:lo] })
	try("s[0:2:4]", func() { _ = s[0:hi:lo] })
	try("s[0:4:2]", func() { _ = s[0:lo:hi] })
	try("s[4:2:3]", func() { _ = s[lo:hi:3] })

	// A nil pointer to an array.
	try("p[0]", func() { _ = p[0] })
	try("p[:]", func() { _ = p[:] })
	try("len(p)", func() { _ = len(p) })
	n := 0
	for range *p { // not evaluated: len(*p) is constant
		n++
	}
	fmt.Println("range *p:", n)
	var q unsafe.Pointer
	n = 0
	for i := range *(*[3]int)(q) { // a conversion is not a call: not evaluated
		n += i
	}
	fmt.Println("range *(*[3]int)(nil):", n)
	try("range *nilArray()", func() {
		for range *nilArray() { // evaluated: it has a call
		}
	})

	// make.
	try("make len", func() { _ = make([]int, neg) })
	try("make cap", func() { _ = make([]int, lo, hi) })
	try("make const cap", func() { _ = make([]int, lo, 1) })
	// gc compares len*size and cap*size to its largest allocation.
	huge := 1 << 30
	try("make big elem len", func() { _ = make([][1 << 20]byte, huge) })
	try("make big elem cap", func() { _ = make([][1 << 20]byte, 0, huge) })
	try("make big elem const cap", func() { _ = make([][1 << 20]byte, huge, 2) })
	// A zero-size element uses no memory: gc allows any length. The VM keeps
	// a large slice of it as a zslice, with the same bound errors.
	maxI := int(^uint(0) >> 1)
	try("make zero-size maxInt", func() { fmt.Println(len(make([]struct{}, maxI))) })
	try("make zero-size const cap", func() { _ = make([]struct{}, maxI, 2) })
	zs := make([]struct{}, 1<<21)
	try("zslice[u64]", func() { _ = zs[u64] })
	try("&zslice[-1]", func() { _ = &zs[neg] })
	try("zslice[:u64]", func() { _ = zs[:u64] })
	try("zslice[4:2]", func() { _ = zs[lo:hi] })
	try("zslice[0:4:2]", func() { _ = zs[0:lo:hi] })
	var za [1 << 21]struct{}
	try("zarray[u32]", func() { _ = za[u32] })
	try("zarray[:u64]", func() { _ = za[:u64] })
	// An array pointer made from native bytes: the same bound errors.
	nb := (*[4]byte)(bytes.ToUpper([]byte("abcd")))
	try("native arr[:u64]", func() { _ = nb[:u64] })
	try("native arr[0:2:5]", func() { _ = nb[0:hi:5+neg*0] })

	// Type assertions.
	var e interface{} = 1
	var ni interface{}
	var i I = V(1)
	try("e.(string)", func() { _ = e.(string) })
	try("nil.(int)", func() { _ = ni.(int) })
	try("e.(I)", func() { _ = e.(I) })
	try("i.(J)", func() { _ = i.(J) })
	var nilI I
	try("nil.(I)", func() { _ = ni.(I) }) // "interface is nil"
	try("nilI.(J)", func() { _ = nilI.(J) })
	try("nilI.(*T)", func() { _ = nilI.(*struct{ I }) })
	var rnd interface{} = mrand.New(mrand.NewSource(1))
	try("other package", func() { _ = rnd.(*randv2.Rand) })
	try("other scope", func() { scopeB(scopeA()) })

	// Comparison and hash of values that are not comparable.
	var x, y interface{} = T{}, T{}
	try("T == T", func() { fmt.Println(x == y) })
	try("[]int == string", func() { fmt.Println(interface{}(s) == interface{}("a")) })
	try("[2]any", func() { fmt.Println([2]interface{}{1, s} == [2]interface{}{2, s}) })
	try("[2]any first", func() { fmt.Println([2]interface{}{s, 1} == [2]interface{}{s, 2}) })
	try("map key", func() { m := map[interface{}]int{}; m[x] = 1 })

	// Other run-time errors.
	try("nil map", func() { var m map[string]int; m["a"] = 1 })
	try("shift", func() { fmt.Println(1 << neg) })
	// ssa turns these local variables into constants.
	try("const shift", func() { x, y := 1, -2; fmt.Println(x >> y) })
	try("const shift int8", func() { x := 1; var c int8 = -1; fmt.Println(x << c) })
	zero := 0
	try("div", func() { fmt.Println(1 / zero) })
	try("panicwrap", func() { nilV.M() })
	try("panicwrap generic", func() { nilG.M() })

	// Deferred calls.
	try("defer nil func", func() {
		var f func()
		defer f()
		fmt.Println("defer nil func: the defer statement does not panic")
	})
	try("defer panic", func() {
		defer panic("deferred")
	})
	deferredRecover()
	deferredRecoverTooEarly()
	goexit()
}

// A deferred recover() stops the panic when the function that defers it is a
// deferred call that returns normally.
func deferredRecover() {
	defer func() { fmt.Println("after deferred recover:", recover()) }()
	defer func() {
		defer recover()
	}()
	panic(1)
}

// Run by a new panic of the same function, a deferred recover() does nothing.
func deferredRecoverTooEarly() {
	defer func() { fmt.Println("outer:", recover()) }()
	defer func() {
		defer func() { fmt.Println("inner:", recover()) }()
		defer recover()
		panic(3)
	}()
	panic(2)
}

// runtime.Goexit runs the deferred calls, and recover does not stop it.
func goexit() {
	done := make(chan bool)
	go func() {
		defer close(done)
		defer fmt.Println("goexit: deferred call runs")
		func() {
			defer func() { fmt.Println("goexit: recover:", recover()) }()
			runtime.Goexit()
		}()
		fmt.Println("goexit: not reached")
	}()
	<-done

	// The other ways to call runtime.Goexit: deferred, as a func value,
	// in a go statement, and from a callback of native code (fmt, sort).
	exit := runtime.Goexit
	cases := []struct {
		name string
		f    func()
	}{
		{"defer", func() { defer runtime.Goexit() }},
		{"deferred func", func() { defer func() { runtime.Goexit() }() }},
		{"recovered panic during goexit", func() {
			// The panic is recovered, but the Goexit goes on.
			defer func() { fmt.Println("goexit: recovered:", recover()) }()
			defer func() { panic("x") }()
			runtime.Goexit()
		}},
		{"func value", func() { exit() }},
		{"String method", func() { fmt.Println(goexitStringer{}) }},
		{"sort less", func() {
			s := []int{2, 1}
			sort.Slice(s, func(i, j int) bool { runtime.Goexit(); return false })
		}},
	}
	for _, c := range cases {
		done := make(chan bool)
		go func() {
			defer close(done)
			defer func() { fmt.Println("goexit", c.name+": deferred call runs, recover:", recover()) }()
			c.f()
			fmt.Println("goexit", c.name+": not reached")
		}()
		<-done
	}
	done = make(chan bool)
	go func() {
		defer close(done)
		go runtime.Goexit()
	}()
	<-done
}

type goexitStringer struct{}

func (goexitStringer) String() string {
	runtime.Goexit()
	return "not reached"
}
