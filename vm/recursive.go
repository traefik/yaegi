package vm

// Recursive interpreted types and native decoders.
//
// A recursive named type that is not a struct (type M map[string]M,
// type T []T, type A [1]*A) is cut in the parts of its underlying type that
// refer back to it (cutNamed): map[string]any, []any, [1]any. Its uniform
// types unroll these parts (unrolledNamed). For M and T, the generic value of
// a decoder (map[string]any, []any) is also their native type, so valid data
// needs no unrolling; only data of the wrong kind does, and the decoder then
// reports its own error. These types are not unrolled along the data (one
// recursive part: a data-shaped type would not be smaller).
//
// reflect cannot build a recursive type, so the native type of an
// interpreted recursive struct cuts the references back to the struct
// (reflectType, cutType): `type Rule struct { Name string; Rules []Rule }`
// becomes struct { Name string; Rules []any }. This is enough when native code
// only reads the value (json.Marshal, fmt): each any holds the native value
// of its static type. It is wrong when native code creates the values: a
// decoder (encoding/json, mapstructure) fills an any with its own generic
// value (map[string]any), which is not a Rule, and the conversion back
// panicked.
//
// The fix gives such decoders a native type unrolled deep enough for the
// data: a recursive field is built with the struct itself instead of any
// (struct { Name string; Rules []struct { Name string; Rules []any } }). The
// decoder runs a first time on the cut type; the generic values it put in
// the cuts tell how deep the data goes (their keys are matched with the field
// names and tags). The decoder then runs again on a native copy of the
// original value with a deeper type. When the data stops before every cut,
// the decoder has filled typed values only, with its own exact semantics
// (numbers, tags, weak typing, errors), and the result converts back as
// usual. Two runs are usually enough; a value that matches no field (an
// embedded struct, another naming rule) deepens the type by one more level
// per run.
//
// reflect keeps every type it builds (StructOf, PointerTo, SliceOf, ArrayOf,
// MapOf) for the life of the process. The data often comes from an HTTP
// client, so the types built must not depend freely on the data, or each
// request with a new shape would keep host memory forever. There are two
// kinds of unrolled types:
//
//   - Uniform types (uniformType): every recursive field is unrolled to the
//     same depth d. They depend only on the static type and d, so a static
//     type has at most one family of them, bounded by maxUniformBytes. This
//     is the usual case. Its limit: the type string of a struct holds the
//     strings of its field types, so a chain (one recursive field) of n
//     levels costs about n² bytes per byte of struct string (about 800
//     levels for struct { V int; Next *T }, 400 for a struct of 10 fields),
//     and a struct with k recursive fields costs about k^n (about 15 levels
//     for k = 2). The bound is per static type and per Machine, with no cap
//     for the whole Machine: a plugin with many recursive types can keep
//     maxUniformBytes for each of them (30 types fed depths 1 to 1000 keep
//     about 640 MB of heap). This is bounded by the code of the plugin, not
//     by the data, and yaegi v2 does not bound the memory of a plugin anyway
//     (docs/SANDBOX.md). The uniform types of one depth are the same for
//     every data, so data cannot make another decode of the same static
//     type fail.
//   - Data-shaped types (shapedType), when the uniform type for the depth of
//     the data is too large: only the fields the data uses are unrolled
//     (deep data on one field of a struct with several recursive fields, for
//     example a long chain of fallback rules). They depend on the data, so
//     the bytes of the new ones are bounded, with budgets (see below). One
//     call builds at most maxShapedCall bytes (a chain of about 400 levels on
//     one field of a small struct).
//
// Budgets of data-shaped types. reflect gives back the same type for the same
// fields, so a data-shaped type is counted once for the whole process, by the
// Machine that builds it first; a type that was already built (by any
// Machine) is found by its key (typeBuild.make) and always works, even when
// every budget is spent. The bytes of the new types are counted per use: data
// decoded by the plugin at run time (json.Unmarshal, usually request data
// that a client controls) and Object.Decode (the configuration, at load time).
// Each use has a budget per Machine (maxShapedMachine, 32 MB) and a cap for
// the whole process (maxShapedProcess, 256 MB; about 1.5 times that in
// heap). So the clients of one plugin can only spend the run-time budget of
// that plugin: the other plugins, and Object.Decode of every plugin (reloads
// of the configuration included), keep their own budgets. Only the process cap is shared:
// clients that spend the run-time budgets of 8 plugins reach it, and then
// new data shapes fail at run time in every plugin (known shapes and
// Object.Decode still work). Once a budget is spent, data that needs a new
// data-shaped type gets an error (never a crash); data that fits a uniform
// type or a known type still works.
//
// So default mode accepts less depth than gc (encoding/json stops at 10000
// levels): deeper data gets an error, never a crash. json.Unmarshal in AOT
// mode has no such limit (Object.Decode has it in both modes).
//
// Used by json.Unmarshal (a vmHook, default mode) and Object.Decode.
// In AOT mode, encoding/json works on VM values through the twin reflect and
// needs none of this. Other native decoders (json.Decoder.Decode, yaml in
// default mode) still get the cut type; the conversion back then fails with a
// clear message instead of a reflect panic.

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// maxShapeRuns is the maximum number of runs of the decoder.
	maxShapeRuns = 64
	// maxShapeSteps bounds the walk of the native and generic values after
	// one run.
	maxShapeSteps = 1 << 20
)

// Limits on the unrolled types, in bytes of type strings (the string is
// the main cost of a type built by reflect). They are variables for tests.
var (
	// maxUniformBytes bounds the uniform types of one static type in one
	// Machine.
	maxUniformBytes = 16 << 20
	// maxShapedCall bounds the data-shaped types built by one call (new or
	// not: it also bounds the work).
	maxShapedCall = 8 << 20
	// maxShapedMachine bounds the new data-shaped types built by one
	// Machine for one use (shapedUse).
	maxShapedMachine = 32 << 20
	// maxShapedProcess bounds the new data-shaped types built by the whole
	// process for one use.
	maxShapedProcess = 256 << 20
)

// shapedUse tells who asks for data-shaped types: each use has its own
// budgets, so that data sent by clients cannot spend the budget of the
// configuration.
type shapedUse int

