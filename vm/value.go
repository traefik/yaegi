package vm

import (
	"go/types"
	"reflect"
	"sync"
	"sync/atomic"
	"weak"

	"golang.org/x/tools/go/ssa"
)

// Value is the uniform representation of every interpreted value.
// No reflect.Value is used for interpreted data:
//   - ints, bools, floats (bits), runes: N
//   - string: R = string
//   - slice / array / struct: R = []Value (structs and arrays copied on Load/Store)
//   - pointer to interpreted memory: R = *Value
//   - interface: R = *Iface (nil interface: R == nil)
//   - func value: R = *Closure
//   - map: R = *MapV
//   - native (compiled) value crossing the boundary: R = reflect.Value
type Value struct {
	N int64
	R any
}

// Iface is an interface value: dynamic type + payload.
type Iface struct {
	T *RType
	V Value
}

// Closure is an interpreted function value.
type Closure struct {
	Fn  *Func
	Env []Value
	Go  func(args []Value) Value // VM intrinsic implemented in Go (Fn == nil)
	// Direct, with Go, is the interpreted method that Go calls, with the
	// receiver Recv first when it is not nil (method values and method
	// expressions made by reflect). defer and go call it directly, so that
	// recover works in it as in gc.
	Direct *Func
	Recv   *Value
	// GoP, with Go, is Go called by a deferred call: p is the panic that
	// runs the defer (nil at a normal return). Its first interpreted call
	// gets p, so that recover works there (functions made by
	// reflect.MakeFunc).
	GoP func(p *panicState, args []Value) Value
}

// MapV is an interpreted map. M is indexed by a comparable Go key derived from
// the interpreted key (keyOf); K keeps the original key Values for range.
// An entry with a NaN key is stored under its own nanKey (storeKey): a
// lookup never finds it, as in Go, but M and K keep it under the same key.
//
// K is nil while every key Value is the one that its derived key gives back
// (rebuildKey): a string, or an integer or a bool. Most maps have such keys,
// and then a store writes one Go map, not two. The first key that cannot be
// rebuilt makes K (setKey).
type MapV struct {
	M      map[any]Value
	K      map[any]Value
	clears int // number of clear(m) calls: NaN keys can only go away by clear
	// writing is 1 while the VM writes to the map, and readers counts the
	// reads of the VM, with Options.PanicOnFatal (fatal.go).
	writing, readers atomic.Int32
}

// newMapV returns an empty interpreted map, with room for n entries.
func newMapV(n int) *MapV { return &MapV{M: make(map[any]Value, n)} }

// key returns the original key Value of the entry that M keeps under k.
func (mp *MapV) key(k any) Value {
	if mp.K != nil {
		return mp.K[k]
	}
	return rebuildKey(k)
}

// rebuildKey returns the key Value whose derived key is k, when k is a
// string or an int64 (keyOf of a string, an integer or a bool).
func rebuildKey(k any) Value {
	if n, ok := k.(int64); ok {
		return Value{N: n}
	}
	return Value{R: k}
}

// setKey records kv as the original key of the entry under k, after the
// entry is stored in M.
func (mp *MapV) setKey(k any, kv Value) {
	if mp.K == nil {
		switch x := k.(type) {
		case string:
			if s, ok := kv.R.(string); ok && kv.N == 0 && s == x && optMapKeys {
				return
			}
		case int64:
			if kv.R == nil && kv.N == x && optMapKeys {
				return
			}
		}
		// The first key that rebuildKey cannot give back: K gets the keys
		// of the other entries, which it can.
		mp.K = make(map[any]Value, len(mp.M))
		for k2 := range mp.M {
			mp.K[k2] = rebuildKey(k2)
		}
	}
	mp.K[k] = kv
}

// deleteKey removes the original key of the entry under k.
func (mp *MapV) deleteKey(k any) {
	if mp.K != nil {
		delete(mp.K, k)
	}
}

// clear removes every entry of mp, like clear(m) in Go. It also counts the
// call: a range loop over mp stops giving its NaN keys after a clear (NaN
// keys cannot be deleted in another way, see execRange).
func (mp *MapV) clear() {
	clear(mp.M)
	if mp.K != nil {
		clear(mp.K)
	}
	mp.clears++
}

// each calls f for each entry with its original key Value.
func (mp *MapV) each(f func(k, v Value)) {
	for k, v := range mp.M {
		f(mp.key(k), v)
	}
}

