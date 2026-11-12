package vm

import (
	"go/types"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// A pointer, a map, a chan or a func is one machine word in Go, so a racy
// read of such a variable sees the old or the new value, never a mix: many
// plugins swap a config pointer from a reload goroutine without a lock
// while ServeHTTP reads it. In the VM the same variable is a Value of three
// words (N, and the type and data words of R). A racy read could see the
// type word of one write and the data word of another: a non nil R with a
// nil data word (a nil dereference that Go never gives), or worse, the type
// of an interpreted *Closure with the data of a native reflect.Value (type
// confusion, a crash of the host).
//
// A cell whose static type is one of these kinds (wordKind) is read and
// written under a seqlock: a writer makes the sequence odd, writes, and
// makes it even again; a reader reads the words with atomic loads and tries
// again when the sequence was odd or changed. The sequences are striped by
// the address of the cell (the Go heap does not move). A reader writes no
// shared memory, so many goroutines that read the same hot variable do not
// contend; on amd64 the atomic loads are plain loads. An interface is two
// words in Go too and may tear there, but in the VM its R may hold an
// *Iface or a native reflect.Value: it gets the same lock, so that a torn
// read never confuses two Go types.

// wordKind reports whether a variable of type t is one machine word in Go
// (or an interface), so that a racy read must not tear.
func wordKind(t types.Type) bool {
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	}
	return false
}

const wordStripes = 1 << 10

// wordSeq is one stripe, on its own cache line so that writes to one
// variable do not slow down the readers of another.
type wordSeq struct {
	n atomic.Uint32
	_ [60]byte
}

var wordSeqs [wordStripes]wordSeq

func wordStripe(p *Value) *atomic.Uint32 {
	return &wordSeqs[(uintptr(unsafe.Pointer(p))>>4)%wordStripes].n
}

// valueWords is the memory layout of a Value: N, then the type and data
// words of R.
type valueWords struct {
	n int64
	t unsafe.Pointer
	d unsafe.Pointer
}

// loadWord reads *p as one snapshot: never a mix of two writes made with
// storeWord. The loop is in loadWordSlow, so that the first try has no
// loop.
func loadWord(p *Value) Value {
	seq := wordStripe(p)
	if v, ok := readWords(p, seq, seq.Load()); ok {
		return v
	}
	return loadWordSlow(p, seq)
}

func loadWordSlow(p *Value, seq *atomic.Uint32) Value {
	for spin := 0; ; spin++ {
		if spin&63 == 63 {
			runtime.Gosched()
		}
		if v, ok := readWords(p, seq, seq.Load()); ok {
			return v
		}
	}
}

// readWords reads the words of *p with atomic loads: the Go memory model
// keeps them in order with the loads of seq (a plain load could move after
// the second load of seq on arm64). A pointer read with a racy load is
// safe for the GC, as in Go. ok is false when a write ran meanwhile.
func readWords(p *Value, seq *atomic.Uint32, s1 uint32) (v Value, ok bool) {
	w := (*valueWords)(unsafe.Pointer(p))
	n := loadN(&w.n)
	t := atomic.LoadPointer(&w.t)
	d := atomic.LoadPointer(&w.d)
	if s1&1 != 0 || seq.Load() != s1 {
		return Value{}, false
	}
	vw := (*valueWords)(unsafe.Pointer(&v))
	vw.n, vw.t, vw.d = n, t, d
	return v, true
}

// storeWord writes *p = v so that loadWord never sees half of it.
//
// The words are written with atomic stores, not with *p = v. Go atomics
// are sequentially consistent, so a reader that sees one of the new words
// sees the odd sequence on its second load, and tries again. With a plain
// store this holds on amd64 (stores are not reordered there) but not on
// arm64: without LSE (GOARM64=v8.0) the CompareAndSwap is an LDAXR/STLXR
// loop, and a later plain store may become visible before the STLXR, so a
// reader could see an even sequence, torn words, and the same even
// sequence again. Go has no fence alone; the atomic stores are the
// cheapest correct way (on amd64 three XCHG instead of three MOV, on a
// path that is much less hot than the loads).
func storeWord(p *Value, v Value) {
	seq := wordStripe(p)
	for spin := 0; ; spin++ {
		s := seq.Load()
		if s&1 == 0 && seq.CompareAndSwap(s, s+1) {
			storeWords(p, v)
			seq.Store(s + 2)
			return
		}
		if spin&63 == 63 {
			runtime.Gosched()
		}
	}
}