const (
	shapedData   shapedUse = iota // json.Unmarshal called by the plugin
	shapedConfig                  // Object.Decode
	numShapedUses
)

func (u shapedUse) String() string {
	if u == shapedConfig {
		return "DecodeConfig"
	}
	return "data decoded by the plugin"
}

// unrolledBytes counts the bytes of the type strings of the unrolled types
// built for the first time (uniform and data-shaped), for tests.
var unrolledBytes atomic.Int64

// shaped holds the data-shaped types of the process (one lock: they are
// rare). Machine.shapedBytes is also guarded by mu.
var shaped struct {
	mu    sync.Mutex
	ids   map[reflect.Type]int    // a number for each type used in a key
	built map[string]reflect.Type // key (typeBuild.make) -> data-shaped type
	bytes [numShapedUses]int      // bytes of the new types, per use
}

// uniformTypes is the family of uniform types of one static type.
type uniformTypes struct {
	mu      sync.Mutex
	types   map[uniformKey]reflect.Type
	bytes   int // bytes of the type strings built for this family
	tooDeep int // smallest depth found too large, 0 if none
}

type uniformKey struct {
	t     types.Type
	depth int
}

var (
	errTooLarge = errors.New("unrolled type too large")
	errBudget   = errors.New("budget of data-shaped types spent")
)

// typeBuild builds the unrolled types of one call and counts their size.
type typeBuild struct {
	work, max int // bytes of the type strings built by this call, and the limit
	// built and ids, when not nil, find the types already built by their
	// key (data-shaped types: shaped.built and shaped.ids); without them,
	// every type counts as new. added counts the bytes of the new types and
	// must stay below budget.
	built         map[string]reflect.Type
	ids           map[reflect.Type]int
	added, budget int
}

// make builds a type whose string has about n bytes with f, unless a limit
// would be passed: it panics with errTooLarge or errBudget before building.
// key gives a key of the type from the types it is made of (they are built
// before it), the same for two calls of f that give the same type. A type
// already built is given back without checking the budget: the budget only
// bounds new types, so known types keep working when it is spent.
func (b *typeBuild) make(n int, key func() string, f func() reflect.Type) reflect.Type {
	if b.work+n > b.max {
		panic(errTooLarge)
	}
	b.work += n
	k := ""
	if b.built != nil {
		k = key()
		if r, ok := b.built[k]; ok {
			return r
		}
	}
	if b.added+n > b.budget {
		panic(errBudget)
	}
	r := f()
	if b.built != nil {
		b.built[k] = r
	}
	b.added += n
	unrolledBytes.Add(int64(n))
	return r
}

// id is the part of a key for the type r: a number given to r the first time
// it is seen. Types built by reflect are unique (the same fields give the same
// type), so equal keys mean equal types.
func (b *typeBuild) id(r reflect.Type) string {
	n, ok := b.ids[r]
	if !ok {
		n = len(b.ids) + 1
		b.ids[r] = n
	}
	return strconv.Itoa(n)
}

// buildErr turns the panic of a typeBuild into an error for the type t.
func buildErr(r any, t types.Type, use shapedUse) error {
	switch r {
	case errTooLarge:
		return fmt.Errorf("vm: the data for the recursive type %s is nested too deeply for yaegi v2 default mode: "+
			"its native type, unrolled to the depth of the data, would be too large "+
			"(about 800 levels for a small struct with one recursive field, fewer for a large struct "+
			"or data on several recursive fields; reflect cannot build recursive types)", t)
	case errBudget:
		return fmt.Errorf("vm: the data for the recursive type %s needs new native types, "+
			"but yaegi v2 default mode reached its limit for such types (for %s: %d MB per plugin, %d MB for the process; "+
			"reflect never frees types)", t, use, maxShapedMachine>>20, maxShapedProcess>>20)
	}
	panic(r)
}

// shape tells how a struct type is unrolled: kids[i] is the shape of the
// struct reached through field i (through pointers, slices, arrays and map
// values). A field without kid has its usual native type: the cut type for a
// field that refers back to the struct, reflectType otherwise.
type shape struct {
	kids map[int]*shape
}

func (s *shape) kid(i int) *shape {
	if k := s.kids[i]; k != nil {
		return k
	}
	if s.kids == nil {
		s.kids = map[int]*shape{}
	}
	k := &shape{}
	s.kids[i] = k
	return k
}

type ushadowKey struct {
	cell *Value
	rt   reflect.Type
}

func init() {
	vmHooks["encoding/json.Unmarshal"] = func(m *Machine, a []Value) Value { return m.jsonUnmarshal(a) }
}

var (
	byteSliceT = types.NewSlice(types.Typ[types.Byte])
	anyT       = types.Universe.Lookup("any").Type()
	errorT     = types.Universe.Lookup("error").Type()
)

// jsonUnmarshal is json.Unmarshal called from interpreted code in default
// mode. A pointer to a recursive interpreted type gets a native type
// unrolled along the data (decodeShaped); other values are converted as for
// any native call.
func (m *Machine) jsonUnmarshal(a []Value) Value {
	if m.routed["encoding/json"] && m.routeNeeded(a[1], false, "encoding/json") {
		// the AOT copy decodes into the interpreted values, with their
		// methods, recursive types included (aotroute.go)
		if fn, ok := aotPkg("encoding/json").Funcs["Unmarshal"]; ok {
			return m.callAOTRoute("encoding/json", jsonUnmarshalSite, fn, 1, a)
		}
	}
	if err := m.lostCheck("encoding/json.Unmarshal", a[1]); err != nil {
		return m.errValue(err) // lostmethods.go
	}
	data := m.toNative(a[0], byteSliceT).Bytes()
	if iv, _ := a[1].R.(*Iface); iv != nil && iv.T.Native == nil && iv.T.T != nil {
		if pt, ok := iv.T.T.Underlying().(*types.Pointer); ok && m.hasCut(pt.Elem()) {
			if _, ok := iv.V.R.(*Value); ok {
				err := m.decodeShaped(iv.V, iv.T.T, shapedData, true, func(rv reflect.Value) error {
					return json.Unmarshal(data, rv.Interface())
				})
				return m.errValue(err)
			}
		}
	}
	var wb []func()
	c := newConv(&wb)
	c.dst = decodeJSON
	v := m.toNativeW(a[1], anyT, c)
	c.dst = decodeNone
	defer writeBack(wb)
	defer m.flushShadows()
	return m.errValue(json.Unmarshal(data, v.Interface()))
}

