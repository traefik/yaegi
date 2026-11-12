package vm

// Generic functions and methods of native standard library packages.
//
// Go has no instantiation at run time: a native package holds only the
// instances its own binary needed, so an instance of a generic function of
// the standard library for a type of the plugin (atomic.Pointer[Config],
// rand.N[time.Duration]) does not exist natively. Two ways run them:
//
//   - the VM interprets the source of the package. stdtypes embeds the
//     sources of cmp, iter, slices and maps, which are plain Go;
//   - the VM implements the function itself: genericImpls below.
//
// The other generic APIs of the standard library reach runtime internals
// from their source (linkname, internal/abi.Escape, an unexported global
// like the source of math/rand/v2, crypto/internal/fips140only), so their
// source cannot run in the VM, with any loader. Each one has an entry here,
// marked always: the VM uses it even when a loader built the package from
// source (go list loads that need the full syntax, -src). So the result
// does not depend on the loader or on the other imports of the plugin.
//
// database/sql.Null[T] works only for some T: see sqlNullTypes.

import (
	"crypto/hkdf"
	"crypto/pbkdf2"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"go/types"
	"hash"
	"hash/maphash"
	"maps"
	"math/rand/v2"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unsafe"
	"weak"

	"golang.org/x/tools/go/ssa"
)

// genericImpl implements the instances of one generic function or method.
type genericImpl struct {
	// always: also when the package was built from source (its source
	// needs runtime internals). Without it, the source runs when a loader
	// built it, and the VM implementation only replaces a missing body.
	always bool
	// make returns the implementation of instance f; args are the
	// registers of the arguments (the receiver first for a method).
	make func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value
}

// genericImpls are keyed by genericName.
var genericImpls map[string]genericImpl

func init() {
	genericImpls = map[string]genericImpl{
		"iter.Pull":  {true, (*Machine).iterPull},
		"iter.Pull2": {true, (*Machine).iterPull},
		"sync.OnceValue": {false, func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
			return m.onceValue(args)
		}},
		"sync.OnceValues": {false, func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
			return m.onceValue(args)
		}},
		"errors.AsType": {false, func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
			et := f.TypeArgs()[0]
			return func(regs []Value) Value { return m.errorsAsType(regs[args[0]], et) }
		}},
		"unique.Make": {true, func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
			s := m.uniqueSetOf(f.TypeArgs()[0])
			return func(regs []Value) Value { return s.make(regs[args[0]]) }
		}},
		// Handle[T].Value: the value pointer is a native cell (vm/unique.go).
		"unique.Handle.Value": {true, func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
			s := m.uniqueSetOf(f.TypeArgs()[0])
			return func(regs []Value) Value { return s.value(regs[args[0]]) }
		}},
		"sync/atomic.Pointer.Load":           {true, atomicPointerOp("LoadPointer")},
		"sync/atomic.Pointer.Store":          {true, atomicPointerOp("StorePointer")},
		"sync/atomic.Pointer.Swap":           {true, atomicPointerOp("SwapPointer")},
		"sync/atomic.Pointer.CompareAndSwap": {true, atomicPointerOp("CompareAndSwapPointer")},
		"math/rand/v2.N":                     {true, randN},
		"weak.Make":                          {true, weakMake},
		"weak.Pointer.Value":                 {true, weakValue},
		"runtime.AddCleanup":                 {true, addCleanup},
		"hash/maphash.Comparable":            {true, maphashComparable},
		"hash/maphash.WriteComparable":       {true, maphashWriteComparable},
		"crypto/hkdf.Key":                    {true, hashGeneric(hkdf.Key[hash.Hash])},
		"crypto/hkdf.Extract":                {true, hashGeneric(hkdf.Extract[hash.Hash])},
		"crypto/hkdf.Expand":                 {true, hashGeneric(hkdf.Expand[hash.Hash])},
		"crypto/pbkdf2.Key":                  {true, hashGeneric(pbkdf2.Key[hash.Hash])},
		"database/sql.Null.Scan":             {true, sqlNullMethod},
		"database/sql.Null.Value":            {true, sqlNullMethod},
		"slices.overlaps":                    {true, slicesOverlaps},
	}
	maps.Copy(genericImpls, jsonv2Generics())
}

