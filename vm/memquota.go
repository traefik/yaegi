package vm

// An approximate memory quota per plugin (Options.MaxMemory).
//
// Go has no memory limit per part of a program: debug.SetMemoryLimit is
// for the whole process, and a plugin that allocates in a loop ends the
// host with a fatal "out of memory". The VM counts the large allocations
// that it makes for the plugin, and refuses the one that would take the
// plugin past its quota with the panic ErrMemory.
//
// What is counted: the allocations of memMin bytes or more made by the
// instructions of the VM: make of a slice (before the allocation), the new
// array of an append that grows its slice, a large array (new, a composite
// literal, a local that escapes), a string concatenation. The count is of
// live memory: each counted allocation has a weak pointer, and the bytes of
// the ones that the GC freed are given back. The count is updated lazily:
// when an allocation would go past the quota, the dead objects are removed
// first, and if that is not enough, a garbage collection runs and they are
// removed again. At most one such collection runs every gcEvery in the
// whole process (forcedGC), whatever the number of machines, and a machine
// whose collection did not make room waits twice as long before the next
// one, up to gcMaxWait (memQuota.gcWait): an allocation that needs a
// collection sooner waits for its turn, without the lock of the quota. So a
// plugin that makes and drops large buffers faster than the GC frees them
// is slowed down, not refused, and a plugin that recovers ErrMemory in a
// loop (or many such plugins) cannot make the host collect all the time.
//
// Native calls whose result size is known from their arguments
// (strings.Repeat, bytes.Repeat, strings.Builder.Grow, bytes.Buffer.Grow,
// reflect.MakeSlice) are checked before the call, and io.ReadAll checks each
// growth of its buffer (nativemem.go): a huge request is ErrMemory, not a
// fatal "out of memory" of the host.
//
// What is not counted, so the guarantee is weak: the allocations below
// memMin (a map that grows one small entry at a time, many small structs,
// many 32 KiB buffers kept in a global), the other allocations of native
// code (encoding/json of a huge input, io.Copy into a bytes.Buffer, the AOT
// copies), and the memory of the VM itself. A quota stops a
// plugin that makes large buffers in a loop or keeps them in a global; it
// does not bound the memory of a plugin written to get around it. The real
// bound stays a memory limit for the process (GOMEMLIMIT, a cgroup).

import (
	"errors"
	"go/types"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"
)

// ErrMemory is the panic of an allocation of the plugin that would take it
// past Options.MaxMemory. The plugin can recover it: the allocation was not
// made, or is dropped.
var ErrMemory = errors.New("vm: memory quota of the plugin exceeded (MaxMemory)")

const (
	memMin    = 64 << 10              // smaller allocations are not counted
	gcEvery   = 50 * time.Millisecond // at most one forced GC per period, in the process
	gcMaxWait = 2 * time.Second       // longest wait of a machine whose GCs do not make room
)

// memQuota is the memory count of a machine with a MaxMemory.
type memQuota struct {
	max     int64
	mu      sync.Mutex
	used    int64          // bytes of the counted objects that may be alive
	objs    []memObj       // the counted objects
	sweepAt int            // len(objs) at which add removes the dead ones
	stacks  []*stackCharge // the charges of the stacks of the ctxs (stacklimit.go)
	stackAt int            // len(stacks) at which a new charge removes the dead ones
	gcWait  time.Duration  // wait between two forced GCs of this machine (0: gcEvery)
	nextGC  time.Time      // when this machine may force the next GC
}

type memObj struct {
	p weak.Pointer[byte]
	n int64
}

// stackCharge is the part of the stack of a ctx (its frame-local cells and
// deferred calls) counted in a quota (stacklimit.go). Unlike a memObj, the
// ctx gives the bytes back itself when it ends (ctx.releaseMem), and a ctx
// of the pool keeps its stackCharge from one call to the next, so a call
// adds no object to the quota. tok is a weak pointer to a small object of
// the ctx: when a ctx that did not give its charge back (a panic left the
// call) is freed by the GC, sweep gives the bytes back.
type stackCharge struct {
	q    *memQuota
	tok  weak.Pointer[[16]byte]
	n    int64 // bytes counted; q.mu is held to change it
	gone bool  // the ctx no longer uses it: sweep drops it
}

// forcedGC spaces the garbage collections that the quotas force: at most one
// every gcEvery in the process. Before, each machine had its own period, so
// a plugin per core that recovered ErrMemory in a loop made the host
// collect all the time.
var forcedGC struct {
	mu    sync.Mutex
	start time.Time // start of the last forced collection
	last  time.Time // end of the last forced collection
}

// fits reports whether n more bytes fit in the quota, after removing the
// dead objects. q.mu is held.
func (q *memQuota) fits(n int64) bool {
	if q.used+n <= q.max {
		return true
	}
	q.sweep()
	return q.used+n <= q.max
}