// jsonUnmarshalSite is the site of json.Unmarshal(data, v) for its hook: the
// arguments of the hook, in order.
var jsonUnmarshalSite = &nativeSite{args: []int32{0, 1}, params: []types.Type{byteSliceT, anyT}, results: []types.Type{errorT}, stores: -1}

func (m *Machine) errValue(err error) Value {
	return m.fromNative(reflect.ValueOf(&err).Elem(), errorT)
}

// decodeShaped runs decode on a native copy of v, a pointer of type t whose
// native type has cuts, then copies the result back into v. While decode
// fills a cut with its own value, it runs again on a new copy, with a deeper
// uniform type, or a data-shaped type when the uniform type is too large.
// writeOnErr also copies the result back when decode fails (json keeps what
// it decoded before the error). use picks the budget of new data-shaped
// types.
func (m *Machine) decodeShaped(v Value, t types.Type, use shapedUse, writeOnErr bool, decode func(rv reflect.Value) error) error {
	depth := 0
	var first reflect.Value // result of the run with the cut type
	for run := 0; run < maxShapeRuns; run++ {
		rt, ok := m.uniformTop(t, depth)
		if !ok {
			return m.decodeByShape(v, t, use, writeOnErr, decode, run, first)
		}
		var wb []func()
		c := newConv(&wb)
		rv := m.toNativeU(v, t, rt, c.nested())
		err := decode(rv)
		steps := maxShapeSteps
		need := m.cutDepth(rv, t, map[uintptr]bool{}, &steps)
		if need < 0 {
			if err == nil || writeOnErr {
				m.fromNativeDeepC(rv, t, c)
				writeBack(wb)
			}
			return err
		}
		// decode filled cuts: run again with a deeper type
		if depth == 0 {
			first = rv
		} else {
			first = reflect.Value{}
		}
		depth = max(need, depth+1)
	}
	return buildErr(errTooLarge, t, use)
}

// decodeByShape is decodeShaped with data-shaped types. first, when valid,
// is the result of a run with the cut type (reflectType(t)); runs is the
// number of runs already done.
func (m *Machine) decodeByShape(v Value, t types.Type, use shapedUse, writeOnErr bool, decode func(rv reflect.Value) error, runs int, first reflect.Value) error {
	sh := &shape{}
	if first.IsValid() {
		steps := maxShapeSteps
		m.grow(first, t, sh, map[uintptr]bool{}, &steps)
	}
	for run := runs; run < maxShapeRuns; run++ {
		rt, err := m.shapedTypeTop(t, sh, use)
		if err != nil {
			return err
		}
		var wb []func()
		c := newConv(&wb)
		rv := m.toNativeU(v, t, rt, c.nested())
		err = decode(rv)
		steps := maxShapeSteps
		if m.grow(rv, t, sh, map[uintptr]bool{}, &steps) {
			continue // decode filled cuts: run again with a deeper type
		}
		if steps := maxShapeSteps; m.cutDepth(rv, t, map[uintptr]bool{}, &steps) >= 0 {
			// values of the decoder are still in cuts that the shape
			// cannot unroll (a selfRecursive type, data deeper than its
			// uniform types): the conversion back would fail
			return buildErr(errTooLarge, t, use)
		}
		if err == nil || writeOnErr {
			m.fromNativeDeepC(rv, t, c)
			writeBack(wb)
		}
		return err
	}
	return buildErr(errTooLarge, t, use)
}

// decodeConfigShaped is Object.Decode for a type t (a pointer)
// whose native type has cuts. It has its own budget of data-shaped types, so
// that data decoded at run time cannot make the configuration fail.
func (m *Machine) decodeConfigShaped(cfg Value, t types.Type, decode func(ptr any) error) error {
	return m.decodeShaped(cfg, t, shapedConfig, false, func(rv reflect.Value) error { return decode(rv.Interface()) })
}

// hasCut reports whether the native type of t (reflectType) cuts a
// recursive field somewhere.
func (m *Machine) hasCut(t types.Type) bool {
	if r, ok := m.hasCuts.Load(t); ok {
		return r.(bool)
	}
	r := m.hasCutIn(t, map[types.Type]bool{})
	m.hasCuts.Store(t, r)
	return r
}