// genericName is "pkg.Func" or "pkg.Type.Method" for the generic function
// or method o (an origin, see ssa.Function.Origin).
func genericName(o *ssa.Function) string {
	if o.Pkg == nil {
		return ""
	}
	name := o.Name()
	if recv := o.Signature.Recv(); recv != nil {
		t := recv.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := types.Unalias(t).(*types.Named); ok {
			name = n.Obj().Name() + "." + name
		}
	}
	return o.Pkg.Pkg.Path() + "." + name
}

// isVMGeneric reports whether calls of f run genericIntrinsic: f is an
// instance of a generic function of a native package without body (no
// native instance of it exists), or genericImpls always implements it.
func isVMGeneric(f *ssa.Function) bool {
	o := f.Origin()
	if o == nil {
		return false
	}
	return len(f.Blocks) == 0 || genericImpls[genericName(o)].always
}

// genericIntrinsic implements instances of generic functions of native
// packages (they have no native instantiation).
func (m *Machine) genericIntrinsic(f *ssa.Function, args []int32) func([]Value) Value {
	if h := m.aotGeneric(f); h != nil {
		return m.aotGenericCall(h, f, args)
	}
	if g := genericImpls[genericName(f.Origin())]; g.make != nil {
		return g.make(m, f, args)
	}
	unsupported("generic function %s of a native package", f)
	return nil
}

// onceValue implements sync.OnceValue and sync.OnceValues. As in Go, when f
// panics, the first call and every later call panic with the same value.
// ErrBudget, ErrStackOverflow and runtime.Goexit leave the first call as
// they are; a later call then panics with the same fatal error, or, after a
// Goexit, with a *runtime.PanicNilError (what the Go code gives: it panics
// again with the nil value of recover).
func (m *Machine) onceValue(args []int32) func([]Value) Value {
	return func(regs []Value) Value {
		fn := regs[args[0]]
		var (
			once  sync.Once
			valid bool
			p     any
			res   Value
		)
		return Value{R: &Closure{Go: func([]Value) Value {
			once.Do(func() {
				defer func() {
					if valid {
						return
					}
					r := recover()
					switch r {
					case nil: // runtime.Goexit of the host: it goes on
						p = new(runtime.PanicNilError)
						return
					case errGoexit:
						p = new(runtime.PanicNilError)
					default:
						p = r
					}
					panic(r)
				}()
				res = m.callValue(fn, nil)
				valid = true
			})
			if !valid {
				panic(p)
			}
			return res
		}}}
	}
}

// fieldIndex returns the index of the field name of struct type t.
func fieldIndex(t types.Type, name string) int32 {
	st := t.Underlying().(*types.Struct)
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Name() == name {
			return int32(i)
		}
	}
	panic("vm: no field " + name + " in " + t.String())
}

// recvElem returns the type T of the receiver *T or T of method f.
func recvElem(f *ssa.Function) types.Type {
	t := f.Signature.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// atomicPointerOp implements a method of atomic.Pointer[T], which is
// struct{ _ [0]*T; _ noCopy; v unsafe.Pointer }. The methods are the
// unsafe.Pointer hooks of vm/atomic.go on the cell of field v: the cell holds
// the *T value of the VM as it is, so Load returns the same pointer that
// Store got (same identity), and the lock of the cell makes the operations
// atomic.
func atomicPointerOp(hook string) func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	return func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
		fi := fieldIndex(recvElem(f), "v")
		h := vmHooks["sync/atomic."+hook]
		return func(regs []Value) Value {
			a := make([]Value, len(args))
			a[0] = m.fieldAddr(regs[args[0]], fi)
			if _, ok := a[0].R.(*Value); !ok {
				unsupported("%s on an atomic.Pointer in native memory", f)
			}
			for i := 1; i < len(args); i++ {
				a[i] = regs[args[i]]
			}
			return h(m, a)
		}
	}
}