// nanKey is the M and K key of an entry whose key is NaN (or holds a NaN):
// each such entry is a new key, which no lookup can build.
type nanKey struct{ id uint64 }

var nanKeyIDs atomic.Uint64

// storeKey returns the key under which an entry with the derived key k is
// stored: k itself, or a new nanKey when k is not equal to itself.
func storeKey(k any) any {
	if k != k {
		return nanKey{nanKeyIDs.Add(1)}
	}
	return k
}

// RType is the runtime descriptor of a dynamic type stored in an interface.
type RType struct {
	T       types.Type // interpreted type (nil for purely native dynamic types)
	Methods map[string]*Func
	Native  reflect.Type // non-nil when payload is a native reflect.Value
	noCmp   bool         // T is not comparable: == on two such values panics
	// impl caches implements: *types.Interface -> bool. Reads take no
	// lock (a type assertion to an interface type runs on every request,
	// on every goroutine); a miss computes the answer under implMu.
	impl   sync.Map
	implMu sync.Mutex
	ptr    atomic.Uint32 // ptrInfo of a pointer type T (0: not computed)
	// natWrong caches nativeWrong (aotroute.go): 0 unknown, 1 false, 2 true.
	natWrong atomic.Int32
	// optLast is the last payload of the wrappers of a value of T
	// (optPayload, optwrap.go): a value given again and again to native
	// code finds it without building its key. It is weak: the key of the
	// cleanup of a payload holds T, so a strong pointer would keep the
	// payload, and what it holds, alive forever.
	optLast atomic.Pointer[weak.Pointer[Iface]]
	// optFlt caches optFloats (optwrap.go): 0 unknown, else optNoFloat,
	// optFloat or optIface.
	optFlt atomic.Int32
	// optFlat caches optFlat (optwrap.go): 0 unknown, 1 false, 2 true.
	optFlat atomic.Int32
	// nests caches holdsIface (deepprint.go): 0 unknown, 1 false, 2 true.
	nests atomic.Int32
	// nat is set for a comparable named native type of the program
	// (json.Delim, time.Duration): the key of its values in a map, which
	// must be the key of the same value given by native code (keyOf).
	nat *natKey
	// boxes are the static boxes of the values of this type in an
	// interface (boxes.go), made at the first use.
	boxes atomic.Pointer[ifaceBoxes]
}

// natKey is the key of the values of a named native type: the RType of the
// native type and the native value, as for a value that native code gave.
type natKey struct {
	rt *RType
	m  *Machine
	t  types.Type
}

// key returns the native value of v, of the named native type k.t.
func (k *natKey) key(v Value) any { return natKeyValue(k.m, v, k.t) }

// natKeyValue is m.toNative(v, t).Interface(), set in init: called directly,
// it makes an initialization cycle (ctxHooks, ..., rtypeOf, toNative).
var natKeyValue func(m *Machine, v Value, t types.Type) any

func init() {
	natKeyValue = func(m *Machine, v Value, t types.Type) any { return m.toNative(v, t).Interface() }
}

func (rt *RType) String() string {
	if rt.T != nil {
		return rt.T.String()
	}
	return rt.Native.String()
}

// Func is a compiled interpreted function (or a native one when ssa has no body).
type Func struct {
	m         *Machine
	ssa       *ssa.Function
	once      sync.Once
	code      []Instr
	nregs     int
	template  []Value // register file initial content (constants pre-loaded)
	nparams   int
	native    reflect.Value
	recoverPC int
	icount    int64 // executed instructions (VM_PROFILE)
	// wrapper: a method wrapper made by ssa (bound method, thunk, promoted
	// or pointer receiver). recover looks through these frames, as in gc.
	wrapper bool
	// pos maps instruction indexes to source positions (callers.go).
	pos []posRun
	// pcID numbers fn in the program counters of the stack (callers.go);
	// 0 until a stack walk needs it.
	pcID atomic.Int32

	// For runtime.GC (gclive.go): start pc of each SSA instruction in block
	// order, and its register range (first, count; count 0: no register).
	insStart []int32
	insReg   [][2]int32
	gcOnce   sync.Once
	gc       *gcInfo
	// problems: the instructions that could not be compiled. Each one
	// panics when it runs; the load-time check (preflight.go) reports them.
	problems []problem
	// aggVals: the field values of the composite literals built in a
	// register, by the Alloc of the literal (aggv.go, read by gclive.go).
	aggVals map[*ssa.Alloc][]ssa.Value
	// cells: the register that keeps the cell of a frame-local Alloc
	// (escape.go), by the register of the Alloc (read by gclive.go).
	cells map[int32]int32
	// locals: the cell registers of the frame-local Allocs and their
	// sites, which give their cells back to the ctx at a return (escape.go)
	locals []localCell
}