func (m *Machine) hasCutIn(t types.Type, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := t.(type) {
	case *types.Named:
		if m.nativeOrTwin(u) {
			return false
		}
		if m.selfRecursive(u) {
			return true
		}
		return m.hasCutIn(u.Underlying(), seen)
	case *types.Struct:
		cut := m.recursiveFields(u)
		for i := range cut {
			if cut[i] || m.hasCutIn(u.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Pointer:
		return m.hasCutIn(u.Elem(), seen)
	case *types.Slice:
		return m.hasCutIn(u.Elem(), seen)
	case *types.Array:
		return m.hasCutIn(u.Elem(), seen)
	case *types.Map:
		return m.hasCutIn(u.Key(), seen) || m.hasCutIn(u.Elem(), seen)
	}
	return false
}

// nativeOrTwin reports whether reflectType maps the named type n to an
// existing native (or twin) type instead of building it.
func (m *Machine) nativeOrTwin(n *types.Named) bool {
	obj := n.Obj()
	if obj.Pkg() == nil || m.twinNamed(n) != nil {
		return true
	}
	return !m.interpPkg(obj.Pkg()) && n.TypeArgs().Len() == 0
}

// uniformTop returns the native type of t with every recursive field
// unrolled to depth levels (reflectType(t) for depth 0), or false when it is
// too large. The types of a static type are built once per Machine.
func (m *Machine) uniformTop(t types.Type, depth int) (rt reflect.Type, ok bool) {
	if depth == 0 {
		return m.reflectType(t), true
	}
	v, _ := m.uniform.LoadOrStore(t, &uniformTypes{})
	f := v.(*uniformTypes)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.tooDeep > 0 && depth >= f.tooDeep {
		return nil, false
	}
	if f.types == nil {
		f.types = map[uniformKey]reflect.Type{}
	}
	// Check the cost first, so that a type too large builds nothing.
	free := maxUniformBytes - f.bytes
	if cost := 0; m.uniformCost(t, depth, f, map[uniformKey]int{}, &cost, free) < 0 || cost > free {
		if f.tooDeep == 0 || depth < f.tooDeep {
			f.tooDeep = depth
		}
		return nil, false
	}
	b := &typeBuild{max: free, budget: free}
	defer func() {
		f.bytes += b.added
		if r := recover(); r != nil {
			if r != errTooLarge && r != errBudget {
				panic(r)
			}
			if f.tooDeep == 0 || depth < f.tooDeep {
				f.tooDeep = depth
			}
			rt, ok = nil, false
		}
	}()
	return m.uniformType(t, depth, f, b), true
}

// uniformType is the native type of t with every recursive field unrolled
// to depth levels: a field of a struct that refers back to the struct has
// the uniform type of depth-1, the cut type at depth 0. It depends only on t
// and depth, never on the data.
func (m *Machine) uniformType(t types.Type, depth int, f *uniformTypes, b *typeBuild) reflect.Type {
	if depth == 0 || !m.hasCut(t) {
		return m.reflectType(t)
	}
	k := uniformKey{t, depth}
	if r, ok := f.types[k]; ok {
		return r
	}
	var r reflect.Type
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		switch {
		case m.nativeOrTwin(u):
			r = m.reflectType(t)
		case m.selfRecursive(u):
			r = m.unrolledNamed(u, b, func(pt types.Type, cut bool) reflect.Type {
				if cut {
					return m.uniformType(pt, depth-1, f, b)
				}
				return m.uniformType(pt, depth, f, b)
			})
		default:
			r = m.uniformType(u.Underlying(), depth, f, b)
		}
	case *types.Struct:
		r = m.unrolledStruct(u, b, func(i int, ft types.Type, cut bool) reflect.Type {
			if cut {
				return m.uniformType(ft, depth-1, f, b)
			}
			return m.uniformType(ft, depth, f, b)
		})
	default:
		r = m.unrolledComposite(t, b, func(et types.Type) reflect.Type { return m.uniformType(et, depth, f, b) })
	}
	f.types[k] = r
	return r
}

// uniformCost follows uniformType without building types: it returns the
// length of the type string of uniformType(t, depth), at least what
// typeBuild.make will count, and adds to cost the lengths of the types not
// built yet. It returns -1 once cost is past limit.
func (m *Machine) uniformCost(t types.Type, depth int, f *uniformTypes, lens map[uniformKey]int, cost *int, limit int) int {
	if *cost > limit {
		return -1
	}
	if depth == 0 || !m.hasCut(t) {
		return len(m.reflectType(t).String())
	}
	k := uniformKey{t, depth}
	if r, ok := f.types[k]; ok {
		return len(r.String())
	}
	if n, ok := lens[k]; ok {
		return n
	}
	elem := func(et types.Type, d int) int { return m.uniformCost(et, d, f, lens, cost, limit) }
	n := -1
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		if m.nativeOrTwin(u) {
			return len(m.reflectType(t).String())
		}
		if m.selfRecursive(u) {
			// unrolledNamed builds the underlying type, with one level less
			// in the parts that refer back to u
			keyCut, elemCut := m.namedCuts(u)
			ed := depth
			if elemCut {
				ed--
			}
			switch uu := u.Underlying().(type) {
			case *types.Pointer, *types.Slice, *types.Array, *types.Map:
				e := elem(uu.(interface{ Elem() types.Type }).Elem(), ed)
				if e < 0 {
					return -1
				}
				switch uu := uu.(type) {
				case *types.Pointer:
					n = 1 + e
				case *types.Slice:
					n = 2 + e
				case *types.Array:
					n = 22 + e
				case *types.Map:
					kt := anyType
					if !keyCut {
						kt = m.reflectType(uu.Key())
					}
					// MapOf costs more than the string of the map type,
					// but the string is what the outer types contain
					if *cost > limit {
						return -1
					}
					*cost += mapCost(len(kt.String()), e)
					n = len("map[]") + len(kt.String()) + e
					lens[k] = n
					return n
				}
			default:
				return len(m.reflectType(t).String())
			}
			break
		}
		n = elem(u.Underlying(), depth)
		lens[k] = n
		return n // no type built for a named type
	case *types.Struct:
		cut := m.recursiveFields(u)
		n = len("struct {  }")
		for i := 0; i < u.NumFields(); i++ {
			fd := depth
			if cut[i] {
				fd--
			}
			e := elem(u.Field(i).Type(), fd)
			if e < 0 {
				return -1
			}
			n += fieldCost(u.Field(i).Name(), e, u.Tag(i))
		}
	case *types.Pointer, *types.Slice, *types.Array, *types.Map:
		e := elem(u.(interface{ Elem() types.Type }).Elem(), depth)
		if e < 0 {
			return -1
		}
		switch u := u.(type) {
		case *types.Pointer:
			n = 1 + e
		case *types.Slice:
			n = 2 + e
		case *types.Array:
			n = 22 + e
		case *types.Map:
			n = mapCost(len(m.reflectType(u.Key()).String()), e)
		}
	default:
		return len(m.reflectType(t).String())
	}
	if *cost > limit {
		return -1
	}
	*cost += n
	lens[k] = n
	return n
}

// shapedTypeTop returns the native type of t unrolled by sh, or an error
// when it is too large or needs new types once the budgets of use (for this
// Machine or for the process) are spent.
func (m *Machine) shapedTypeTop(t types.Type, sh *shape, use shapedUse) (rt reflect.Type, err error) {
	if len(sh.kids) == 0 {
		return m.reflectType(t), nil
	}
	shaped.mu.Lock()
	defer shaped.mu.Unlock()
	if shaped.built == nil {
		shaped.built, shaped.ids = map[string]reflect.Type{}, map[reflect.Type]int{}
	}
	b := &typeBuild{
		max:    maxShapedCall,
		budget: min(maxShapedMachine-m.shapedBytes[use], maxShapedProcess-shaped.bytes[use]),
		built:  shaped.built,
		ids:    shaped.ids,
	}
	defer func() {
		m.shapedBytes[use] += b.added
		shaped.bytes[use] += b.added
		if r := recover(); r != nil {
			rt, err = nil, buildErr(r, t, use)
		}
	}()
	return m.shapedType(t, sh, b), nil
}