// randN implements math/rand/v2.N: its source reads the unexported
// globalRand, which is rand.Uint64N.
func randN(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	b, _ := f.TypeArgs()[0].Underlying().(*types.Basic)
	unsigned := b != nil && b.Info()&types.IsUnsigned != 0
	return func(regs []Value) Value {
		n := regs[args[0]].N
		if n == 0 || n < 0 && !unsigned {
			panic("invalid argument to N")
		}
		// 0 <= result < n: it fits in the type of n as it is.
		return Value{N: int64(rand.Uint64N(uint64(n)))}
	}
}

// Weak pointers. weak.Pointer[T] is struct{ _ [0]*T; u unsafe.Pointer }, and
// u is the indirection object that the runtime keeps for one (object,
// offset). The VM does the same: weak.Make makes a native weak pointer to
// the memory of the VM pointer (a *Value cell, a *byte, or native memory),
// and u is that same indirection. So two weak pointers made from the same
// pointer compare equal, as in Go, and the native copy of a VM weak pointer
// is a valid native weak pointer. Value.N of u says how the target is
// represented in the VM (weakCell...), to build the *T back.
const (
	weakNone   = iota // u came from native code: the representation is unknown
	weakCell          // a *Value cell
	weakByte          // a *byte
	weakNative        // native memory, a reflect.Value of type *T
)

func weakMake(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	rt := f.Signature.Results().At(0).Type()
	ui := fieldIndex(rt, "u")
	return func(regs []Value) Value {
		w := m.zero(rt)
		var target unsafe.Pointer
		var kind int64
		switch p := regs[args[0]].R.(type) {
		case nil:
			return w
		case *Value:
			if p == nil {
				return w
			}
			target, kind = unsafe.Pointer(p), weakCell
		case *byte:
			target, kind = unsafe.Pointer(p), weakByte
		case reflect.Value:
			if p.IsNil() {
				return w
			}
			target, kind = p.UnsafePointer(), weakNative
		default:
			unsupported("weak.Make of a %T pointer", p)
		}
		wp := weak.Make((*byte)(target))
		u := *(*unsafe.Pointer)(unsafe.Pointer(&wp)) // weak.Pointer is struct{ _ [0]*T; u unsafe.Pointer }
		w.R.([]Value)[ui] = Value{N: kind, R: (*byte)(u)}
		return w
	}
}

func weakValue(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	t := recvElem(f)
	ui := fieldIndex(t, "u")
	elem := f.TypeArgs()[0]
	return func(regs []Value) Value {
		fs, ok := regs[args[0]].R.([]Value)
		if !ok {
			unsupported("%s on a native value", f)
		}
		uv := fs[ui]
		u := ptrOf(uv)
		if u == nil {
			return Value{}
		}
		if uv.N == weakNone {
			unsupported("weak.Pointer[%s] made by native code", elem)
		}
		var wp weak.Pointer[byte]
		*(*unsafe.Pointer)(unsafe.Pointer(&wp)) = u
		p := wp.Value()
		if p == nil {
			return Value{}
		}
		switch uv.N {
		case weakCell:
			return Value{R: (*Value)(unsafe.Pointer(p))}
		case weakByte:
			return Value{R: p}
		}
		return Value{R: reflect.NewAt(m.reflectType(elem), unsafe.Pointer(p))}
	}
}

