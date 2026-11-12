package vm

// sync/atomic on interpreted memory. A native call would convert the
// pointer argument to a native shadow and copy it back after the call
// (natConv write-backs), which loses concurrent updates: 200 goroutines
// adding 100 each to an interpreted int64 got 4304 instead of 20000. These
// hooks operate on the interpreted cell itself:
//
//   - integers live in Value.N, so the operations are sync/atomic ones on
//     &cell.N, with a compare-and-swap loop that keeps the canonical form of
//     32-bit values (sign-extended int32, zero-extended uint32);
//   - unsafe.Pointer values live in Value.R (an interface), which sync/atomic
//     cannot update: those operations take a lock chosen by the cell address,
//     the same for every atomic operation on that cell.
//
// Pointers into native memory (a reflect.Value payload) keep the native call.

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

// atomicKind is the integer type an atomic function works on.
type atomicKind uint8

const (
	atomicI32 atomicKind = iota
	atomicI64
	atomicU32
	atomicU64
	atomicUptr
)

// canon returns the VM representation of x truncated to the kind.
func (k atomicKind) canon(x int64) int64 {
	switch k {
	case atomicI32:
		return int64(int32(x))
	case atomicU32:
		return int64(uint32(x))
	case atomicUptr:
		return int64(uintptr(x)) // 32 bits on a 32-bit platform
	}
	return x
}

// atomicCell returns the interpreted cell a pointer argument designates, or
// nil for native memory.
func atomicCell(p Value) *Value {
	c, _ := p.R.(*Value)
	if c == nil && p.R == nil {
		panic(errNilDeref)
	}
	return c
}

// atomicUpdate applies f to the cell value until no other goroutine changed
// it in between, and returns the old and new values.
func atomicUpdate(c *Value, k atomicKind, f func(int64) int64) (old, new int64) {
	for {
		old = atomic.LoadInt64(&c.N)
		new = k.canon(f(old))
		if atomic.CompareAndSwapInt64(&c.N, old, new) {
			return old, new
		}
	}
}

// pointerLocks serialize the atomic operations on unsafe.Pointer cells.
var pointerLocks [64]sync.Mutex

func pointerLock(c *Value) *sync.Mutex {
	return &pointerLocks[(uintptr(unsafe.Pointer(c))>>4)%uintptr(len(pointerLocks))]
}

func init() {
	kinds := map[string]atomicKind{"Int32": atomicI32, "Int64": atomicI64, "Uint32": atomicU32, "Uint64": atomicU64, "Uintptr": atomicUptr}
	for name, k := range kinds {
		k := k
		hook := func(fn string, vm func(c *Value, a []Value) Value) {
			full := "sync/atomic." + fn + name
			vmHooks[full] = func(m *Machine, a []Value) Value {
				if c := atomicCell(a[0]); c != nil {
					return vm(c, a)
				}
				return m.atomicNative(full, a)
			}
		}
		hook("Add", func(c *Value, a []Value) Value {
			_, n := atomicUpdate(c, k, func(x int64) int64 { return x + a[1].N })
			return Value{N: n}
		})
		hook("And", func(c *Value, a []Value) Value {
			o, _ := atomicUpdate(c, k, func(x int64) int64 { return x & a[1].N })
			return Value{N: o}
		})
		hook("Or", func(c *Value, a []Value) Value {
			o, _ := atomicUpdate(c, k, func(x int64) int64 { return x | a[1].N })
			return Value{N: o}
		})
		hook("Load", func(c *Value, a []Value) Value { return Value{N: atomic.LoadInt64(&c.N)} })
		hook("Store", func(c *Value, a []Value) Value { atomic.StoreInt64(&c.N, k.canon(a[1].N)); return Value{} })
		hook("Swap", func(c *Value, a []Value) Value { return Value{N: atomic.SwapInt64(&c.N, k.canon(a[1].N))} })
		hook("CompareAndSwap", func(c *Value, a []Value) Value {
			return Value{N: b2i(atomic.CompareAndSwapInt64(&c.N, k.canon(a[1].N), k.canon(a[2].N)))}
		})
	}

	ptr := func(fn string, vm func(c *Value, a []Value) Value) {
		full := "sync/atomic." + fn
		vmHooks[full] = func(m *Machine, a []Value) Value {
			c := atomicCell(a[0])
			if c == nil {
				return m.atomicNative(full, a)
			}
			mu := pointerLock(c)
			mu.Lock()
			defer mu.Unlock()
			return vm(c, a)
		}
	}
	// The mutex orders the hooks between them; loadWord and storeWord keep
	// a plain read of the same cell (p := ptr, racy as in Go) from seeing
	// half of a write (wordcell.go).
	ptr("LoadPointer", func(c *Value, a []Value) Value { return loadWord(c) })
	ptr("StorePointer", func(c *Value, a []Value) Value { storeWord(c, a[1]); return Value{} })
	ptr("SwapPointer", func(c *Value, a []Value) Value { old := loadWord(c); storeWord(c, a[1]); return old })
	ptr("CompareAndSwapPointer", func(c *Value, a []Value) Value {
		if !samePointer(loadWord(c), a[1]) {
			return Value{N: 0}
		}
		storeWord(c, a[2])
		return Value{N: 1}
	})
}