// shapedType is the native type of t unrolled along sh: only the fields of
// sh are unrolled.
func (m *Machine) shapedType(t types.Type, sh *shape, b *typeBuild) reflect.Type {
	if len(sh.kids) == 0 {
		return m.reflectType(t)
	}
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		if m.nativeOrTwin(u) || m.selfRecursive(u) {
			// a selfRecursive type is not unrolled along the data (see
			// unrolledNamed)
			return m.reflectType(t)
		}
		return m.shapedType(u.Underlying(), sh, b)
	case *types.Struct:
		return m.unrolledStruct(u, b, func(i int, ft types.Type, cut bool) reflect.Type {
			switch k := sh.kids[i]; {
			case k != nil:
				return m.shapedType(ft, k, b)
			case cut:
				return m.cutType(ft, u)
			}
			return m.reflectType(ft)
		})
	}
	return m.unrolledComposite(t, b, func(et types.Type) reflect.Type { return m.shapedType(et, sh, b) })
}

// unrolledStruct builds the struct u as reflectType does, with the field
// types given by field (cut tells whether the field refers back to u).
func (m *Machine) unrolledStruct(u *types.Struct, b *typeBuild, field func(i int, ft types.Type, cut bool) reflect.Type) reflect.Type {
	cut := m.recursiveFields(u)
	fs := make([]reflect.StructField, u.NumFields())
	n := len("struct {  }")
	for i := range fs {
		f := u.Field(i)
		fs[i] = reflect.StructField{Name: f.Name(), Tag: reflect.StructTag(u.Tag(i)), Anonymous: f.Embedded()}
		fs[i].Type = field(i, f.Type(), cut[i])
		if cut[i] {
			fs[i].Anonymous = fs[i].Anonymous && fs[i].Type.Kind() != reflect.Interface
		}
		if !f.Exported() {
			fs[i].PkgPath = "main"
		}
		n += fieldCost(f.Name(), len(fs[i].Type.String()), u.Tag(i))
	}
	key := func() string {
		var k strings.Builder
		k.WriteString("s")
		for _, f := range fs {
			fmt.Fprintf(&k, "%s\x00%s\x00%q\x00%t\x00%s\x01", f.Name, f.PkgPath, f.Tag, f.Anonymous, b.id(f.Type))
		}
		return k.String()
	}
	return b.make(n, key, func() reflect.Type { return structOf(fs) })
}

// unrolledComposite builds a pointer, slice, array or map type t whose
// element type is elem(element type of t); map keys are not unrolled.
func (m *Machine) unrolledComposite(t types.Type, b *typeBuild, elem func(types.Type) reflect.Type) reflect.Type {
	switch u := types.Unalias(t).(type) {
	case *types.Pointer:
		e := elem(u.Elem())
		return b.make(1+len(e.String()), func() string { return "p" + b.id(e) }, func() reflect.Type { return reflect.PointerTo(e) })
	case *types.Slice:
		e := elem(u.Elem())
		return b.make(2+len(e.String()), func() string { return "l" + b.id(e) }, func() reflect.Type { return reflect.SliceOf(e) })
	case *types.Array:
		e := elem(u.Elem())
		return b.make(22+len(e.String()), func() string { return fmt.Sprintf("a%d:%s", u.Len(), b.id(e)) },
			func() reflect.Type { return reflect.ArrayOf(int(u.Len()), e) })
	case *types.Map:
		k, e := m.reflectType(u.Key()), elem(u.Elem())
		return b.make(mapCost(len(k.String()), len(e.String())), func() string { return "m" + b.id(k) + ":" + b.id(e) },
			func() reflect.Type { return reflect.MapOf(k, e) })
	}
	return m.reflectType(t)
}

// unrolledNamed builds the underlying type of n, a selfRecursive type, as
// cutNamed does, with the part types given by part (cut tells whether the
// part refers back to n). Pointers, slices, arrays and map values are
// unrolled; a map key that refers back to n stays an any, and funcs and
// channels keep their cut type (decoders do not fill them).
//
// Only uniform types are built for these types: they have one recursive
// part (two for a map whose key is cut, which decoders do not fill), so
// unrolling along the data would not make the type smaller. Data too deep
// for the uniform types keeps the cut type, and the conversion back reports
// a value of the decoder in a cut.
func (m *Machine) unrolledNamed(n *types.Named, b *typeBuild, part func(pt types.Type, cut bool) reflect.Type) reflect.Type {
	keyCut, elemCut := m.namedCuts(n)
	elem := func(et types.Type) reflect.Type { return part(et, elemCut) }
	switch u := n.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Array:
		return m.unrolledComposite(u, b, elem)
	case *types.Map:
		k, e := anyType, elem(u.Elem())
		if !keyCut {
			k = m.reflectType(u.Key())
		}
		return b.make(mapCost(len(k.String()), len(e.String())), func() string { return "m" + b.id(k) + ":" + b.id(e) },
			func() reflect.Type { return reflect.MapOf(k, e) })
	}
	return m.reflectType(n)
}

// namedCuts reports whether the key and the element of the underlying type
// of n, a selfRecursive type, refer back to n (cutNamed makes them an any).
// Only maps have a key.
func (m *Machine) namedCuts(n *types.Named) (keyCut, elemCut bool) {
	reach := func(t types.Type) bool { return m.reachesNamed(t, n, map[types.Type]bool{}) }
	switch u := n.Underlying().(type) {
	case *types.Map:
		return reach(u.Key()), reach(u.Elem())
	case *types.Pointer:
		return false, reach(u.Elem())
	case *types.Slice:
		return false, reach(u.Elem())
	case *types.Array:
		return false, reach(u.Elem())
	case *types.Chan:
		return false, reach(u.Elem())
	}
	return false, false
}

