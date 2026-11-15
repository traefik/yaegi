// A racy read of a pointer, map, chan or func variable is one machine word
// in Go: it sees the old or the new value, never a mix. Plugins rely on it
// when a reload goroutine swaps a config pointer without a lock while
// ServeHTTP reads it. In the VM such a variable is a Value of three words;
// a torn read gave a nil dereference (a non nil R with a nil data word) or
// type confusion between an interpreted closure and a native func (a crash
// or a "stack overflow" of the host). The writes are plain stores, copy,
// atomic.StorePointer, whole struct copies (one ended with a SIGBUS of
// the host), the swaps of sort.Slice and reflect.Swapper, clear, and
// reflect Set, Copy and Clear. An interface is two words and tears
// in Go too, so it is not tested here.
//
// A slice is three words in Go and tears there too, but in the VM its R
// may be a []Value or a native reflect.Value (strings.Split): a torn read
// confused the two Go types, a "fatal error: fault" of the host. The
// slices swapped here have the same length and capacity, so that a torn
// slice of gc is still one of the two. fmt reads a struct through a
// reflect view: it read the live cell more than once, and a swap of a
// field gave "interface conversion: interface {} is nil, not *vm.MapV".
//
// Each reader checks what Go guarantees (the value is one of the values
// written) and counts the panics. The program has data races on purpose:
// run it without -race.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type Config struct{ A, B int }

type Holder struct {
	C *Config
	F func(string) string
}

type Pair struct {
	P *Config
	F func(string) string
}

type SlicePair struct {
	P *Config
	L []string
}

type View struct {
	M map[string]int
	F func() int
	P *Config
}

// Swapped holds fields that a writer swaps between a native value and a
// value of the VM while fmt prints the struct.
type Swapped struct {
	F map[string][]string
	S []string
	A any
}

var (
	cfg  *Config
	mp   map[string]int
	ch   <-chan time.Time
	fn   func() int
	sfn  = strings.ToLower
	ip   *int
	hold = &Holder{F: strings.ToLower}
	cfgs = make([]*Config, 4)
	up   unsafe.Pointer
	gs   Pair
	sl   []string
	rsrc []string
	sp   SlicePair
	sa   [2][]string
	view = &View{}
	swp  = &Swapped{}
	gm   map[string][]string
	gss  []string
)

// chk reports whether s is one of the two slices that the slice cases
// write: a native one (strings.Split) and one of the VM.
func chk(s []string) bool {
	return len(s) == 2 && (s[0] == "a" && s[1] == "b" || s[0] == "l1" && s[1] == "l2")
}

// run calls write in one goroutine and read in readers goroutines for a
// while. A read that panics or returns false saw a torn value.
func run(name string, write func(i int), read func() bool) {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			if i&255 == 0 {
				select {
				case <-stop:
					return
				default:
				}
			}
			write(i)
		}
	}()
	var bad atomic.Int64
	var first sync.Once
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				func() {
					defer func() {
						if e := recover(); e != nil {
							first.Do(func() { fmt.Fprintln(os.Stderr, name, "panic:", e) })
							bad.Add(1)
						}
					}()
					for k := 0; k < 1000; k++ {
						if !read() {
							first.Do(func() { fmt.Fprintln(os.Stderr, name, "wrong value") })
							bad.Add(1)
						}
					}
				}()
			}
		}()
	}
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	fmt.Println(name, "torn:", bad.Load() > 0)
}

const duration = 250 * time.Millisecond

var readers = 3