type Op uint8

const (
	OpNop Op = iota
	OpMov
	OpJmp
	OpIf
	OpRet
	OpAddI
	OpSubI
	OpMulI
	OpDivI
	OpRemI
	OpDivU
	OpRemU
	OpAndI
	OpOrI
	OpXorI
	OpAndNotI
	OpShlI
	OpShrI
	OpShrU
	OpEqI
	OpNeI
	OpLtI
	OpLeI
	OpGtI
	OpGeI
	OpLtU
	OpLeU
	OpGtU
	OpGeU
	OpAddF
	OpSubF
	OpMulF
	OpDivF
	OpLtF
	OpLeF
	OpGtF
	OpGeF
	OpEqF
	OpAddS
	OpEqS
	OpNeS
	OpLtS
	OpEqV
	OpNeV
	OpNegI
	OpNegF
	OpNot
	OpCompl
	OpTrunc
	OpI2F
	OpF2I
	OpStr2Bytes
	OpBytes2Str
	OpRune2Str
	OpCall
	OpCallClosure
	OpCallNative
	OpInvoke
	OpMakeClosure
	OpAlloc
	OpLoad
	OpStore
	OpFieldAddr
	OpField
	OpIndexAddr
	OpIndex
	OpSlice
	OpMakeSlice
	OpLen
	OpCap
	OpAppend
	OpCopy
	OpMakeInterface
	OpTypeAssert
	OpExtract
	OpMakeMap
	OpMapUpdate
	OpLookup
	OpPrintln
	OpPanic
	OpDefer
	OpRunDefers
	OpRecover
	OpDeferStack // A = the defer stack of the current frame (ssa:deferstack)
	OpGo
	OpMakeChan
	OpSend
	OpRecv
	OpClose
	OpSelect
	OpRange
	OpNext
	OpDelete
	OpFunc // rare operations implemented as a Go closure over the register file

	// Optimization 6: superinstructions.
	OpFieldLoad  // A = *(&B.field C)            (FieldAddr+Load)
	OpFieldStore // *(&A.field C) = B            (FieldAddr+Store)
	OpIndexLoad  // A = *(&B[C])                 (IndexAddr+Load)
	OpIndexStore // *(&A[C]) = B                 (IndexAddr+Store)
	OpJEqI       // if A == B goto C             (compare+If, fall through on false)
	OpJNeI
	OpJLtI
	OpJLeI
	OpJGtI
	OpJGeI
	OpJLtU
	OpJLeU
	OpJGtU
	OpJGeU
	OpJEqS
	OpJNeS

	OpStr2Runes  // A = []rune(B)
	OpRunes2Str  // A = string(B), B a []rune
	OpWrapNilChk // A = B, or panic X (an error) if B is a nil pointer
	OpGC         // runtime.GC(): clear dead registers first (gclive.go)

	OpCtxFunc // A = X(cx, regs): a Go call that reads the stack of cx (callers.go)

	OpAppendV // A = append(B, elements...) with the elements in registers (appendv.go)
	OpMakeAgg // A = a struct or array value built from registers (aggv.go)

	OpSwitch // goto the pc of the value of A in the table X (switches.go)
	OpMovJmp // A = B; goto C (the last phi move of an edge and its jump)

	OpMakeIfaceBox // A = B in an interface, with a static box when there is one (boxes.go)

	OpAllocLocal // A = the cell in register B, set to zero again, or a new one stored in B (escape.go)

	OpStoreAgg // *A = a struct or array literal built from registers, written in place when it can (aggv.go)
)

// Instr is one VM instruction. A is usually the destination register.
type Instr struct {
	Op Op
	// F holds the flags of some instructions (idxPtr and idxUnsigned for
	// the index instructions, shiftSigned for the shifts). It is kept apart
	// from T, so a flag is never read as a tuple size.
	F uint8
	// T is the number of result registers when the result is a tuple
	// (see put and putOk); 0 or 1 otherwise.
	T       int16
	A, B, C int32
	X       any
}