// storeWords writes the words of v into *p with atomic stores. Both words
// of R are pointers for the GC (as in any interface), so StorePointer has
// the write barrier that *p = v would have.
func storeWords(p *Value, v Value) {
	w := (*valueWords)(unsafe.Pointer(p))
	vw := (*valueWords)(unsafe.Pointer(&v))
	storeN(&w.n, vw.n)
	atomic.StorePointer(&w.t, vw.t)
	atomic.StorePointer(&w.d, vw.d)
}

// loadN is an atomic load of N. On 32-bit platforms an int64 field may be
// only 4-byte aligned, which atomic.LoadInt64 refuses: N is then read as two
// atomic halves (the sequence check makes the pair consistent).
func loadN(p *int64) int64 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return atomic.LoadInt64(p)
	}
	h := (*[2]uint32)(unsafe.Pointer(p))
	a, b := atomic.LoadUint32(&h[0]), atomic.LoadUint32(&h[1])
	var n int64
	nh := (*[2]uint32)(unsafe.Pointer(&n))
	nh[0], nh[1] = a, b
	return n
}

// storeN is an atomic store of N, in two atomic halves on 32-bit platforms
// (see loadN).
func storeN(p *int64, n int64) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		atomic.StoreInt64(p, n)
		return
	}
	h := (*[2]uint32)(unsafe.Pointer(p))
	nh := (*[2]uint32)(unsafe.Pointer(&n))
	atomic.StoreUint32(&h[0], nh[0])
	atomic.StoreUint32(&h[1], nh[1])
}

// copyWords is copy(dst, src) for elements of a wordKind type: each element
// is read with loadWord and written with storeWord, so that a racy reader
// of dst (or a racy writer of src) never sees half of an element. dst and
// src can overlap, as in Go.
func copyWords(dst, src []Value) int {
	n := min(len(dst), len(src))
	if n == 0 {
		return 0
	}
	if uintptr(unsafe.Pointer(&dst[0])) > uintptr(unsafe.Pointer(&src[0])) {
		for i := n - 1; i >= 0; i-- {
			storeWord(&dst[i], loadWord(&src[i]))
		}
		return n
	}
	for i := 0; i < n; i++ {
		storeWord(&dst[i], loadWord(&src[i]))
	}
	return n
}

// appendWords is append(s, t...) for elements of a wordKind type. When s
// has room, the new elements go into memory that another slice may share
// and read: they are written with storeWord. A new array is not seen by
// anyone yet: a plain append.
func appendWords(s, t []Value) []Value {
	old := len(s)
	if cap(s)-old < len(t) {
		return append(s, t...)
	}
	s = s[:old+len(t)]
	copyWords(s[old:], t)
	return s
}

// swapWords swaps s[i] and s[j], elements of a wordKind type, with loadWord
// and storeWord (sort.Slice, reflect.Swapper).
func swapWords(s []Value, i, j int) {
	a, b := loadWord(&s[i]), loadWord(&s[j])
	storeWord(&s[i], b)
	storeWord(&s[j], a)
}

// storeCell writes v into the cell p of type t: with storeWord when t is a
// wordKind, else as it is.
func storeCell(p *Value, v Value, t types.Type) {
	if wordKind(t) {
		storeWord(p, v)
		return
	}
	*p = v
}

// loadCell reads the cell p of type t: with loadWord when t is a wordKind
// (a field or an element of a struct or array copied as a whole, which a
// racy writer may change), else as it is.
func loadCell(p *Value, t types.Type) Value {
	if wordKind(t) {
		return loadWord(p)
	}
	return *p
}
