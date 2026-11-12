// Generic types and functions of native standard library packages, without
// slices, maps, iter or cmp (their source is not loaded): atomic.Pointer
// shared by goroutines (also run under -race), math/rand/v2.N, weak
// pointers, runtime.AddCleanup, reflect.TypeAssert, hash/maphash,
// crypto/hkdf, crypto/pbkdf2 and database/sql.Null.
package main

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"hash/maphash"
	"math/rand/v2"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"weak"
)

type config struct {
	gen  int
	name string
}

type key struct {
	a int
	b string
}

type status string

func atomicPointer() {
	var cur atomic.Pointer[config]
	fmt.Println("zero:", cur.Load() == nil)
	first := &config{name: "first"}
	cur.Store(first)
	fmt.Println("same pointer:", cur.Load() == first)

	// Each goroutine replaces the config with a new one, gen+1, with a
	// CompareAndSwap loop: no update may be lost.
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				for {
					old := cur.Load()
					if cur.CompareAndSwap(old, &config{gen: old.gen + 1, name: old.name}) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("gen:", cur.Load().gen, cur.Load().name)
	old := cur.Swap(first)
	fmt.Println("swap:", old.gen, cur.Load() == first, cur.CompareAndSwap(old, nil), cur.CompareAndSwap(first, nil), cur.Load() == nil)
}

func randN() {
	ok := true
	for i := 0; i < 1000; i++ {
		if n := rand.N(10); n < 0 || n >= 10 {
			ok = false
		}
		if d := rand.N(time.Second); d < 0 || d >= time.Second {
			ok = false
		}
		if u := rand.N(uint8(200)); u >= 200 {
			ok = false
		}
		if u := rand.N(uint64(1) << 63); u >= 1<<63 {
			ok = false
		}
	}
	fmt.Println("rand.N in range:", ok, rand.N(1), rand.N(int8(1)))
	for _, f := range []func(){func() { rand.N(0) }, func() { rand.N(-3) }, func() { rand.N(uint(0)) }} {
		func() {
			defer func() { fmt.Println("recovered:", recover()) }()
			f()
		}()
	}
}

func weakPointers() {
	x := &config{name: "x"}
	w := weak.Make(x)
	fmt.Println("weak value:", w.Value() == x, w.Value().name)
	fmt.Println("weak equal:", w == weak.Make(x), w != weak.Make(&config{name: "x"}))
	var zero weak.Pointer[config]
	fmt.Println("weak nil:", weak.Make[config](nil) == zero, zero.Value() == nil)
	s := struct{ a, b int }{1, 2}
	wa, wb := weak.Make(&s.a), weak.Make(&s.b)
	fmt.Println("weak fields:", wa != wb, *wb.Value())
	cache := map[weak.Pointer[config]]string{w: "cached"}
	fmt.Println("weak key:", cache[weak.Make(x)])
	runtime.KeepAlive(x)
	runtime.KeepAlive(&s)
}

func cleanup() {
	x := &config{name: "c"}
	c := runtime.AddCleanup(x, func(s string) { fmt.Println("never:", s) }, "cleanup")
	c.Stop()
	runtime.KeepAlive(x)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		runtime.AddCleanup(x, func(p *config) {}, x)
	}()
	fmt.Println("cleanup stopped")
}

func typeAssert() {
	n, ok := reflect.TypeAssert[int](reflect.ValueOf(3))
	fmt.Println("TypeAssert:", n, ok)
	c, ok := reflect.TypeAssert[config](reflect.ValueOf(config{gen: 4}))
	fmt.Println("TypeAssert:", c.gen, ok)
	s, ok := reflect.TypeAssert[fmt.Stringer](reflect.ValueOf(time.Second))
	fmt.Println("TypeAssert:", s, ok)
	_, ok = reflect.TypeAssert[string](reflect.ValueOf(3))
	fmt.Println("TypeAssert:", ok)
	var err error
	_, ok = reflect.TypeAssert[error](reflect.ValueOf(&err).Elem())
	fmt.Println("TypeAssert nil error:", ok)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		reflect.TypeAssert[int](reflect.Value{})
	}()
}

func hashes() {
	seed := maphash.MakeSeed()
	fmt.Println("Comparable:", maphash.Comparable(seed, key{1, "a"}) == maphash.Comparable(seed, key{1, "a"}),
		maphash.Comparable(seed, "s") == maphash.Comparable(seed, "s"))
	var h1, h2 maphash.Hash
	h1.SetSeed(seed)
	h2.SetSeed(seed)
	maphash.WriteComparable(&h1, key{2, "b"})
	maphash.WriteComparable(&h2, key{2, "b"})
	fmt.Println("WriteComparable:", h1.Sum64() == h2.Sum64())

	k, err := hkdf.Key(sha256.New, []byte("secret"), []byte("salt"), "info", 16)
	fmt.Printf("hkdf.Key: %x %v\n", k, err)
	prk, err := hkdf.Extract(sha256.New, []byte("secret"), []byte("salt"))
	fmt.Printf("hkdf.Extract: %x %v\n", prk, err)
	out, err := hkdf.Expand(sha256.New, prk, "info", 16)
	fmt.Printf("hkdf.Expand: %x %v\n", out, err)
	dk, err := pbkdf2.Key(sha256.New, "password", []byte("salt"), 10, 16)
	fmt.Printf("pbkdf2.Key: %x %v\n", dk, err)
}

func sqlNull() {
	var s sql.Null[string]
	fmt.Println("Null.Scan:", s.Scan("x"), s.V, s.Valid)
	var n sql.Null[int64]
	err := n.Scan([]byte("42"))
	v, verr := n.Value()
	fmt.Println("Null.Scan:", err, n.V, n.Valid, v, verr)
	fmt.Println("Null.Scan error:", n.Scan("abc") != nil)
	var st sql.Null[status]
	fmt.Println("Null.Scan named:", st.Scan("ok"), st.V, st.Valid)
	fmt.Println("Null.Scan nil:", st.Scan(nil), st.V == "", st.Valid)
	v, verr = st.Value()
	fmt.Println("Null.Value:", v, verr)
}

func main() {
	atomicPointer()
	randN()
	weakPointers()
	cleanup()
	typeAssert()
	hashes()
	sqlNull()
}
