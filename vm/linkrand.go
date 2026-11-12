package vm

// Emulated functions of the runtime that many modules link to with
// //go:linkname: the cheap random numbers of the runtime (cheaprand,
// fastrand...) and its memory hash (memhash, strhash...). The runtime has
// them in Go or in assembly, on its own state (the random state of the M,
// the hash key of the process), which the VM cannot reach and must not
// copy: no runtime internals here. Each one is given with the standard
// library, with the meaning its callers rely on:
//
//   - the random functions give random numbers of the same range
//     (math/rand/v2, which also uses the runtime source);
//   - the hash functions give the same hash for the same bytes and seed
//     in one process, and other hashes for other bytes, but not the values
//     of the runtime: these change at each start of a gc program too (the
//     hash key is random), so no program can depend on them. They use
//     hash/maphash with a seed of the process.
//
// The list follows the runtime of a Go release: Go 1.22 renamed fastrand to
// cheaprand and kept the old names for "legacy code" (runtime/rand.go). A
// new name, or a function whose signature changes, is not in the table and
// stays refused at load time, as every other function without a body.

import (
	"encoding/binary"
	"go/types"
	"hash/maphash"
	"math/rand/v2"
)

// hashSeed is the seed of the emulated hash functions: one per process, as
// the hash key of the runtime.
var hashSeed = maphash.MakeSeed()

func init() {
	r32 := func(m *Machine, a []Value) Value { return Value{N: int64(rand.Uint32())} }
	r32n := func(m *Machine, a []Value) Value { return Value{N: int64(cheaprandn(uint32(a[0].N)))} }
	r64 := func(m *Machine, a []Value) Value { return Value{N: int64(rand.Uint64())} }
	for name, h := range map[string]NativeHook{
		"runtime.cheaprand":  r32,
		"runtime.fastrand":   r32,
		"runtime.cheaprandn": r32n,
		"runtime.fastrandn":  r32n,
		"runtime.randn":      r32n,
		"runtime.fastrand64": r64,
		"runtime.rand":       r64,
		"runtime.cheaprand64": func(m *Machine, a []Value) Value {
			// a non-negative int64, as the runtime gives
			return Value{N: rand.Int64()}
		},
		"runtime.memhash": func(m *Machine, a []Value) Value {
			n := uint64(a[2].N)
			var b []byte
			if n > 0 {
				b = m.hashBytes(a[0], n, "runtime.memhash")
			}
			return Value{N: int64(memHash(b, uint64(a[1].N)))}
		},
		"runtime.memhash32": func(m *Machine, a []Value) Value {
			return Value{N: int64(memHash(m.hashBytes(a[0], 4, "runtime.memhash32"), uint64(a[1].N)))}
		},
		"runtime.memhash64": func(m *Machine, a []Value) Value {
			return Value{N: int64(memHash(m.hashBytes(a[0], 8, "runtime.memhash64"), uint64(a[1].N)))}
		},
		"runtime.strhash": func(m *Machine, a []Value) Value {
			return Value{N: int64(memHash([]byte(hashString(a[0])), uint64(a[1].N)))}
		},
	} {
		runtimeLinks[name] = h
	}
}

// cheaprandn is runtime.cheaprandn: a random number in [0, n), 0 for n = 0.
func cheaprandn(n uint32) uint32 {
	return uint32(uint64(rand.Uint32()) * uint64(n) >> 32)
}

// memHash is the emulated memhash of b with the seed h.
func memHash(b []byte, h uint64) uintptr {
	var mh maphash.Hash
	mh.SetSeed(hashSeed)
	var w [8]byte
	binary.LittleEndian.PutUint64(w[:], h)
	mh.Write(w[:])
	mh.Write(b)
	return uintptr(mh.Sum64())
}

// hashBytes returns the n bytes at p for a hash function: the bytes the VM
// knows (a Go []byte, a string, plain data in real bytes, with the checks of
// memmove), or the integer in a cell for 4 or 8 bytes (a uint32 or a uint64
// variable: the cell holds its bits). Other cells are refused.
func (m *Machine) hashBytes(p Value, n uint64, name string) []byte {
	if c, ok := p.R.(*Value); ok && c != nil && c.R == nil && (n == 4 || n == 8) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(c.N))
		return b[:n]
	}
	if p.R == nil {
		panic(errNilDeref)
	}
	if _, ok := p.R.(*Value); ok {
		unsupported("%s of a value held in cells: the VM hashes only the bytes of a []byte, of a string, of plain data in real bytes, or a 4 or 8 byte integer", name)
	}
	return m.knownBytes(p, n, false, name)
}

// hashString returns the string at p for runtime.strhash (p is a *string).
func hashString(p Value) string {
	c, ok := p.R.(*Value)
	if !ok || c == nil {
		if p.R == nil {
			panic(errNilDeref)
		}
		unsupported("runtime.strhash of a %T: the VM needs a pointer to a string variable", p.R)
	}
	switch s := c.R.(type) {
	case string:
		return s
	case nil:
		return "" // an empty string (N may keep the address of its data)
	}
	unsupported("runtime.strhash of a %T: the VM needs a pointer to a string variable", c.R)
	return ""
}

// randLinkSignatureOK is linkSignatureOK for the functions of this file.
func randLinkSignatureOK(target string, sig *types.Signature) bool {
	ps, rs := sig.Params(), sig.Results()
	is := func(t types.Type, kinds ...types.BasicKind) bool {
		b, ok := t.Underlying().(*types.Basic)
		if !ok {
			return false
		}
		for _, k := range kinds {
			if b.Kind() == k {
				return true
			}
		}
		return false
	}
	ptr := func(t types.Type) bool { return is(t, types.UnsafePointer) }
	if sig.Variadic() || rs.Len() != 1 {
		return false
	}
	r := rs.At(0).Type()
	switch target {
	case "runtime.cheaprand", "runtime.fastrand":
		return ps.Len() == 0 && is(r, types.Uint32)
	case "runtime.cheaprandn", "runtime.fastrandn", "runtime.randn":
		return ps.Len() == 1 && is(ps.At(0).Type(), types.Uint32) && is(r, types.Uint32)
	case "runtime.fastrand64", "runtime.rand":
		return ps.Len() == 0 && is(r, types.Uint64)
	case "runtime.cheaprand64":
		return ps.Len() == 0 && is(r, types.Int64)
	case "runtime.memhash":
		return ps.Len() == 3 && ptr(ps.At(0).Type()) && is(ps.At(1).Type(), types.Uintptr) && is(ps.At(2).Type(), types.Uintptr) && is(r, types.Uintptr)
	case "runtime.memhash32", "runtime.memhash64", "runtime.strhash":
		return ps.Len() == 2 && ptr(ps.At(0).Type()) && is(ps.At(1).Type(), types.Uintptr) && is(r, types.Uintptr)
	}
	return false
}