// samePointer compares two unsafe.Pointer values (interpreted or native).
func samePointer(x, y Value) bool {
	if x.R == nil || y.R == nil {
		return x.R == nil && y.R == nil
	}
	return ptrOf(x) == ptrOf(y)
}

// atomicNatives are the functions behind the hooks, for native memory.
var atomicNatives = map[string]reflect.Value{}

func init() {
	for name, f := range map[string]any{
		"AddInt32": atomic.AddInt32, "AddInt64": atomic.AddInt64, "AddUint32": atomic.AddUint32, "AddUint64": atomic.AddUint64, "AddUintptr": atomic.AddUintptr,
		"AndInt32": atomic.AndInt32, "AndInt64": atomic.AndInt64, "AndUint32": atomic.AndUint32, "AndUint64": atomic.AndUint64, "AndUintptr": atomic.AndUintptr,
		"OrInt32": atomic.OrInt32, "OrInt64": atomic.OrInt64, "OrUint32": atomic.OrUint32, "OrUint64": atomic.OrUint64, "OrUintptr": atomic.OrUintptr,
		"LoadInt32": atomic.LoadInt32, "LoadInt64": atomic.LoadInt64, "LoadUint32": atomic.LoadUint32, "LoadUint64": atomic.LoadUint64, "LoadUintptr": atomic.LoadUintptr,
		"StoreInt32": atomic.StoreInt32, "StoreInt64": atomic.StoreInt64, "StoreUint32": atomic.StoreUint32, "StoreUint64": atomic.StoreUint64, "StoreUintptr": atomic.StoreUintptr,
		"SwapInt32": atomic.SwapInt32, "SwapInt64": atomic.SwapInt64, "SwapUint32": atomic.SwapUint32, "SwapUint64": atomic.SwapUint64, "SwapUintptr": atomic.SwapUintptr,
		"CompareAndSwapInt32": atomic.CompareAndSwapInt32, "CompareAndSwapInt64": atomic.CompareAndSwapInt64,
		"CompareAndSwapUint32": atomic.CompareAndSwapUint32, "CompareAndSwapUint64": atomic.CompareAndSwapUint64, "CompareAndSwapUintptr": atomic.CompareAndSwapUintptr,
		"LoadPointer": atomic.LoadPointer, "StorePointer": atomic.StorePointer, "SwapPointer": atomic.SwapPointer, "CompareAndSwapPointer": atomic.CompareAndSwapPointer,
	} {
		atomicNatives["sync/atomic."+name] = reflect.ValueOf(f)
	}
}

// atomicNative calls the native function on a pointer into native memory.
func (m *Machine) atomicNative(name string, a []Value) Value {
	f := atomicNatives[name]
	ft := f.Type()
	in := make([]reflect.Value, len(a))
	for i := range a {
		t := ft.In(i)
		switch {
		case i == 0:
			if bv, ok := a[0].R.(*byteView); ok {
				// A variable in real bytes (byteview.go): the Go pointer of
				// its bytes. No pointer word lives in bytes, the value must
				// be inside them, and aligned: a misaligned atomic stops the
				// process on some systems.
				et := t.Elem()
				switch {
				case bv.ro:
					refuseROWrite()
				case et.Kind() == reflect.UnsafePointer:
					unsupported("%s on plain data in bytes", name)
				case bv.off+int(et.Size()) > len(bv.b):
					unsupported("unsafe: a %s at offset %d reaches outside a %d-byte object", et, bv.off, len(bv.b))
				case uintptr(unsafe.Pointer(bv.ptr()))%uintptr(et.Align()) != 0:
					unsupported("%s on a misaligned address", name)
				}
				in[i] = reflect.NewAt(et, unsafe.Pointer(bv.ptr()))
				break
			}
			rv, ok := a[0].R.(reflect.Value)
			if !ok {
				unsupported("%s on %T", name, a[0].R)
			}
			if !strings.HasPrefix(name, "sync/atomic.Load") {
				m.hostWritePtr(rv)
			}
			in[i] = rv.Convert(t)
		case t.Kind() == reflect.UnsafePointer:
			in[i] = reflect.ValueOf(ptrOf(a[i]))
		default:
			in[i] = reflect.New(t).Elem()
			if t.Kind() >= reflect.Uint && t.Kind() <= reflect.Uintptr {
				in[i].SetUint(uint64(a[i].N))
			} else {
				in[i].SetInt(a[i].N)
			}
		}
	}
	out := f.Call(in)
	if len(out) == 0 {
		return Value{}
	}
	r := out[0]
	switch r.Kind() {
	case reflect.Bool:
		return Value{N: b2i(r.Bool())}
	case reflect.UnsafePointer:
		return Value{R: r}
	case reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Value{N: int64(r.Uint())}
	}
	return Value{N: r.Int()}
}
