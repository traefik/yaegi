package vm

import (
	"reflect"
	"sync"
	"testing"
	"unsafe"
)

// TestNewAtRingFlood checks that NewAt finds the VM field whose address
// UnsafeAddrOf gave, also when other goroutines took more addresses than
// the ring holds in between (the go-cmp idiom under concurrency), and
// that NewAt never makes a Go pointer at an address that UnsafeAddr did
// not give: a VM cell has no Go layout.
func TestNewAtRingFlood(t *testing.T) {
	m, spkg := buildSrc(t, "package p\ntype T struct{ a, b int }\n")
	if m.vr == nil {
		m.vr = &vmReflect{m: m, sf: map[string]int{}}
	}
	typ := spkg.Pkg.Scope().Lookup("T").Type()
	vt := m.vr.vtypeOf(typ)
	cell := m.vr.zeroCell(typ)
	cell.R.([]Value)[1].N = 7
	base := UnsafeAddrOf(mkVM(vt, cell, flagAddr))

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 2 * len(unsafeRing) {
				UnsafeAddrOf(mkVM(vt, m.vr.zeroCell(typ), flagAddr))
			}
		})
	}
	wg.Wait()

	ft := vt.fieldT(1).cn()
	f := NewAt(ft, unsafe.Add(base, vt.fieldOffsets()[1])).Elem()
	if _, nat := f.native(); nat {
		t.Fatal("NewAt made a Go pointer into a VM cell")
	}
	if got := f.Int(); got != 7 {
		t.Errorf("NewAt: field b = %d, want 7", got)
	}

	// A native value: NewAt works inside the value whose address UnsafeAddr
	// gave, and refuses any other address.
	var x struct{ a, b int }
	x.b = 9
	nv := natRV(reflect.ValueOf(&x).Elem())
	p := UnsafeAddrOf(nv)
	if got := NewAt(ft, unsafe.Add(p, unsafe.Offsetof(x.b))).Elem().Int(); got != 9 {
		t.Errorf("NewAt in a native value: %d, want 9", got)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewAt at an address that UnsafeAddr did not give: no panic")
			}
		}()
		var y int
		NewAt(ft, unsafe.Pointer(&y))
	}()
}

// TestUnsafeByGBounded checks that the per-goroutine table of UnsafeAddr
// does not grow with the number of goroutines: its entries keep VM cells
// alive, and on wasm the key of a goroutine is never reused.
func TestUnsafeByGBounded(t *testing.T) {
	n := maxUnsafeByG + 100
	var x struct{ a, b int }
	var wg sync.WaitGroup
	release := make(chan struct{})
	wg.Add(n)
	for range n {
		go func() {
			rememberNative(reflect.ValueOf(&x).Elem())
			wg.Done()
			<-release // all n goroutines are alive at the same time
		}()
	}
	wg.Wait()
	unsafeMu.Lock()
	got := len(unsafeByG)
	unsafeMu.Unlock()
	close(release)
	if got > maxUnsafeByG {
		t.Errorf("unsafeByG has %d entries, want at most %d", got, maxUnsafeByG)
	}
}