// addCleanup implements runtime.AddCleanup(ptr, cleanup, arg): the native
// cleanup is attached to the memory of the VM pointer, and it calls the
// interpreted cleanup with arg.
func addCleanup(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	at := f.TypeArgs()[1]
	var argPtr bool
	switch u := at.Underlying().(type) {
	case *types.Pointer:
		argPtr = true
	case *types.Basic:
		argPtr = u.Kind() == types.UnsafePointer
	}
	ct := f.Signature.Results().At(0).Type()
	return func(regs []Value) Value {
		ptr, fn, arg := regs[args[0]], regs[args[1]], regs[args[2]]
		target := ptrOf(ptr)
		if target == nil {
			panic("runtime.AddCleanup: ptr is nil")
		}
		if argPtr && ptrOf(arg) == target {
			panic("runtime.AddCleanup: ptr is equal to arg, cleanup will never run")
		}
		// The cleanup must not capture ptr, or it would never run. It runs
		// on the cleanup goroutine of the runtime, where nothing recovers:
		// it runs as a goroutine of the program (runGoroutine), so a panic,
		// ErrBudget or ErrStackOverflow goes to GoPanic and does not end the
		// host.
		c := runtime.AddCleanup((*byte)(target), func(arg Value) {
			args := []Value{arg}
			if cl, _ := fn.R.(*Closure); cl != nil && cl.Go == nil && cl.Fn != nil {
				m.runGoroutine(deferred{fn: cl.Fn, env: cl.Env, args: args}, nil)
				return
			}
			m.runGoroutine(deferred{nat: func() { m.callValue(fn, args) }}, nil)
		}, arg)
		return m.fromNative(reflect.ValueOf(c), ct)
	}
}

// maphashComparable implements maphash.Comparable(seed, v): the hash of the
// key of v that VM maps use (keyOf), which is equal for equal values.
func maphashComparable(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	ps := tupleTypes(f.Signature.Params())
	return func(regs []Value) Value {
		seed := m.toNative(regs[args[0]], ps[0]).Interface().(maphash.Seed)
		return Value{N: int64(maphash.Comparable[any](seed, keyOf(regs[args[1]], ps[1])))}
	}
}

// maphashWriteComparable implements maphash.WriteComparable(h, x), as
// maphashComparable.
func maphashWriteComparable(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	ps := tupleTypes(f.Signature.Params())
	return func(regs []Value) Value {
		var wb []func()
		h := m.toNativeW(regs[args[0]], ps[0], newConv(&wb)).Interface().(*maphash.Hash)
		maphash.WriteComparable[any](h, keyOf(regs[args[1]], ps[1]))
		writeBack(wb)
		return Value{}
	}
}

// bytesGeneric implements a generic function whose first type parameter is
// ~[]byte | ~string with its native instances str (for string) and byts
// (for []byte): the argument of the type parameter goes as its underlying
// type.
func bytesGeneric(str, byts any) func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	return func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
		bt := f.TypeArgs()[0].Underlying()
		nf := reflect.ValueOf(byts)
		if b, ok := bt.(*types.Basic); ok && b.Info()&types.IsString != 0 {
			nf = reflect.ValueOf(str)
		}
		orig := tupleTypes(f.Origin().Signature.Params())
		ps := tupleTypes(f.Signature.Params())
		rs := tupleTypes(f.Signature.Results())
		return func(regs []Value) Value {
			in := make([]reflect.Value, len(args))
			for i := range args {
				t := ps[i]
				if _, ok := orig[i].(*types.TypeParam); ok {
					t = bt
				}
				in[i] = m.toNative(regs[args[i]], t)
			}
			if nf.Type().IsVariadic() {
				return m.pack(nf.CallSlice(in), rs)
			}
			return m.pack(nf.Call(in), rs)
		}
	}
}