// reserve counts n more bytes in the quota, for the object at p (nil: a
// check before an allocation), or reports false when they do not fit after
// a garbage collection. q.mu is not held: the waits for the collection
// happen without it, so the other goroutines of the plugin go on.
func (q *memQuota) reserve(n int64, p unsafe.Pointer) bool {
	return q.reserveFor(n, p, nil)
}

// reserveFor is reserve, with the n bytes counted in the stack charge sc
// when sc is not nil.
func (q *memQuota) reserveFor(n int64, p unsafe.Pointer, sc *stackCharge) bool {
	q.mu.Lock()
	if q.fits(n) {
		q.addFor(n, p, sc)
		q.mu.Unlock()
		return true
	}
	wait := q.gcWait
	if wait == 0 {
		wait = gcEvery
	}
	next := q.nextGC
	q.nextGC = time.Now().Add(wait) // the next goroutine of the plugin waits
	q.mu.Unlock()
	collect(next)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fits(n) {
		q.gcWait = gcEvery
		q.addFor(n, p, sc)
		return true
	}
	q.gcWait = min(2*wait, gcMaxWait)
	q.nextGC = time.Now().Add(q.gcWait)
	return false
}

// tryReserveFor is reserveFor with no forced garbage collection: it only
// removes the objects that the GC already freed.
func (q *memQuota) tryReserveFor(n int64, sc *stackCharge) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.fits(n) {
		return false
	}
	q.addFor(n, nil, sc)
	return true
}

// collect runs a garbage collection, not before next (the turn of the
// machine) and not within gcEvery of the last forced one (forcedGC). A
// collection that started after the call of collect counts as its own: it
// frees all that was dead when collect was called. One that only ended
// after it does not: it can have started before the caller dropped its
// objects (before, the caller then found the quota still full of dead
// objects, and got ErrMemory).
func collect(next time.Time) {
	start := time.Now()
	if d := time.Until(next); d > 0 {
		time.Sleep(d)
	}
	forcedGC.mu.Lock()
	defer forcedGC.mu.Unlock()
	if forcedGC.start.After(start) {
		return
	}
	if d := gcEvery - time.Since(forcedGC.last); d > 0 {
		time.Sleep(d)
	}
	forcedGC.start = time.Now()
	runtime.GC()
	forcedGC.last = time.Now()
}

// addFor counts the object of n bytes at p (nil: nothing to count), or n
// more bytes in the stack charge sc when sc is not nil. q.mu is held.
func (q *memQuota) addFor(n int64, p unsafe.Pointer, sc *stackCharge) {
	if sc != nil {
		q.used += n
		sc.n += n
		return
	}
	q.add(n, p)
}

// add counts the object of n bytes at p (nil: nothing to count). q.mu is
// held.
func (q *memQuota) add(n int64, p unsafe.Pointer) {
	if p == nil {
		return
	}
	q.used += n
	q.objs = append(q.objs, memObj{weak.Make((*byte)(p)), n})
	if len(q.objs) >= q.sweepAt {
		q.sweep()
	}
}

// sweep removes the objects that the GC freed. q.mu is held.
func (q *memQuota) sweep() {
	live := q.objs[:0]
	for _, o := range q.objs {
		if o.p.Value() == nil {
			q.used -= o.n
			continue
		}
		live = append(live, o)
	}
	clear(q.objs[len(live):])
	q.objs = live
	q.sweepAt = 2*len(live) + 64
	q.sweepStacks()
}

// sweepStacks removes the stack charges that no ctx uses, and gives back
// the bytes of the ones whose ctx the GC freed. q.mu is held.
func (q *memQuota) sweepStacks() {
	live := q.stacks[:0]
	for _, sc := range q.stacks {
		if sc.tok.Value() == nil {
			q.used -= sc.n
			sc.n, sc.gone = 0, true
		}
		if !sc.gone {
			live = append(live, sc)
		}
	}
	clear(q.stacks[len(live):])
	q.stacks = live
	q.stackAt = 2*len(live) + 64
}

// newStackCharge returns a stack charge of q held by tok. q.mu is not held.
func (q *memQuota) newStackCharge(tok *[16]byte) *stackCharge {
	sc := &stackCharge{q: q, tok: weak.Make(tok)}
	q.mu.Lock()
	if len(q.stacks) >= q.stackAt {
		q.sweepStacks()
	}
	q.stacks = append(q.stacks, sc)
	q.mu.Unlock()
	return sc
}

// release gives back the bytes of the stack charge sc, and drops it from
// its quota when gone is true.
func (sc *stackCharge) release(gone bool) {
	q := sc.q
	q.mu.Lock()
	q.used -= sc.n
	sc.n = 0
	sc.gone = sc.gone || gone
	q.mu.Unlock()
}

// memCheck panics with ErrMemory when n more bytes do not fit in the quota
// of m (before an allocation of n bytes).
func (m *Machine) memCheck(n int64) {
	if !m.mem.reserve(n, nil) {
		panic(ErrMemory)
	}
}