func main() {
	// at least 4 threads, so that the writer and the readers run in
	// parallel even on a small machine
	runtime.GOMAXPROCS(max(4, runtime.NumCPU()))
	readers = max(3, runtime.GOMAXPROCS(0)-1)
	one := &Config{A: 1}
	okc := func(c *Config) bool { return c == nil || c.A == 1 }
	run("pointer", func(i int) {
		if i&1 == 0 {
			cfg = one
		} else {
			cfg = nil
		}
	}, func() bool { return okc(cfg) })
	m := map[string]int{"a": 1}
	run("map", func(i int) {
		if i&1 == 0 {
			mp = m
		} else {
			mp = nil
		}
	}, func() bool { v, ok := mp["a"]; return v == 1 || !ok })
	// a native chan (a reflect.Value in the VM) and a chan of the VM: a
	// torn read mixes the two Go types
	native := time.NewTimer(time.Hour).C
	var mine <-chan time.Time = make(chan time.Time)
	run("chan", func(i int) {
		if i&1 == 0 {
			ch = native
		} else {
			ch = mine
		}
	}, func() bool {
		select {
		case <-ch:
			return false
		default:
		}
		c := ch
		return c == nil || c == native || c == mine
	})
	f1 := func() int { return 1 }
	run("func", func(i int) {
		if i&1 == 0 {
			fn = f1
		} else {
			fn = nil
		}
	}, func() bool {
		if f := fn; f != nil {
			return f() == 1
		}
		return true
	})
	id := func(s string) string { return s }
	run("native func", func(i int) {
		if i&1 == 0 {
			sfn = strings.ToUpper
		} else {
			sfn = id
		}
	}, func() bool { r := sfn("a"); return r == "a" || r == "A" })
	run("field", func(i int) {
		if i&1 == 0 {
			hold.C, hold.F = one, strings.ToUpper
		} else {
			hold.C, hold.F = nil, id
		}
	}, func() bool {
		r := hold.F("a")
		return okc(hold.C) && (r == "a" || r == "A")
	})
	all := func(s []*Config) bool {
		for _, c := range s {
			if !okc(c) {
				return false
			}
		}
		return true
	}
	run("slice element", func(i int) {
		if i&1 == 0 {
			cfgs[i&3] = one
		} else {
			cfgs[(i-1)&3] = nil
		}
	}, func() bool { return all(cfgs) })
	ones, nils := []*Config{one, one, one, one}, make([]*Config, 4)
	run("copy", func(i int) {
		if i&1 == 0 {
			copy(cfgs, ones)
		} else {
			copy(cfgs, nils)
		}
	}, func() bool { return all(cfgs) })
	run("atomic.StorePointer", func(i int) {
		if i&1 == 0 {
			atomic.StorePointer(&up, unsafe.Pointer(one))
		} else {
			atomic.StorePointer(&up, nil)
		}
	}, func() bool { return okc((*Config)(up)) })
	run("struct copy", func(i int) {
		if i&1 == 0 {
			gs = Pair{one, strings.ToUpper}
		} else {
			gs = Pair{nil, id}
		}
	}, func() bool {
		// Go may write the fields one by one (a zero between them), but
		// never half of one.
		ok := okc(gs.P)
		if f := gs.F; f != nil {
			r := f("a")
			ok = ok && (r == "a" || r == "A")
		}
		s := gs
		ok = ok && okc(s.P)
		if s.F != nil {
			r := s.F("a")
			ok = ok && (r == "a" || r == "A")
		}
		return ok
	})
	nip := flag.Int("x", 7, "")
	zero := new(int)
	run("native pointer", func(i int) {
		if i&1 == 0 {
			ip = nip
		} else {
			ip = zero
		}
	}, func() bool {
		p := ip
		return p == nil || *p == 7 || *p == 0
	})
	readAll := func(s []*Config) func() bool { return func() bool { return all(s) } }
	srt := []*Config{one, nil, one, nil}
	run("sort.Slice", func(i int) {
		up := i&1 == 0
		sort.Slice(srt, func(a, b int) bool { return (srt[a] == nil) == up && (srt[b] == nil) != up })
	}, readAll(srt))
	sw := []*Config{one, nil}
	swap := reflect.Swapper(sw)
	run("reflect.Swapper", func(int) { swap(0, 1) }, readAll(sw))
	cl := make([]*Config, 4)
	run("clear", func(i int) {
		if i&1 == 0 {
			clear(cl)
		} else {
			copy(cl, ones)
		}
	}, readAll(cl))
	var rs *Config
	rv, vone := reflect.ValueOf(&rs).Elem(), reflect.ValueOf(one)
	run("reflect Set", func(i int) {
		if i&1 == 0 {
			rv.Set(vone)
		} else {
			rv.SetZero()
		}
	}, func() bool { return okc(rs) })
	rc := make([]*Config, 4)
	rdst, rones, rnils := reflect.ValueOf(rc), reflect.ValueOf(ones), reflect.ValueOf(nils)
	run("reflect.Copy", func(i int) {
		if i&1 == 0 {
			reflect.Copy(rdst, rones)
		} else {
			reflect.Copy(rdst, rnils)
		}
	}, readAll(rc))
	rcl := make([]*Config, 4)
	rclv := reflect.ValueOf(rcl)
	run("reflect Clear", func(i int) {
		if i&1 == 0 {
			rclv.Clear()
		} else {
			copy(rcl, ones)
		}
	}, readAll(rcl))

	// Slices: a native slice and a slice of the VM, of the same length
	// and capacity.
	nat := strings.Split("a,b", ",")
	lit := []string{"l1", "l2"}
	sl = lit
	run("slice", func(i int) {
		if i&1 == 0 {
			sl = nat
		} else {
			sl = lit
		}
	}, func() bool { return chk(sl) })
	loc := lit // a variable captured by the closures: a cell of the heap
	run("captured slice", func(i int) {
		if i&1 == 0 {
			loc = nat
		} else {
			loc = lit
		}
	}, func() bool { return chk(loc) })
	sp = SlicePair{nil, lit}
	run("slice field", func(i int) {
		if i&1 == 0 {
			sp = SlicePair{one, nat}
		} else {
			sp = SlicePair{nil, lit}
		}
	}, func() bool {
		s := sp
		return chk(s.L) && okc(s.P) && chk(sp.L)
	})
	sa = [2][]string{lit, lit}
	run("array of slices", func(i int) {
		if i&1 == 0 {
			sa = [2][]string{nat, lit}
		} else {
			sa = [2][]string{lit, nat}
		}
	}, func() bool {
		a := sa
		return chk(a[0]) && chk(a[1]) && chk(sa[1])
	})
	ss := [][]string{lit, lit, lit, lit}
	run("slice of slices", func(i int) {
		if i&1 == 0 {
			ss[i&3] = nat
		} else {
			ss[(i-1)&3] = lit
		}
	}, func() bool {
		for _, s := range ss {
			if !chk(s) {
				return false
			}
		}
		return true
	})
	nats, lits := [][]string{nat, nat, nat, nat}, [][]string{lit, lit, lit, lit}
	cs := [][]string{lit, lit, lit, lit}
	run("copy of slices", func(i int) {
		if i&1 == 0 {
			copy(cs, nats)
		} else {
			copy(cs, lits)
		}
	}, func() bool { return chk(cs[0]) && chk(cs[3]) })
	// append writes into the free room of base, which base[:1] reads
	base := append(make([][]string, 0, 4), lit)[:0]
	run("append of slices", func(i int) {
		if i&1 == 0 {
			_ = append(base, nat)
		} else {
			_ = append(base, lit)
		}
	}, func() bool { return chk(base[:1][0]) })
	// an append that grows reads the elements of a slice that a writer
	// changes
	src := [][]string{lit, lit}
	run("append grows", func(i int) {
		if i&1 == 0 {
			src[1] = nat
		} else {
			src[1] = lit
		}
	}, func() bool {
		g := append([][]string(nil), src...)
		g = append(src[:2:2], g...)
		return chk(g[1]) && chk(g[3])
	})
	rsl := reflect.ValueOf(&sl).Elem()
	vnat, vlit := reflect.ValueOf(nat), reflect.ValueOf(lit)
	run("reflect slice", func(i int) {
		if i&1 == 0 {
			rsl.Set(vnat)
		} else {
			rsl.Set(vlit)
		}
	}, func() bool {
		a, b := rsl.Index(0).String(), rsl.Index(1).String()
		return chk(rsl.Interface().([]string)) && rsl.Len() == 2 &&
			(a == "a" || a == "l1") && (b == "b" || b == "l2")
	})
	// A live reflect view of a slice variable that the writer assigns: Set,
	// Append and Copy read the variable once for the check of its type and
	// for its data. Two reads gave the []Value type of the VM with the data
	// of a native slice: a fatal crash of the host, or a forged length.
	writeSrc := func(i int) {
		if i&1 == 0 {
			rsrc = nat
		} else {
			rsrc = lit
		}
	}
	rsrc = lit
	vsrc := reflect.ValueOf(&rsrc).Elem()
	run("reflect Set from a view", writeSrc, func() bool {
		var d []string
		reflect.ValueOf(&d).Elem().Set(vsrc)
		return chk(d)
	})
	vz := reflect.ValueOf("z")
	run("reflect.Append of a view", writeSrc, func() bool {
		s := reflect.Append(vsrc, vz).Interface().([]string)
		return len(s) == 3 && chk(s[:2]) && s[2] == "z"
	})
	run("reflect.Copy from a view", writeSrc, func() bool {
		d := make([]string, 2)
		return reflect.Copy(reflect.ValueOf(d), vsrc) == 2 && chk(d)
	})

	// fmt reads a struct through a reflect view while its fields change.
	fone := func() int { return 1 }
	run("fmt", func(i int) {
		if i&1 == 0 {
			view.M, view.F, view.P = m, fone, one
		} else {
			view.M, view.F, view.P = nil, nil, nil
		}
	}, func() bool {
		_ = fmt.Sprintf("%v %+v", view, *view)
		return true
	})

	// fmt prints the fields of a struct and the variable behind a pointer:
	// the conversion for native code read them with plain loads, and a
	// swap between a native map or slice (url.ParseQuery, strings.Split)
	// and one of the VM gave "concurrent map iteration and map write" or
	// "fatal error: fault" of the host.
	nq, _ := url.ParseQuery("a=1&b=2")
	vq := map[string][]string{"x": {"y"}}
	run("fmt struct fields", func(i int) {
		if i&1 == 0 {
			swp.F, swp.S, swp.A = nq, nat, nat
		} else {
			swp.F, swp.S, swp.A = vq, lit, lit
		}
	}, func() bool {
		func() {
			// fmt reads the live map field of swp twice (its length,
			// then its entries): gc itself may panic there when the
			// field changes between the two reads. The VM must not
			// crash the host (a fatal error is not recovered).
			defer func() { _ = recover() }()
			_ = fmt.Sprint(swp)
		}()
		_ = fmt.Sprint(*swp)
		return true
	})
	gm, gss = vq, lit
	run("fmt pointer to a map", func(i int) {
		if i&1 == 0 {
			gm = nq
		} else {
			gm = vq
		}
	}, func() bool {
		s := fmt.Sprintf("%v", &gm)
		return s == "&map[a:[1] b:[2]]" || s == "&map[x:[y]]"
	})
	run("fmt pointer to a slice", func(i int) {
		if i&1 == 0 {
			gss = nat
		} else {
			gss = lit
		}
	}, func() bool {
		// fmt reads the live variable for each element (reflect Index of
		// an addressable slice), so gc may mix the two slices too
		s := fmt.Sprintf("%v", &gss)
		return s == "&[a b]" || s == "&[l1 l2]" || s == "&[a l2]" || s == "&[l1 b]"
	})

	// A native call that only reads through a pointer (fmt.Sprint(p))
	// must not write the value back: it undid the writes of the owner made
	// during the call (gc loses none).
	n := 0
	pn := &n
	var lost atomic.Int64
	run("read only call", func(i int) {
		*pn = i
		if *pn != i {
			lost.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(pn)
		return true
	})
	fmt.Println("read only call lost writes:", lost.Load() > 0)

	// The same with a value that holds a slice or a map: in -aot the
	// shadow of the variable was written back after the call, even when
	// native code did not change it.
	x1, x2 := []string{"a"}, []string{"b"}
	ros := x1
	var lostS atomic.Int64
	run("read only call on a slice", func(i int) {
		w := x1
		if i&1 == 1 {
			w = x2
		}
		ros = w
		if &ros[0] != &w[0] {
			lostS.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(&ros)
		return true
	})
	fmt.Println("read only call on a slice lost writes:", lostS.Load() > 0)
	m1, m2 := map[string]int{"a": 1}, map[string]int{"b": 2}
	rm := m1
	var lostM atomic.Int64
	run("read only call on a map", func(i int) {
		w := m1
		if i&1 == 1 {
			w = m2
		}
		rm = w
		if len(rm) != 1 || rm["a"] != w["a"] || rm["b"] != w["b"] {
			lostM.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(&rm)
		return true
	})
	fmt.Println("read only call on a map lost writes:", lostM.Load() > 0)
	ra := &[2][]string{x1, x1}
	var lostA atomic.Int64
	run("read only call on an array of slices", func(i int) {
		w := x1
		if i&1 == 1 {
			w = x2
		}
		ra[1] = w
		if &ra[1][0] != &w[0] {
			lostA.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(ra)
		return true
	})
	fmt.Println("read only call on an array of slices lost writes:", lostA.Load() > 0)
	// native memory that the variable shares: a native slice and a []byte
	n1, n2 := strings.Split("a", ","), strings.Split("b", ",")
	rn := n1
	var lostN atomic.Int64
	run("read only call on a native slice", func(i int) {
		w := n1
		if i&1 == 1 {
			w = n2
		}
		rn = w
		if &rn[0] != &w[0] {
			lostN.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(&rn)
		return true
	})
	fmt.Println("read only call on a native slice lost writes:", lostN.Load() > 0)
	b1, b2 := []byte("a"), []byte("b")
	rb := b1
	var lostB atomic.Int64
	run("read only call on a []byte", func(i int) {
		w := b1
		if i&1 == 1 {
			w = b2
		}
		rb = w
		if &rb[0] != &w[0] {
			lostB.Add(1)
		}
	}, func() bool {
		_ = fmt.Sprint(&rb)
		return true
	})
	fmt.Println("read only call on a []byte lost writes:", lostB.Load() > 0)
}