// hashGeneric implements the functions of crypto/hkdf and crypto/pbkdf2:
// their type parameter H is constrained by hash.Hash and only appears in
// their first parameter, func() H. They call the native instance for
// hash.Hash, nat, with a func() hash.Hash that calls the plugin function.
func hashGeneric(nat any) func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	nf := reflect.ValueOf(nat)
	return func(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
		ht := f.TypeArgs()[0]
		hashT := f.Origin().TypeParams().At(0).Constraint()
		ps := tupleTypes(f.Signature.Params())
		rs := tupleTypes(f.Signature.Results())
		return func(regs []Value) Value {
			fn := regs[args[0]]
			if fn.R == nil {
				panic(errNilDeref) // as calling a nil func() H
			}
			in := make([]reflect.Value, len(args))
			hf := nf.Type().In(0)
			if rv, ok := fn.R.(reflect.Value); ok { // a native function, like sha256.New
				if rv.Type() != hf {
					in[0] = reflect.MakeFunc(hf, func([]reflect.Value) []reflect.Value {
						h := reflect.New(hf.Out(0)).Elem()
						h.Set(rv.Call(nil)[0])
						return []reflect.Value{h}
					})
				} else {
					in[0] = rv
				}
			} else {
				in[0] = reflect.MakeFunc(hf, func([]reflect.Value) []reflect.Value {
					h := m.callValue(fn, nil)
					if !types.IsInterface(ht) {
						h = Value{R: &Iface{T: m.rtypeOf(ht), V: h}}
					}
					return []reflect.Value{m.toNative(h, hashT)}
				})
			}
			for i := 1; i < len(args); i++ {
				in[i] = m.toNative(regs[args[i]], ps[i])
			}
			return m.pack(nf.Call(in), rs)
		}
	}
}

// sqlNullTypes are the native instances of sql.Null, by the native type of
// T. The source of Null[T].Scan calls the unexported convertAssign, which
// the VM cannot call for another T: Null[T] runs with the native instance
// for these types and for the named types whose underlying type is one of
// them. sqlNullMethod handles the plugin types that implement sql.Scanner
// or driver.Valuer, and the struct types, itself.
var sqlNullTypes = map[reflect.Type]reflect.Type{}

func init() {
	for _, v := range []any{
		sql.Null[string]{}, sql.Null[[]byte]{}, sql.Null[bool]{}, sql.Null[time.Time]{}, sql.Null[any]{},
		sql.Null[int]{}, sql.Null[int8]{}, sql.Null[int16]{}, sql.Null[int32]{}, sql.Null[int64]{},
		sql.Null[uint]{}, sql.Null[uint8]{}, sql.Null[uint16]{}, sql.Null[uint32]{}, sql.Null[uint64]{},
		sql.Null[float32]{}, sql.Null[float64]{},
	} {
		t := reflect.TypeOf(v)
		sqlNullTypes[t.Field(0).Type] = t
	}
}

// sqlMethod returns the method name of the method set of t when it has the
// signature of sig (the method of sql.Scanner or driver.Valuer), or nil.
func (m *Machine) sqlMethod(t types.Type, name string, sig *types.Signature) *Func {
	sel := types.NewMethodSet(t).Lookup(nil, name)
	if sel == nil || !types.Identical(sel.Type().(*types.Signature), sig) {
		return nil
	}
	return m.rtypeOf(t).Methods[name]
}

var (
	tAny          = types.NewInterfaceType(nil, nil)
	tError        = types.Universe.Lookup("error").Type()
	sqlScannerSig = types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(0, nil, "", tAny)), types.NewTuple(types.NewParam(0, nil, "", tError)), false)
)

// sqlValuerSig is the signature of Value of driver.Valuer, with the type
// driver.Value of the package of f (database/sql), or nil.
func sqlValuerSig(f *ssa.Function) *types.Signature {
	o := f.Origin()
	if o == nil || o.Pkg == nil {
		return nil
	}
	for _, p := range o.Pkg.Pkg.Imports() {
		if p.Path() == "database/sql/driver" {
			if tn, ok := p.Scope().Lookup("Value").(*types.TypeName); ok {
				return types.NewSignatureType(nil, nil, nil, nil,
					types.NewTuple(types.NewParam(0, nil, "", tn.Type()), types.NewParam(0, nil, "", tError)), false)
			}
		}
	}
	return nil
}

