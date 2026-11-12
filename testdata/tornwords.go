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
// in Go too, so it is not tested here. The program has data races on
// purpose: run it without -race.
package main

import (
	"flag"
	"fmt"
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
)

func run(name string, write func(i int), read func()) {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			write(i)
		}
	}()
	var mu sync.Mutex
	bad := 0
	for r := 0; r < 3; r++ {
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
							mu.Lock()
							if bad == 0 {
								fmt.Fprintln(os.Stderr, name, "panic:", e)
							}
							bad++
							mu.Unlock()
						}
					}()
					for k := 0; k < 1000; k++ {
						read()
					}
				}()
			}
		}()
	}
	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
	fmt.Println(name, "torn:", bad > 0)
}

func main() {
	runtime.GOMAXPROCS(4)
	one := &Config{A: 1}
	run("pointer", func(i int) {
		if i&1 == 0 {
			cfg = one
		} else {
			cfg = nil
		}
	}, func() {
		if c := cfg; c != nil {
			_ = c.A
		}
	})
	m := map[string]int{"a": 1}
	run("map", func(i int) {
		if i&1 == 0 {
			mp = m
		} else {
			mp = nil
		}
	}, func() { _ = mp["a"] })
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
	}, func() {
		select {
		case <-ch:
		default:
		}
	})
	f1 := func() int { return 1 }
	run("func", func(i int) {
		if i&1 == 0 {
			fn = f1
		} else {
			fn = nil
		}
	}, func() {
		if f := fn; f != nil {
			f()
		}
	})
	id := func(s string) string { return s }
	run("native func", func(i int) {
		if i&1 == 0 {
			sfn = strings.ToUpper
		} else {
			sfn = id
		}
	}, func() { _ = sfn("a") })
	run("field", func(i int) {
		if i&1 == 0 {
			hold.C, hold.F = one, strings.ToUpper
		} else {
			hold.C, hold.F = nil, id
		}
	}, func() {
		if c := hold.C; c != nil {
			_ = c.A
		}
		_ = hold.F("a")
	})
	run("slice element", func(i int) {
		if i&1 == 0 {
			cfgs[i&3] = one
		} else {
			cfgs[(i-1)&3] = nil
		}
	}, func() {
		for _, c := range cfgs {
			if c != nil {
				_ = c.A
			}
		}
	})
	ones, nils := []*Config{one, one, one, one}, make([]*Config, 4)
	run("copy", func(i int) {
		if i&1 == 0 {
			copy(cfgs, ones)
		} else {
			copy(cfgs, nils)
		}
	}, func() {
		for _, c := range cfgs {
			if c != nil {
				_ = c.A
			}
		}
	})
	run("atomic.StorePointer", func(i int) {
		if i&1 == 0 {
			atomic.StorePointer(&up, unsafe.Pointer(one))
		} else {
			atomic.StorePointer(&up, nil)
		}
	}, func() {
		if p := (*Config)(up); p != nil {
			_ = p.A
		}
	})
	run("struct copy", func(i int) {
		if i&1 == 0 {
			gs = Pair{one, strings.ToUpper}
		} else {
			gs = Pair{nil, id}
		}
	}, func() {
		// Go may write the fields one by one (a zero between them), but
		// never half of one.
		if c := gs.P; c != nil {
			_ = c.A
		}
		if f := gs.F; f != nil {
			_ = f("a")
		}
		s := gs
		if s.P != nil {
			_ = s.P.A
		}
		if s.F != nil {
			_ = s.F("a")
		}
	})
	nip := flag.Int("x", 7, "")
	run("native pointer", func(i int) {
		if i&1 == 0 {
			ip = nip
		} else {
			ip = new(int)
		}
	}, func() {
		if p := ip; p != nil {
			_ = *p
		}
	})
	readAll := func(s []*Config) func() {
		return func() {
			for _, c := range s {
				if c != nil {
					_ = c.A
				}
			}
		}
	}
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
	}, func() {
		if c := rs; c != nil {
			_ = c.A
		}
	})
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
}
