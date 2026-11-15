package vm

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// freeCellsSrc has 16 functions with a frame-local [4000]int (a cell of
// 4001 Values, 96 KB each). Hold starts n goroutines that call them all,
// then block until Release.
const freeCellsSrc = `package p

import "sync"

func f0(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f1(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f2(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f3(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f4(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f5(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f6(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f7(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f8(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f9(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f10(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f11(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f12(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f13(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f14(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

func f15(n int) int {
	var b [4000]int
	b[n%4000] = n
	return b[(n+1)%4000] + n
}

var ch = make(chan struct{})

func Hold(n int) int {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			s := f0(n) + f1(n) + f2(n) + f3(n) + f4(n) + f5(n) + f6(n) + f7(n) + f8(n) + f9(n) + f10(n) + f11(n) + f12(n) + f13(n) + f14(n) + f15(n)
			wg.Done()
			<-ch
			_ = s
		}()
	}
	wg.Wait()
	return n
}

func Release() { close(ch) }

`

// TestMaxMemoryFreeCells checks that the free cells that a ctx keeps (the
// cells of the frame-local arrays of the frames that returned, escape.go)
// are bounded and counted in MaxMemory. Each ctx kept up to 1.5 MB of free
// cells, one list per site, and the quota did not count them: 990
// goroutines that called 16 functions with a local [4000]int and then
// blocked kept about 1.5 GB of heap, with 101 MB in the quota and no
// ErrMemory under MaxMemory 1 GiB.
func TestMaxMemoryFreeCells(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("uses 200 MB")
	}
	dir, pkg := writePlugin(t, freeCellsSrc)
	p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30, MaxGoroutines: 1000}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	const n = 990
	var before, after runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := p.M.Memory()
	if _, err := p.Call(context.Background(), "Hold", n); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	used := p.M.Memory() - start
	heap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("%d blocked goroutines: heap +%d MB, quota +%d MB", n, heap>>20, used>>20)
	if _, err := p.Call(context.Background(), "Release"); err != nil {
		t.Fatal(err)
	}
	// Each goroutine keeps at most maxFreeCells Values of free cells, and
	// the quota counts them: the heap that the goroutines keep is about
	// what the quota counts (the rest: their small register stacks, about
	// 10 KB each).
	if per := heap / n; per > 256<<10 {
		t.Errorf("heap per blocked goroutine: %d bytes, want at most 256 KiB", per)
	}
	if heap > used+32<<20 {
		t.Errorf("heap +%d MB, quota +%d MB: the free cells are not counted", heap>>20, used>>20)
	}
}

// TestFreeCellsOtherMachine checks that a pooled ctx that served another
// Machine takes free cells again for the new one. The free cells of the
// first Machine stayed in the ctx and took the room of maxFreeBytes: each
// call of the second Machine then made a new 98 KiB cell for its local
// [4096]byte, and dropped it at the return.
func TestFreeCellsOtherMachine(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, stackQuotaSrc)
	var ps [2]*Plugin
	for i := range ps {
		p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30}, pkg)
		if err != nil {
			t.Fatal(err)
		}
		ps[i] = p
	}
	arg := strings.Repeat("x", 100)
	for _, p := range ps {
		for range 10 {
			if _, err := p.Call(context.Background(), "H", arg); err != nil {
				t.Fatal(err)
			}
		}
	}
	const calls = 100
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range calls {
		if _, err := ps[1].Call(context.Background(), "H", arg); err != nil {
			t.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)
	if per := (after.TotalAlloc - before.TotalAlloc) / calls; per > 16<<10 {
		t.Errorf("a call of the second Machine allocates %d bytes, want the cell of its local array taken from the free list", per)
	}
}

// pooledFreeSrc: F has a frame-local [5000]int and waits on ch1; Block has
// no local array and waits on ch2. The cell of b is 120 KB with a 24-byte
// Value (64-bit) and 80 KB with a 16-byte Value (386): on both, at least
// memMin (a smaller cell is not counted, as all the counts) and at most
// maxFreeBytes (a larger cell is not kept in the free list).
const pooledFreeSrc = `package p

var ch1 = make(chan struct{})
var ch2 = make(chan struct{})

func F(n int) int {
	var b [5000]int
	b[n%5000] = n
	<-ch1
	return b[(n+1)%5000]
}

func Release1() { close(ch1) }

func Block() int { <-ch2; return 0 }

func Release2() { close(ch2) }
`

// TestMaxMemoryPooledFreeCells checks that the free cells that a pooled
// ctx brings to a new call from the host count in MaxMemory. putCtx gives
// back their charge, and they were counted again only at the next
// OpAllocLocal: 1000 calls of F left 1000 pooled ctxs with a 120 KB free
// cell each, and 1000 calls of Block in flight then held 117 MB of heap
// with no charge in the quota.
func TestMaxMemoryPooledFreeCells(t *testing.T) {
	needSource(t)
	if testing.Short() {
		t.Skip("uses 120 MB")
	}
	if cell := 5000 * valueSize; cell < memMin || cell > maxFreeBytes {
		t.Fatalf("the cell of [5000]int is %d bytes, want it in [memMin, maxFreeBytes]", cell)
	}
	dir, pkg := writePlugin(t, pooledFreeSrc)
	p, err := Load(Options{Dir: dir, MaxMemory: 1 << 30}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	const n = 1000
	var before, after runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := p.M.Memory()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Call(context.Background(), "F", i); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(2 * time.Second)
	if _, err := p.Call(context.Background(), "Release1"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Call(context.Background(), "Block"); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(2 * time.Second)
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	used := p.M.Memory() - start
	heap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("%d requests in flight: heap +%d MB, quota +%d MB", n, heap>>20, used>>20)
	if _, err := p.Call(context.Background(), "Release2"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if heap > used+32<<20 {
		t.Errorf("heap +%d MB, quota +%d MB: the free cells of the pooled ctxs in flight are not counted", heap>>20, used>>20)
	}
}

// TestFreeCellsNativeSize checks that the cell of a frame-local native
// struct counts its gc size, in the free cells of a ctx (maxFreeBytes) and
// in MaxMemory and MaxStack. zeroEntries counts a native struct as 0
// entries (gc sizes it), so the cell of a local base64.Encoding (328
// bytes) counted as one Value (24 bytes), and maxFreeBytes did not bound
// the free cells of such locals.
func TestFreeCellsNativeSize(t *testing.T) {
	needSource(t)
	m, spkg := buildSrc(t, `package p

import "encoding/base64"

type T struct {
	n int
	e base64.Encoding
}

func use(e *base64.Encoding) bool { return e == nil }
func useT(x *T) int { return x.n }
func useA(a *[4]base64.Encoding) bool { return a == nil }

func A() bool { var e base64.Encoding; return use(&e) }
func B() int { var x T; return useT(&x) }
func C() bool { var a [4]base64.Encoding; return useA(&a) }
`)
	for _, name := range []string{"A", "B", "C"} {
		fn := m.funcOf(spkg.Func(name))
		fn.ensure()
		found := false
		for _, in := range fn.code {
			if in.Op != OpAllocLocal {
				continue
			}
			found = true
			s := in.X.(*memSite)
			et := s.t
			if size := gcSizes.Sizeof(et); s.cellVals*valueSize < size || int64(s.localSize)*valueSize < size {
				t.Errorf("%s: cell of %s counts %d Values (free list: %d), want at least its gc size %d bytes", name, et, s.cellVals, s.localSize, size)
			}
		}
		if !found {
			t.Errorf("%s: no frame-local Alloc", name)
		}
	}
}