// sqlNullMethod implements (*Null[T]).Scan and Null[T].Value, as the source
// of database/sql does: with the Scan or Value method of T when T is a
// plugin type that implements sql.Scanner (on *T) or driver.Valuer (on T),
// with the errors of convertAssign and ConvertValue for a struct T, and
// with the native instance of Null for the native type of T otherwise.
func sqlNullMethod(m *Machine, f *ssa.Function, args []int32) func([]Value) Value {
	t := f.TypeArgs()[0]
	st := recvElem(f)
	vi, oki := fieldIndex(st, "V"), fieldIndex(st, "Valid")
	rs := tupleTypes(f.Signature.Results())
	var scan, value *Func
	if n, ok := types.Unalias(t).(*types.Named); !ok || !isStdPath(n.Obj().Pkg().Path()) {
		scan = m.sqlMethod(types.NewPointer(t), "Scan", sqlScannerSig)
		if sig := sqlValuerSig(f); sig != nil {
			value = m.sqlMethod(t, "Value", sig)
		}
	}
	su, isStruct := t.Underlying().(*types.Struct)
	if isStruct && su.NumFields() > 0 && su.Field(0).Pkg() != nil && su.Field(0).Pkg().Path() == "time" {
		isStruct = false // type T time.Time: convertAssign converts a time.Time to it
	}
	field := func(regs []Value, i int32) *Value {
		p, ok := m.fieldAddr(regs[args[0]], i).R.(*Value)
		if !ok {
			unsupported("%s on a native value", f)
		}
		return p
	}
	if f.Name() == "Scan" {
		if scan == nil && !isStruct {
			return m.sqlNullNative(f, args, t, vi, oki, rs)
		}
		return func(regs []Value) Value {
			src := regs[args[1]]
			if src.R == nil {
				*field(regs, vi), *field(regs, oki) = m.zero(t), Value{}
				return Value{}
			}
			// Valid is false when Scan fails since Go 1.27 (n.Valid = err ==
			// nil); Go 1.26 sets it before the Scan (nullValidOnError).
			var err Value
			if scan != nil {
				err = m.Call(scan, []Value{m.fieldAddr(regs[args[0]], vi), src}, nil)
			} else if iv, ok := src.R.(*Iface); ok && iv.T.T != nil &&
				(types.Identical(iv.T.T, t) || types.IdenticalIgnoreTags(iv.T.T.Underlying(), t.Underlying())) {
				// convertAssign assigns a value of T, or converts a struct
				// value with the same fields (reflect: AssignableTo, then
				// ConvertibleTo for the same kind).
				*field(regs, vi) = deepCopy(iv.V, t)
			} else {
				err = m.ErrorVal(fmt.Errorf("unsupported Scan, storing driver.Value type %s into type %s",
					m.dynTypeString(src), typeString(types.NewPointer(t))))
			}
			*field(regs, oki) = Value{N: b2i(err.R == nil || nullValidOnError)}
			return err
		}
	}
	if value == nil && !isStruct {
		return m.sqlNullNative(f, args, t, vi, oki, rs)
	}
	return func(regs []Value) Value {
		recv := regs[args[0]]
		fs, ok := recv.R.([]Value)
		if !ok {
			unsupported("%s on a native value", f)
		}
		if fs[oki].N == 0 {
			return Value{R: []Value{{}, {}}}
		}
		if value == nil {
			return Value{R: []Value{{}, m.ErrorVal(fmt.Errorf("unsupported type %s, a struct", typeString(t)))}}
		}
		v := fs[vi]
		if p, ok := t.Underlying().(*types.Pointer); ok && v.R == nil && types.NewMethodSet(p.Elem()).Lookup(nil, "Value") != nil {
			return Value{R: []Value{{}, {}}} // callValuerValue: a nil *T whose T has Value
		}
		res := m.Call(value, []Value{v}, nil).R.([]Value)
		if res[1].R != nil {
			return Value{R: []Value{res[0], res[1]}}
		}
		out, err := driver.DefaultParameterConverter.ConvertValue(m.toNative(res[0], tAny).Interface())
		return m.pack([]reflect.Value{reflect.ValueOf(&out).Elem(), reflect.ValueOf(&err).Elem()}, rs)
	}
}