// recNamedOf returns t when it is an interpreted selfRecursive named type.
func (m *Machine) recNamedOf(t types.Type) *types.Named {
	if n, ok := types.Unalias(t).(*types.Named); ok && !m.nativeOrTwin(n) && m.selfRecursive(n) {
		return n
	}
	return nil
}

// fieldCost is the length of a field in the string of a struct type, when
// the string of its type has e bytes: `Name type "tag"; `.
func fieldCost(name string, e int, tag string) int {
	n := len(name) + 1 + e + 2
	if tag != "" {
		n += 1 + len(strconv.Quote(tag))
	}
	return n
}

// mapCost is the cost of MapOf for a key and an element type whose strings
// have k and e bytes: MapOf also builds the types of the map groups (a
// slot struct, an array of slots, a group struct, and a pointer to a large
// element), each with the strings of the key and element types.
func mapCost(k, e int) int {
	return 5 * (k + e + 16)
}

// toNativeU converts v, of type t, to rt, a native type of t unrolled by
// uniformType or shapedType. The shadows of interpreted pointers are
// registered in c (c.g.ushadows, one per cell and native type), so
// fromNativeDeepC writes the decoded values back into the same cells.
//
// When c keeps identity (c.keeps(), the conversion of decodeShaped), maps and
// slices are recorded in the conv graph like toNativeW does (mapCopyOf,
// sliceCopyOf), with their unrolled native types: after the run that is
// kept, the write-backs fill the same interpreted maps and arrays, so their
// aliases see what the decoder wrote (json.Unmarshal into a non nil map
// field, or into a slice field with room). The runs that are dropped never
// run their write-backs.
func (m *Machine) toNativeU(v Value, t types.Type, rt reflect.Type, c *natConv) reflect.Value {
	if rt.Kind() == reflect.Interface || rt == m.reflectType(t) {
		if types.IsInterface(t) || rt.Kind() != reflect.Interface {
			return m.toNativeW(v, t, c)
		}
		return m.toNativeAs(v, t, rt, c) // a cut: the any holds the native value
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		cell, _ := v.R.(*Value)
		if cell == nil {
			return reflect.Zero(rt)
		}
		k := ushadowKey{cell, rt}
		if sh, ok := c.g.ushadows[k]; ok {
			return sh
		}
		sh := reflect.New(rt.Elem())
		if c.g.ushadows == nil {
			c.g.ushadows = map[ushadowKey]reflect.Value{}
		}
		if c.g.cells == nil {
			c.g.shadows, c.g.cells = map[*Value]reflect.Value{}, map[uintptr]*Value{}
		}
		c.g.ushadows[k], c.g.cells[sh.Pointer()] = sh, cell
		sh.Elem().Set(m.toNativeU(*cell, u.Elem(), rt.Elem(), c))
		return sh
	case *types.Struct:
		out := reflect.New(rt).Elem()
		fs, _ := v.R.([]Value)
		for i := range fs {
			f := out.Field(i)
			if !f.CanSet() {
				f = reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
			}
			f.Set(m.toNativeU(fs[i], u.Field(i).Type(), f.Type(), c))
		}
		return out
	case *types.Slice:
		s, _ := v.R.([]Value)
		if s == nil {
			return reflect.Zero(rt)
		}
		if !c.keeps() {
			out := reflect.MakeSlice(rt, len(s), len(s))
			for i := range s {
				out.Index(i).Set(m.toNativeU(s[i], u.Elem(), rt.Elem(), c))
			}
			return out
		}
		return m.sliceCopyOf(s, rt, u.Elem(), c, func(e Value) reflect.Value { return m.toNativeU(e, u.Elem(), rt.Elem(), c) })
	case *types.Array:
		out := reflect.New(rt).Elem()
		es, _ := v.R.([]Value)
		for i := range es {
			out.Index(i).Set(m.toNativeU(es[i], u.Elem(), rt.Elem(), c))
		}
		return out
	case *types.Map:
		mp, _ := v.R.(*MapV)
		if mp == nil {
			return reflect.Zero(rt)
		}
		if !c.keeps() {
			out := reflect.MakeMapWithSize(rt, len(mp.M))
			mp.each(func(k, val Value) {
				out.SetMapIndex(m.toNativeU(k, u.Key(), rt.Key(), c), m.toNativeU(val, u.Elem(), rt.Elem(), c))
			})
			return out
		}
		// map keys are not unrolled: mapCopyOf converts them with toNativeW
		return m.mapCopyOf(mp, rt, u, c, func(e Value) reflect.Value { return m.toNativeU(e, u.Elem(), rt.Elem(), c) })
	}
	return m.toNativeAs(v, t, rt, c)
}

// grow walks rv, the native value of type t built with the shape sh, after
// a run of the decoder. For each field in cut form whose cuts hold a value
// of the decoder, it unrolls the field in sh, along that value. It reports
// whether sh changed.
func (m *Machine) grow(rv reflect.Value, t types.Type, sh *shape, seen map[uintptr]bool, steps *int) bool {
	if !m.hasCut(t) || *steps <= 0 || m.recNamedOf(t) != nil {
		return false // a selfRecursive type is not unrolled along the data
	}
	*steps--
	grew := false
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if rv.IsNil() || seen[rv.Pointer()] {
			return false
		}
		seen[rv.Pointer()] = true
		return m.grow(rv.Elem(), u.Elem(), sh, seen, steps)
	case *types.Slice, *types.Array:
		et := u.(interface{ Elem() types.Type }).Elem()
		for i := 0; i < rv.Len(); i++ {
			grew = m.grow(rv.Index(i), et, sh, seen, steps) || grew
		}
	case *types.Map:
		it := rv.MapRange()
		for it.Next() {
			grew = m.grow(it.Value(), u.Elem(), sh, seen, steps) || grew
		}
	case *types.Struct:
		cut := m.recursiveFields(u)
		for i := 0; i < u.NumFields(); i++ {
			f, ft := rv.Field(i), u.Field(i).Type()
			if k := sh.kids[i]; k != nil {
				grew = m.grow(f, ft, k, seen, steps) || grew
				continue
			}
			k, need := &shape{}, false
			if cut[i] {
				m.eachCut(f, ft, u, func(slot reflect.Value, st types.Type) {
					if slot.IsNil() {
						return
					}
					e := slot.Elem()
					if e.Type() != m.reflectType(st) {
						need = true // a value of the decoder
						m.growGeneric(k, st, e, steps)
						return
					}
					need = m.grow(e, st, k, seen, steps) || need
				})
			} else {
				need = m.grow(f, ft, k, seen, steps)
			}
			if need {
				if sh.kids == nil {
					sh.kids = map[int]*shape{}
				}
				sh.kids[i] = k
				grew = true
			}
		}
	}
	return grew
}

