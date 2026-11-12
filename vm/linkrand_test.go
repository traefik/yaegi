package vm

import (
	"go/types"
	"strings"
	"testing"
)

// TestLinkRandChecks checks the refusals of the emulated hash functions of
// the runtime (linkrand.go): a hash of a value held in cells, of a string
// variable that holds something else, and the signatures the table does
// not accept (the function then stays refused at load time).
func TestLinkRandChecks(t *testing.T) {
	m := NewMachine(nil, nil)
	cells := &Value{R: []Value{{N: 1}, {N: 2}}}
	bs := []byte("hello")
	m.extents.addBytes(&bs[0], len(bs)) // the memory of the pointer, as unsafe.SliceData records it
	for _, c := range []struct {
		name string
		fn   func()
		want string
	}{
		{"memhash of cells", func() { runtimeLinks["runtime.memhash"](m, []Value{{R: cells}, {}, {N: 16}}) }, "held in cells"},
		{"memhash64 of cells", func() { runtimeLinks["runtime.memhash64"](m, []Value{{R: cells}, {}}) }, "held in cells"},
		{"memhash nil", func() { runtimeLinks["runtime.memhash"](m, []Value{{}, {}, {N: 1}}) }, "nil pointer"},
		{"memhash past the end", func() { runtimeLinks["runtime.memhash"](m, []Value{{R: &bs[0]}, {}, {N: 6}}) }, "runtime.memhash: 6 bytes reach outside"},
		{"strhash of cells", func() { runtimeLinks["runtime.strhash"](m, []Value{{R: cells}, {}}) }, "needs a pointer to a string"},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(errString(r), c.want) {
					t.Errorf("%s: panic %v, want %q", c.name, r, c.want)
				}
			}()
			c.fn()
		}()
	}

	// memhash32 and memhash64: Go 1.27 refuses to link them, so
	// testdata/linkrand.go cannot call them; modules for older releases can.
	x, y, z := []byte("abcdefgh"), []byte("abcdefgh"), []byte("abcdefgX")
	for _, b := range [][]byte{x, y, z} {
		m.extents.addBytes(&b[0], len(b))
	}
	hash := func(fn string, b []byte, seed int64) int64 {
		return runtimeLinks[fn](m, []Value{{R: &b[0]}, {N: seed}}).N
	}
	for _, fn := range []string{"runtime.memhash32", "runtime.memhash64"} {
		if hash(fn, x, 0) != hash(fn, y, 0) {
			t.Errorf("%s: two hashes of the same bytes differ", fn)
		}
		if hash(fn, x, 0) == hash(fn, x, 9) {
			t.Errorf("%s: the seed does not change the hash", fn)
		}
	}
	if hash("runtime.memhash64", x, 0) == hash("runtime.memhash64", z, 0) {
		t.Errorf("runtime.memhash64: the 8th byte does not change the hash")
	}

	u32, u64, uptr := types.Typ[types.Uint32], types.Typ[types.Uint64], types.Typ[types.Uintptr]
	ptr := types.Typ[types.UnsafePointer]
	sig := func(res types.Type, params ...types.Type) *types.Signature {
		var ps []*types.Var
		for _, p := range params {
			ps = append(ps, types.NewParam(0, nil, "", p))
		}
		return types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), types.NewTuple(types.NewParam(0, nil, "", res)), false)
	}
	for _, c := range []struct {
		target string
		sig    *types.Signature
		ok     bool
	}{
		{"runtime.cheaprand", sig(u32), true},
		{"runtime.cheaprand", sig(u64), false},
		{"runtime.cheaprandn", sig(u32, u32), true},
		{"runtime.cheaprandn", sig(u32, u64), false},
		{"runtime.fastrand64", sig(u64), true},
		{"runtime.memhash", sig(uptr, ptr, uptr, uptr), true},
		{"runtime.memhash", sig(uptr, ptr, uptr), false},
		{"runtime.strhash", sig(uptr, ptr, uptr), true},
		{"runtime.strhash", sig(uptr, types.NewPointer(types.Typ[types.String]), uptr), false},
		{"runtime.procPin", sig(types.Typ[types.Int]), false}, // not emulated
	} {
		if got := linkSignatureOK(c.target, c.sig); got != c.ok {
			t.Errorf("linkSignatureOK(%s, %s) = %v, want %v", c.target, c.sig, got, c.ok)
		}
	}
}