// memAdd counts the object of n bytes that starts at p, or panics with
// ErrMemory when it does not fit in the quota (the object is then dropped).
func (m *Machine) memAdd(p unsafe.Pointer, n int64) {
	if p == nil {
		return
	}
	if !m.mem.reserve(n, p) {
		panic(ErrMemory)
	}
}

// memMakeSlice checks make([]T, c) for elements of entries Value entries
// each (a struct or an array: their own []Value), and returns the bytes to
// count for it (0: below memMin).
func (m *Machine) memMakeSlice(c, entries int64) int64 {
	per := valueSize * (1 + entries)
	if c <= 0 {
		return 0
	}
	n := int64(math.MaxInt64)
	if c <= math.MaxInt64/per {
		n = c * per
	}
	if n < memMin {
		return 0
	}
	m.memCheck(n)
	return n
}

// memBytes returns make([]byte, n), counted in the quota.
func (m *Machine) memBytes(n int) []byte {
	m.memCheck(int64(n))
	b := make([]byte, n)
	m.memSlice(b)
	return b
}

// memSlice counts the array of s (a []Value or a []byte) when it is large.
func (m *Machine) memSlice(s any) {
	m.memSliceOf(s, valueSize)
}

// memSliceOf is memSlice with per bytes for each element of a []Value.
func (m *Machine) memSliceOf(s any, per int64) {
	switch s := s.(type) {
	case []Value:
		if n := satMulBytes(int64(cap(s)), per); n >= memMin {
			m.memAdd(unsafe.Pointer(unsafe.SliceData(s[:cap(s)])), n)
		}
	case []byte:
		if n := int64(cap(s)); n >= memMin {
			m.memAdd(unsafe.Pointer(unsafe.SliceData(s[:cap(s)])), n)
		}
	}
}

// satMulBytes is a*b, or math.MaxInt64 when it overflows (a, b >= 0).
func satMulBytes(a, b int64) int64 {
	if b != 0 && a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// memAppend counts the result r of an append to old (elements of type et,
// nil: unknown) when the append made a new large array. When the elements
// are structs or arrays, the growth also made a copy of each of them
// (appendValues): they count with the array.
func (m *Machine) memAppend(old, r any, et types.Type) {
	switch r := r.(type) {
	case []Value:
		if o, _ := old.([]Value); cap(r) != cap(o) || unsafe.SliceData(r[:cap(r)]) != unsafe.SliceData(o[:cap(o)]) {
			per := int64(valueSize)
			if et != nil && int64(cap(r))*valueSize < memMin {
				et = nil // a small array: no need for the size of the elements
			}
			if et != nil && needsCopy(et) {
				per = satMulBytes(valueSize, 1+min(m.zeroEntries(et), maxZeroElems+1))
			}
			m.memSliceOf(r, per)
		}
	case []byte:
		if o, _ := old.([]byte); cap(r) != cap(o) || unsafe.SliceData(r[:cap(r)]) != unsafe.SliceData(o[:cap(o)]) {
			m.memSlice(r)
		}
	}
}

// memString counts the string s made by a concatenation when it is large.
func (m *Machine) memString(s string) {
	if n := int64(len(s)); n >= memMin {
		m.memAdd(unsafe.Pointer(unsafe.StringData(s)), n)
	}
}

// Memory returns the bytes that the memory quota counts for the plugin
// (Options.MaxMemory): the large allocations of the VM that may still be
// alive. 0 without a quota.
func (m *Machine) Memory() int64 {
	q := m.mem
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sweep()
	return q.used
}

// memGlobal checks the zero value of the package-level variable g of type t
// against the memory quota, before it is allocated: a plugin that declares
// var big [60 << 20]byte would need 1.5 GB of Values, below the bound of
// one value (maxZeroElems), and the host would allocate it at load time,
// before any code of the plugin runs. It panics with a *LoadError when the
// value does not fit in the quota, and returns the bytes to count for it
// (0: no quota, or a small value).
func (m *Machine) memGlobal(g *ssa.Global, t types.Type) int64 {
	if m.mem == nil {
		return 0
	}
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
	default:
		return 0
	}
	n := satMulBytes(valueSize, 1+min(m.zeroEntries(t), maxZeroElems+1))
	if n < memMin {
		return 0
	}
	if !m.mem.reserve(n, nil) {
		panic(loadErrorf("vm: package-level variable %s.%s of type %s needs about %d MiB, more than the memory quota of the plugin (MaxMemory, %d MiB)",
			g.Pkg.Pkg.Path(), g.Name(), t.String(), n>>20, m.mem.max>>20))
	}
	return n
}

// lenValues is the length of the []Value of a zero value (1 for others).
func lenValues(r any) int {
	if s, ok := r.([]Value); ok {
		return len(s)
	}
	return 1
}