// cutDepth walks rv, the native value of type t built with any unrolled
// type, after a run of the decoder. It returns the depth of the deepest
// value the decoder put in a cut, counted in recursive fields crossed from
// rv (the depth of the uniform type that holds the data), or -1 when the
// decoder filled no cut. A field is in cut form when its native type is its
// cut type; otherwise it is unrolled and holds typed values.
func (m *Machine) cutDepth(rv reflect.Value, t types.Type, seen map[uintptr]bool, steps *int) int {
	if !m.hasCut(t) || *steps <= 0 {
		return -1
	}
	*steps--
	if n := m.recNamedOf(t); n != nil {
		return m.namedCutDepth(rv, n, seen, steps)
	}
	best := -1
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if rv.IsNil() || seen[rv.Pointer()] {
			return -1
		}
		seen[rv.Pointer()] = true
		return m.cutDepth(rv.Elem(), u.Elem(), seen, steps)
	case *types.Slice, *types.Array:
		et := u.(interface{ Elem() types.Type }).Elem()
		for i := 0; i < rv.Len(); i++ {
			best = max(best, m.cutDepth(rv.Index(i), et, seen, steps))
		}
	case *types.Map:
		it := rv.MapRange()
		for it.Next() {
			best = max(best, m.cutDepth(it.Value(), u.Elem(), seen, steps))
		}
	case *types.Struct:
		cut := m.recursiveFields(u)
		for i := 0; i < u.NumFields(); i++ {
			f, ft := rv.Field(i), u.Field(i).Type()
			switch {
			case !cut[i]:
				best = max(best, m.cutDepth(f, ft, seen, steps))
			case f.Type() == m.cutType(ft, u):
				m.eachCut(f, ft, u, func(slot reflect.Value, st types.Type) {
					if slot.IsNil() {
						return
					}
					e := slot.Elem()
					if e.Type() != m.reflectType(st) {
						best = max(best, 1+m.genericDepth(st, e, steps)) // a value of the decoder
					} else if d := m.cutDepth(e, st, seen, steps); d >= 0 {
						best = max(best, 1+d)
					}
				})
			default:
				if d := m.cutDepth(f, ft, seen, steps); d >= 0 {
					best = max(best, 1+d)
				}
			}
		}
	}
	return best
}

// namedCutDepth is cutDepth for rv, the native value of n, a selfRecursive
// type, in cut form (cutNamed) or unrolled (unrolledNamed). Each part that
// refers back to n counts as one level, like a recursive field. A value of
// the decoder in a cut (any) is found by its type, as for structs. For
// type M map[string]M the decoder's map[string]any for a JSON object is
// also the native type of M: it is walked as a typed value, so valid data
// needs no unrolling, and only a value of another kind (a number where an
// M is expected) makes the type deeper; the decoder then reports it with
// its own error, as with go run.
func (m *Machine) namedCutDepth(rv reflect.Value, n *types.Named, seen map[uintptr]bool, steps *int) int {
	_, elemCut := m.namedCuts(n)
	part := func(p reflect.Value, pt types.Type) int {
		if !elemCut {
			return m.cutDepth(p, pt, seen, steps)
		}
		if p.Kind() == reflect.Interface && !types.IsInterface(pt) { // cut form
			if p.IsNil() {
				return -1
			}
			p = p.Elem()
			if p.Type() != m.reflectType(pt) {
				return 1 + m.genericDepth(pt, p, steps) // a value of the decoder
			}
		}
		if d := m.cutDepth(p, pt, seen, steps); d >= 0 {
			return 1 + d
		}
		return -1
	}
	best := -1
	switch u := n.Underlying().(type) {
	case *types.Pointer:
		if rv.IsNil() || seen[rv.Pointer()] {
			return -1
		}
		seen[rv.Pointer()] = true
		return part(rv.Elem(), u.Elem())
	case *types.Slice:
		// a native value can contain itself (s[0] = s): each array is
		// walked once (an odd key, apart from the pointers)
		if rv.Len() == 0 || seen[rv.Pointer()+1] {
			return -1
		}
		seen[rv.Pointer()+1] = true
		for i := 0; i < rv.Len(); i++ {
			best = max(best, part(rv.Index(i), u.Elem()))
		}
	case *types.Array:
		for i := 0; i < rv.Len(); i++ {
			best = max(best, part(rv.Index(i), u.Elem()))
		}
	case *types.Map:
		if rv.IsNil() || seen[rv.Pointer()] {
			return -1
		}
		seen[rv.Pointer()] = true
		it := rv.MapRange()
		for it.Next() {
			best = max(best, part(it.Value(), u.Elem()))
		}
	}
	return best
}

