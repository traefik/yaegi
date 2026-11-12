package main

import (
	"fmt"
	"sync"
	"unsafe"
)

// ssa builtins that the VM must run: unsafe.Sizeof, Alignof and Offsetof
// in generic functions, ssa:deferstack (defer in the body of a
// range-over-func loop) and a deferred panic.

type Inner struct {
	A byte
	B int64
}

type S[K any] struct {
	X K
	Inner
	In Inner
}

type GInner[K any] struct {
	A K
	C int8
}

type G[K any] struct {
	X int8
	GInner[K]
}

type Mid struct {
	P int8
	C int32
}

type Emb[K any] struct {
	A K
	Mid
}

type H[K any] struct {
	X int8
	Emb[K]
}

// hoffsets: x itself goes through an implicit step (h.Mid is h.Emb.Mid),
// and a promoted field goes through two implicit steps.
func hoffsets[K any](h *H[K]) []uintptr {
	return []uintptr{unsafe.Offsetof(h.Mid.C), unsafe.Offsetof(h.C), unsafe.Offsetof(h.Emb.Mid.C), unsafe.Offsetof(h.Mid)}
}

var called int

func calls() int { called++; return 1 }

// The operand of Sizeof, Alignof and Offsetof is never evaluated: no nil
// dereference, no call.
func notEvaluated[K any](s *S[K]) []uintptr {
	var p *K
	var m map[int]K
	var sl []K
	return []uintptr{
		unsafe.Sizeof(*p), unsafe.Alignof(*p), unsafe.Offsetof(s.X), unsafe.Offsetof(s.In.B),
		unsafe.Sizeof(s.In), unsafe.Sizeof(m[calls()]), unsafe.Sizeof(sl[5]),
		unsafe.Sizeof(func() K { calls(); return *p }()) + unsafe.Sizeof(calls()),
	}
}

type Lit[K any] struct {
	A K
	B int
	M map[int]K
	L []K
	P *Lit[K]
}

// Composite literals in the operand: ssa builds them with Alloc, Store
// and MapUpdate instructions, which must not run either.
func literals[K any]() []uintptr {
	var p *int
	var k *K
	return []uintptr{
		unsafe.Sizeof(Lit[K]{B: *p}),
		unsafe.Sizeof(Lit[K]{B: calls()}),
		unsafe.Sizeof(Lit[K]{M: map[int]K{calls(): *k}}),
		unsafe.Sizeof(Lit[K]{L: []K{*k, *k}, P: &Lit[K]{A: *k}}),
		unsafe.Sizeof([2]K{*k}),
	}
}

func goffsets[K any](g G[K]) []uintptr {
	// promoted field, explicit embedded field, field of a field
	return []uintptr{unsafe.Offsetof(g.C), unsafe.Offsetof(g.GInner), unsafe.Offsetof(g.GInner.C)}
}

func sizes[K any](s *S[K]) []uintptr {
	return []uintptr{
		unsafe.Sizeof(s.X), unsafe.Alignof(s.X), unsafe.Sizeof(*s),
		unsafe.Offsetof(s.X), unsafe.Offsetof(s.Inner), unsafe.Offsetof(s.B),
		unsafe.Offsetof(s.In), unsafe.Offsetof(s.In.B), unsafe.Offsetof(s.Inner.B),
	}
}

var log []string

func say(s string) { log = append(log, s) }

func seq(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		defer say("seq done")
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func loops() {
	defer say("first")
	for i := range seq(2) {
		for j := range seq(2) {
			defer say(fmt.Sprint(i, j))
		}
	}
	say("end of loops")
}

func early() int {
	for i := range seq(5) {
		defer say(fmt.Sprint("early ", i))
		if i == 1 {
			return i
		}
	}
	return -1
}

// A defer in the loop body can change a named result.
func named() (r int, s string) {
	for i := range seq(2) {
		defer func() { r = r*10 + i + 40 }()
	}
	return 1, "named"
}

// A recover in a loop body defer sets the named result.
var recovered any

func namedRecover() (r int) {
	for range seq(1) {
		defer func() {
			if recovered = recover(); recovered != nil {
				r = 7
			}
		}()
	}
	panic("boom")
}

// goSeq calls yield from another goroutine and waits for it.
func goSeq(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				if !yield(i) {
					return
				}
			}
		}()
		wg.Wait()
	}
}

func fromGoroutine() (r int) {
	for i := range goSeq(3) {
		defer func() { r += i }()
	}
	return 100
}

func deferPanic() (r any) {
	defer func() { r = recover() }()
	defer panic("deferred")
	return nil
}

func main() {
	fmt.Println(sizes(&S[int16]{}))
	fmt.Println(sizes(&S[[3]int32]{}))
	fmt.Println(goffsets(G[int64]{}), goffsets(G[byte]{}))
	fmt.Println(hoffsets[int64](nil), hoffsets[[3]byte](nil))
	fmt.Println(notEvaluated[[2]int64](nil), notEvaluated[byte](nil), called)
	fmt.Println(literals[int8](), literals[[3]int64](), called)
	loops()
	fmt.Println(log)
	log = nil
	fmt.Println(early(), log)
	fmt.Println(deferPanic())
	fmt.Println(named())
	fmt.Println(namedRecover(), recovered)
	fmt.Println(fromGoroutine())
}
