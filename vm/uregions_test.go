package vm

import (
	"go/types"
	"runtime"
	"testing"
)

// TestURegionsShareAnchor checks that the regions of the subslices s[i:] of
// one array hold one weak pointer, to the last element of the capacity:
// the runtime keeps one special per address of an object and searches them
// linearly, so a weak pointer per subslice made slices.Insert in a loop
// quadratic. The regions must still give back their
// own elements.
func TestURegionsShareAnchor(t *testing.T) {
	if !inHeapOK {
		t.Skip("inHeap is off on this runtime: the regions are strong")
	}
	b := make([]byte, 8, 16)
	for i := range b {
		b[i] = byte(i)
	}
	cells := make([]Value, 5, 9)
	for i := range cells {
		cells[i] = Value{N: int64(i)}
	}
	var rs uregions
	for i := 0; i < 4; i++ {
		rs.add(&uregion{base: uintptr(1000 + i), bytes: b[i:]})
		rs.add(&uregion{base: uintptr(2000 + i), elems: cells[i:], t: types.Typ[types.Int]})
	}
	var wb, we = rs.ring[0].wb, rs.ring[1].we
	for i := 0; i < 8; i++ {
		e := &rs.ring[i]
		if i%2 == 0 && e.wb != wb || i%2 == 1 && e.we != we {
			t.Fatalf("entry %d: a weak pointer of its own, not the one of the array", i)
		}
		var r uregion
		if !e.get(&r) {
			t.Fatalf("entry %d: region lost", i)
		}
		if i%2 == 0 {
			if len(r.bytes) != 8-i/2 || r.bytes[0] != byte(i/2) {
				t.Errorf("entry %d: bytes %v", i, r.bytes)
			}
		} else if len(r.elems) != 5-i/2 || r.elems[0].N != int64(i/2) {
			t.Errorf("entry %d: elems %v", i, r.elems)
		}
	}
	// the regions hold the arrays weakly: a GC before the checks would
	// collect them
	runtime.KeepAlive(b)
	runtime.KeepAlive(cells)
}