// genericDepth returns the number of recursive fields crossed by the
// deepest path of g, the generic value a decoder made for a value of type t
// (maps with string keys for structs). The keys are matched with the field
// names and tags, as in growGeneric.
func (m *Machine) genericDepth(t types.Type, g reflect.Value, steps *int) int {
	for g.Kind() == reflect.Interface {
		if g.IsNil() {
			return 0
		}
		g = g.Elem()
	}
	if !g.IsValid() || !m.hasCut(t) || *steps <= 0 {
		return 0
	}
	*steps--
	best := 0
	if n := m.recNamedOf(t); n != nil {
		// each part that refers back to n is one level (namedCutDepth)
		_, elemCut := m.namedCuts(n)
		part := func(et types.Type, e reflect.Value) {
			if isNilGeneric(e) {
				return
			}
			d := m.genericDepth(et, e, steps)
			if elemCut {
				d++
			}
			best = max(best, d)
		}
		switch u := n.Underlying().(type) {
		case *types.Pointer:
			part(u.Elem(), g)
		case *types.Slice, *types.Array:
			if g.Kind() == reflect.Slice || g.Kind() == reflect.Array {
				et := u.(interface{ Elem() types.Type }).Elem()
				for i := 0; i < g.Len(); i++ {
					part(et, g.Index(i))
				}
			}
		case *types.Map:
			if g.Kind() == reflect.Map {
				it := g.MapRange()
				for it.Next() {
					part(u.Elem(), it.Value())
				}
			}
		}
		return best
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return m.genericDepth(u.Elem(), g, steps)
	case *types.Slice, *types.Array:
		if g.Kind() == reflect.Slice || g.Kind() == reflect.Array {
			et := u.(interface{ Elem() types.Type }).Elem()
			for i := 0; i < g.Len(); i++ {
				best = max(best, m.genericDepth(et, g.Index(i), steps))
			}
		}
	case *types.Map:
		if g.Kind() == reflect.Map {
			it := g.MapRange()
			for it.Next() {
				best = max(best, m.genericDepth(u.Elem(), it.Value(), steps))
			}
		}
	case *types.Struct:
		if g.Kind() != reflect.Map || g.Type().Key().Kind() != reflect.String {
			return 0
		}
		cut := m.recursiveFields(u)
		for i := 0; i < u.NumFields(); i++ {
			ft := u.Field(i).Type()
			if !m.hasCut(ft) {
				continue
			}
			if u.Field(i).Embedded() && !cut[i] {
				best = max(best, m.genericDepth(ft, g, steps)) // json promotes its fields
				continue
			}
			it := g.MapRange()
			for it.Next() {
				if !fieldKey(u, i, it.Key().String()) || isNilGeneric(it.Value()) {
					continue
				}
				d := m.genericDepth(ft, it.Value(), steps)
				if cut[i] {
					d++
				}
				best = max(best, d)
			}
		}
	}
	return best
}

// isNilGeneric reports whether g holds no value (a JSON null).
func isNilGeneric(g reflect.Value) bool {
	for g.Kind() == reflect.Interface {
		if g.IsNil() {
			return true
		}
		g = g.Elem()
	}
	return !g.IsValid()
}

// eachCut calls fn for each cut of rv, a field of type t in cut form
// (cutType(t, st)), with the static type of the cut.
func (m *Machine) eachCut(rv reflect.Value, t types.Type, st *types.Struct, fn func(slot reflect.Value, t types.Type)) {
	if !m.reaches(t, st, map[types.Type]bool{}) {
		return
	}
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		if _, ok := u.Underlying().(*types.Struct); ok || m.recNamedOf(u) != nil {
			fn(rv, t) // cutType made it an any
			return
		}
		m.eachCut(rv, u.Underlying(), st, fn)
	case *types.Slice:
		for i := 0; i < rv.Len(); i++ {
			m.eachCut(rv.Index(i), u.Elem(), st, fn)
		}
	case *types.Array:
		for i := 0; i < rv.Len(); i++ {
			m.eachCut(rv.Index(i), u.Elem(), st, fn)
		}
	case *types.Map:
		it := rv.MapRange()
		for it.Next() {
			m.eachCut(it.Key(), u.Key(), st, fn)
			m.eachCut(it.Value(), u.Elem(), st, fn)
		}
	case *types.Chan, *types.Signature:
	default: // pointers, structs: cutType made them an any
		fn(rv, t)
	}
}

// growGeneric unrolls sh, the shape of the struct reached through t, along
// g, the generic value a decoder made for a value of type t (maps with
// string keys for structs). The keys are matched with the field names and
// tags.
func (m *Machine) growGeneric(sh *shape, t types.Type, g reflect.Value, steps *int) {
	for g.Kind() == reflect.Interface {
		if g.IsNil() {
			return
		}
		g = g.Elem()
	}
	if !m.hasCut(t) || *steps <= 0 || m.recNamedOf(t) != nil {
		return // a selfRecursive type is not unrolled along the data
	}
	*steps--
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		m.growGeneric(sh, u.Elem(), g, steps)
	case *types.Slice, *types.Array:
		if g.Kind() == reflect.Slice || g.Kind() == reflect.Array {
			et := u.(interface{ Elem() types.Type }).Elem()
			for i := 0; i < g.Len(); i++ {
				m.growGeneric(sh, et, g.Index(i), steps)
			}
		}
	case *types.Map:
		if g.Kind() == reflect.Map {
			it := g.MapRange()
			for it.Next() {
				m.growGeneric(sh, u.Elem(), it.Value(), steps)
			}
		}
	case *types.Struct:
		if g.Kind() != reflect.Map || g.Type().Key().Kind() != reflect.String {
			return
		}
		cut := m.recursiveFields(u)
		for i := 0; i < u.NumFields(); i++ {
			ft := u.Field(i).Type()
			if !m.hasCut(ft) {
				continue
			}
			if u.Field(i).Embedded() && !cut[i] {
				m.growGeneric(sh.kid(i), ft, g, steps) // json promotes its fields
				continue
			}
			it := g.MapRange()
			for it.Next() {
				if fieldKey(u, i, it.Key().String()) {
					m.growGeneric(sh.kid(i), ft, it.Value(), steps)
				}
			}
		}
	}
}

// fieldKey reports whether a decoder may store the value of key in the field
// i of st: the key is the field name or a name of its tags, in any case.
func fieldKey(st *types.Struct, i int, key string) bool {
	if strings.EqualFold(key, st.Field(i).Name()) {
		return true
	}
	tag := reflect.StructTag(st.Tag(i))
	for _, k := range []string{"json", "mapstructure", "yaml", "toml"} {
		if n, _, _ := strings.Cut(tag.Get(k), ","); n != "" && strings.EqualFold(n, key) {
			return true
		}
	}
	return false
}

// cutKindOK reports whether a value of kind k, found in a cut, can be the
// native value of the static type t.
func cutKindOK(k reflect.Kind, t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Struct:
		return k == reflect.Struct
	case *types.Pointer:
		return k == reflect.Pointer
	case *types.Slice:
		return k == reflect.Slice
	case *types.Map:
		return k == reflect.Map
	case *types.Array:
		return k == reflect.Array
	}
	return true
}
