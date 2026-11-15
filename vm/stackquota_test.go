package vm

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
)

// stackQuotaSrc is a handler with a frame-local array of more than memMin
// bytes of Values ([4096]byte: a cell of 4097 Values, 98 KiB), as a request
// handler that copies a header into a local buffer.
const stackQuotaSrc = `package p

func f(p string) int {
	var buf [4096]byte
	n := copy(buf[:], p)
	s := 0
	for i := 0; i < n; i++ {
		s += int(buf[i])
	}
	return s
}

func H(p string) int { return f(p) }

func g(n int) int {
	var a [2000]int
	a[n%2000] = n
	if n == 0 {
		return a[0]
	}
	return g(n-1) + a[n%2000]
}

func R(n int) int { return g(n) }

func P(p string) int {
	var buf [4096]byte
	copy(buf[:], p)
	panic(string(buf[:1]))
}

func Spin(p string) int {
	var buf [4096]byte
	copy(buf[:], p)
	s := 0
	for i := 0; ; i++ {
		s += int(buf[i%4096])
	}
}
`

// TestMaxMemoryLocalCellRequests checks that a call from the host with a
// large frame-local array does not fill the memory quota. Each call took a
// pooled ctx with its cell in the free list, but reserved 1 MiB of the
// quota again with a new weak token, freed only by a garbage collection:
// after about 1024 calls with a 1 GiB quota, each call forced a
// collection, and with concurrent calls a collection run by another
// goroutine counted as its own, so the call got ErrMemory.
func TestMaxMemoryLocalCellRequests(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, stackQuotaSrc)
	p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	arg := strings.Repeat("x", 100)
	want := 100 * int('x')
	if _, err := p.Call(context.Background(), "H", arg); err != nil {
		t.Fatal(err)
	}
	if f := stackFunc(t, p, "f"); len(f.locals) != 1 || f.locals[0].site.cellVals*valueSize < memMin {
		t.Fatalf("f: the local array must be a cell of at least memMin bytes")
	}
	for _, c := range []int{1, 4, 64} {
		t.Run(fmt.Sprint("c=", c), func(t *testing.T) {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			gcs := ms.NumForcedGC
			const calls = 3000
			var wg sync.WaitGroup
			errs := make(chan error, c)
			for range c {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range calls / c {
						res, err := p.Call(context.Background(), "H", arg)
						if err != nil {
							errs <- err
							return
						}
						if v, _ := res[0].Interface(); v != want {
							errs <- fmt.Errorf("H returned %v, want %d", v, want)
							return
						}
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Fatalf("%d concurrent calls: %v", c, err)
			}
			runtime.ReadMemStats(&ms)
			if n := ms.NumForcedGC - gcs; n > 2 {
				t.Errorf("%d calls with %d at a time forced %d garbage collections, want none", calls, c, n)
			}
			if used := p.M.Memory(); used > 64<<20 {
				t.Errorf("after %d calls: %d bytes in the quota, want the cells of the calls in flight at most", calls, used)
			}
		})
	}
}

// TestMaxMemoryParallelRecursion checks that goroutines that recurse with
// large frame-local arrays, together well below the quota, do not get
// ErrMemory: 10 recursions of 800 frames with a [2000]int local (38 MB of
// cells each, 384 MB in all) with a quota of 1 GiB. The cells were charged
// in steps of 25 %, and the charge of a recursion that stopped stayed until
// a garbage collection.
func TestMaxMemoryParallelRecursion(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("uses 300 MB")
	}
	dir, pkg := writePlugin(t, stackQuotaSrc)
	p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	for round := range 3 {
		var wg sync.WaitGroup
		errs := make(chan error, 10)
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := p.Call(context.Background(), "R", 800); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		n := 0
		var first error
		for err := range errs {
			if n++; first == nil {
				first = err
			}
		}
		if n > 0 {
			t.Fatalf("round %d: %d of 10 recursions failed: %v (ErrMemory: %v)", round, n, first, errors.Is(first, ErrMemory))
		}
	}
	if used := p.M.Memory(); used > 64<<20 {
		t.Errorf("after the recursions: %d bytes in the quota, want the charges of the stacks given back", used)
	}
}

// TestMaxMemoryLocalCellPanic checks that a call from the host that ends
// with a panic gives back the stack charge of its ctx at once, without a GC:
// a panic of the plugin, ErrStackOverflow, ErrBudget. The ctx does not go
// back to the pool, and its charge stayed in the quota until a GC freed it
// (about 16 MB for each overflow below): about 60 calls that overflow filled
// a quota of 1 GiB, and the next calls waited for forced collections. The
// case P (a panic of the plugin) passed before too: it is a guard.
func TestMaxMemoryLocalCellPanic(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, stackQuotaSrc)
	// No GC during the test: only the give-back at the end of the call can
	// empty the quota.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	cases := []struct {
		name string
		args []any
		want func(error) bool
	}{
		{"P", []any{"x"}, func(err error) bool { return err != nil && strings.Contains(err.Error(), "x") }},
		{"R", []any{1 << 30}, func(err error) bool { return errors.Is(err, ErrStackOverflow) }},
		{"Spin", []any{"x"}, func(err error) bool { return errors.Is(err, ErrBudget) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30, MaxStack: 16 << 20, Budget: 1_000_000}, pkg)
			if err != nil {
				t.Fatal(err)
			}
			start := p.M.Memory()
			for i := range 10 {
				_, err := p.Call(context.Background(), c.name, c.args...)
				if !c.want(err) {
					t.Fatalf("unexpected error %v", err)
				}
				if n := stackCharged(p.M.mem); n != 0 {
					t.Fatalf("after %d calls that panic: %d bytes of stack charges in the quota, want 0", i+1, n)
				}
			}
			// The rest is the heap of the calls (the register stacks of the
			// ctxs that did not go back to the pool), counted until a GC
			// frees it.
			if used := p.M.Memory() - start; used > 16<<20 {
				t.Errorf("after 10 calls that panic: %d bytes more in the quota, want less than 16 MiB", used)
			}
		})
	}
}

// stackCharged returns the bytes of the stack charges counted in q.
func stackCharged(q *memQuota) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var n int64
	for _, sc := range q.stacks {
		n += sc.n
	}
	return n
}