// dynTypeString is %T of the interface value v.
func (m *Machine) dynTypeString(v Value) string {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return "<nil>"
	case iv.T.T != nil:
		return typeString(iv.T.T)
	}
	return iv.T.Native.String()
}

// sqlNullConvert is the end of Null[T].Value in database/sql for a value v
// of native code: the Value of a driver.Valuer in v first
// (callValuerValue), then ConvertValue, which turns an int returned by
// Value into an int64. The Valuer is found by its method: a sql.NullString
// of the AOT copy of database/sql implements the driver.Valuer of the
// copy, not the native one.
func sqlNullConvert(v any) (driver.Value, error) {
	if val, ok, err := sqlCallValuer(v); ok {
		if err != nil {
			return val, err
		}
		v = val
	}
	return sqlConvertValue(v)
}

// sqlConvertValue is driver.DefaultParameterConverter.ConvertValue, with
// the Valuer of the AOT copy of database/sql/driver too.
func sqlConvertValue(v any) (driver.Value, error) {
	if !driver.IsValue(v) {
		if val, ok, err := sqlCallValuer(v); ok {
			if err != nil {
				return nil, err
			}
			if !driver.IsValue(val) {
				return nil, fmt.Errorf("non-Value type %T returned from Value", val)
			}
			return val, nil
		}
	}
	return driver.DefaultParameterConverter.ConvertValue(v)
}

// sqlCallValuer calls the Value method of v when v is a driver.Valuer,
// as callValuerValue: a nil *T whose T has Value gives nil.
func sqlCallValuer(v any) (driver.Value, bool, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, false, nil
	}
	mv := rv.MethodByName("Value")
	if !mv.IsValid() || !sqlValuerFunc(mv.Type()) {
		return nil, false, nil
	}
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		if em, ok := rv.Type().Elem().MethodByName("Value"); ok && sqlValuerFunc(em.Type) {
			return nil, true, nil
		}
	}
	out := mv.Call(nil)
	err, _ := out[1].Interface().(error)
	return out[0].Interface(), true, err
}

// sqlValuerFunc reports whether ft is the type of the Value method of a
// driver.Valuer, of the native database/sql/driver or of its AOT copy
// (with a receiver for a method of a type).
func sqlValuerFunc(ft reflect.Type) bool {
	in := 0
	if ft.NumIn() == 1 {
		in = 1 // the receiver of Method of a type
	}
	return ft.NumIn() == in && ft.NumOut() == 2 && ft.Out(0).Kind() == reflect.Interface &&
		ft.Out(0).NumMethod() == 0 && ft.Out(1) == errorType
}

// sqlValuerValue calls the Value method of the dynamic type of the
// interface value v when it is a type of the plugin that implements
// driver.Valuer, as callValuerValue: a nil *T whose T has Value gives nil.
// The types of the standard library are left to sqlNullConvert: their
// methods are native.
func (m *Machine) sqlValuerValue(v Value, sig *types.Signature) ([]Value, bool) {
	iv, _ := v.R.(*Iface)
	if iv == nil || iv.T.T == nil || sig == nil {
		return nil, false
	}
	nt := iv.T.T
	if p, ok := nt.Underlying().(*types.Pointer); ok {
		nt = p.Elem()
	}
	if n, ok := types.Unalias(nt).(*types.Named); ok && (n.Obj().Pkg() == nil || isStdPath(n.Obj().Pkg().Path())) {
		return nil, false
	}
	value := m.sqlMethod(iv.T.T, "Value", sig)
	if value == nil {
		return nil, false
	}
	if p, ok := iv.T.T.Underlying().(*types.Pointer); ok && iv.V.R == nil && types.NewMethodSet(p.Elem()).Lookup(nil, "Value") != nil {
		return []Value{{}, {}}, true
	}
	return m.Call(value, []Value{iv.V}, nil).R.([]Value), true
}

// sqlNative is the interface value v for native code, not opaque
// (OpaqueAny): ConvertValue reads it.
func (m *Machine) sqlNative(v Value) any {
	iv, _ := v.R.(*Iface)
	switch {
	case iv == nil:
		return nil
	case iv.T.T == nil: // a native value (*errors.errorString)
		return m.toNative(v, tAny).Interface()
	}
	return m.boxAs(iv, newConv(nil), false).Interface()
}

// sqlNullNative implements the Scan or Value method f of Null[T] with the
// native instance of Null for the native type of T.
func (m *Machine) sqlNullNative(f *ssa.Function, args []int32, t types.Type, vi, oki int32, rs []types.Type) func([]Value) Value {
	ut := t
	if _, ok := t.Underlying().(*types.Basic); ok {
		ut = t.Underlying()
	}
	nt := sqlNullTypes[m.reflectType(ut)]
	if (nt == nil || types.IsInterface(t)) && f.Name() == "Value" {
		// No native instance for this T (sql.Null[[]int],
		// Null[json.RawMessage]) or an interface T: Value as in database/sql,
		// nil for an invalid value, else the Value of a driver.Valuer in V,
		// then ConvertValue. database/sql calls it through driver.Valuer,
		// and the call panicked.
		sig := sqlValuerSig(f)
		return func(regs []Value) Value {
			fs, ok := regs[args[0]].R.([]Value)
			if !ok {
				unsupported("%s on a native value", f)
			}
			if fs[oki].N == 0 {
				return Value{R: []Value{{}, {}}}
			}
			v := fs[vi]
			if !types.IsInterface(t) {
				v = Value{R: &Iface{T: m.rtypeOf(t), V: v}}
			}
			res, ok := m.sqlValuerValue(v, sig)
			if !ok {
				out, err := sqlNullConvert(m.sqlNative(v))
				return m.pack([]reflect.Value{reflect.ValueOf(&out).Elem(), reflect.ValueOf(&err).Elem()}, rs)
			}
			if res[1].R != nil {
				return Value{R: []Value{res[0], res[1]}}
			}
			// ConvertValue of the result: a Valuer of the VM in it is called
			// once more, and must give a driver.Value.
			v = res[0]
			if res, ok = m.sqlValuerValue(v, sig); ok {
				if res[1].R != nil {
					return Value{R: []Value{{}, res[1]}}
				}
				if !driver.IsValue(m.sqlNative(res[0])) {
					return Value{R: []Value{{}, m.ErrorVal(fmt.Errorf("non-Value type %s returned from Value", m.dynTypeString(res[0])))}}
				}
				return Value{R: []Value{res[0], {}}}
			}
			out, err := sqlConvertValue(m.sqlNative(v))
			return m.pack([]reflect.Value{reflect.ValueOf(&out).Elem(), reflect.ValueOf(&err).Elem()}, rs)
		}
	}
	if nt == nil {
		unsupported("%s for this T", f)
	}
	if f.Name() == "Scan" {
		return func(regs []Value) Value {
			n := reflect.New(nt)
			// Not opaque (OpaqueAny): Scan reads the driver value.
			src := reflect.Zero(reflect.TypeFor[any]())
			if iv, _ := regs[args[1]].R.(*Iface); iv != nil {
				src = m.boxAs(iv, newConv(nil), false)
			}
			out := n.MethodByName("Scan").Call([]reflect.Value{src})
			recv := regs[args[0]]
			*m.fieldAddr(recv, vi).R.(*Value) = m.fromNative(n.Elem().Field(0), ut)
			*m.fieldAddr(recv, oki).R.(*Value) = Value{N: b2i(n.Elem().Field(1).Bool())}
			return m.pack(out, rs)
		}
	}
	return func(regs []Value) Value {
		fs, ok := regs[args[0]].R.([]Value)
		if !ok {
			unsupported("%s on a native value", f)
		}
		n := reflect.New(nt).Elem()
		n.Field(0).Set(m.toNative(fs[vi], ut).Convert(nt.Field(0).Type))
		n.Field(1).SetBool(fs[oki].N != 0)
		return m.pack(n.MethodByName("Value").Call(nil), rs)
	}
}
