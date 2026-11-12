package vm

// Iteration 2 of the spike: defer/recover, goroutines, channels, select,
// range over maps/strings, map keys of any comparable type.

import (
	"fmt"
	"go/types"
	"math"
	"os"
	"reflect"
	"sync"
	"unicode/utf8"

	"golang.org/x/tools/go/ssa"
)

// callDesc describes a call whose arguments are evaluated now and executed
// later (defer) or elsewhere (go).
type callDesc struct {
	static  *Func
	gofn    func([]Value) Value // genericIntrinsic, with identity arg indices
	nat     *nativeSite         // with identity arg indices
	fnReg   int32
	recvReg int32
	method  string
	sig     *types.Signature
	builtin string
	args    []int32
	ident   []int32
	types   []types.Type
	goStmt  bool       // a go statement (else a defer statement)
	hook    NativeHook // a native function replaced by a vmHooks entry
	stack   int32      // register of the defer stack (ssa:deferstack), or -1
	// memCheck: as invokeSite.memCheck (nativemem.go)
	memCheck bool
}

func (c *compiler) callDesc(cc *ssa.CallCommon) *callDesc {
	d := &callDesc{fnReg: -1, recvReg: -1, stack: -1, args: c.regList(cc.Args)}
	d.ident = make([]int32, len(d.args))
	for i := range d.ident {
		d.ident[i] = int32(i)
	}
	for _, a := range cc.Args {
		d.types = append(d.types, a.Type())
	}
	if cc.IsInvoke() {
		d.recvReg, d.method, d.sig = c.reg(cc.Value), cc.Method.Name(), cc.Method.Type().(*types.Signature)
		d.memCheck = c.m.mem != nil && nativeMemMethodNames[d.method]
		return d
	}
	switch callee := cc.Value.(type) {
	case *ssa.Builtin:
		d.builtin = callee.Name()
	case *ssa.Function:
		if isVMGeneric(callee) { // as in compiler.call
			d.gofn = c.m.genericIntrinsic(callee, d.ident)
		} else if h := c.m.storageHook(callee); h != nil {
			// as in compiler.call: defer fs.IntVar(&n, ...) (flagvars.go)
			d.hook = func(m *Machine, a []Value) Value { return h(m, callee, a) }
		} else if h := c.m.nativeMemHook(callee); h != nil {
			d.hook = h // as in compiler.call: defer io.ReadAll(r) (nativemem.go)
		} else if h := c.m.nativeMemCall(callee); h != nil {
			d.hook = h // defer strings.Repeat(s, n): the size check, then the call
		} else if c.m.isInterp(callee) {
			d.static = c.m.funcOf(callee)
		} else if h := hookOf(callee); h != nil {
			d.hook = h // defer runtime.Goexit(), go sort.Slice(...)
		} else if h := c.m.bodylessHook(callee); h != nil {
			d.hook = h // linkname.go
		} else {
			d.nat = c.m.nativeSite(callee, d.ident)
		}
	default:
		d.fnReg, d.sig = c.reg(cc.Value), cc.Signature()
	}
	return d
}

func (cx *ctx) prepare(d *callDesc, regs []Value) deferred {
	m := cx.m
	args := make([]Value, len(d.args))
	for i, a := range d.args {
		args[i] = regs[a]
	}
	out := deferred{depth: len(cx.frames), args: args}
	switch {
	case d.gofn != nil:
		out.nat = func() { d.gofn(args) }
	case d.static != nil:
		out.fn = d.static
	case d.hook != nil:
		out.nat = func() { d.hook(m, args) }
	case d.nat != nil:
		out.nat = func() { m.callNativeSite(d.nat, args) }
	case d.method != "":
		iv, _ := regs[d.recvReg].R.(*Iface)
		if iv == nil {
			panic(errNilDeref)
		}
		if iv.T.Native != nil {
			out.nat = func() {
				if d.memCheck {
					m.nativeMemInvoke(nil, iv.V, d.method, args, d.ident)
				}
				m.callNativeMethod(iv.V.R.(reflect.Value), d.method, d.sig, args, d.ident)
			}
			break
		}
		fn := iv.T.Methods[d.method]
		if !m.isInterp(fn.ssa) {
			out.nat = func() {
				if d.memCheck {
					m.nativeMemInvoke(fn.ssa, iv.V, d.method, args, d.ident)
				}
				m.callBodyless(fn.ssa, iv.V, args, d.ident)
			}
			break
		}
		recv := iv.V
		out.fn, out.recv = fn, &recv
	case d.fnReg >= 0:
		switch cl := regs[d.fnReg].R.(type) {
		case *Closure:
			if cl.Direct != nil {
				out.fn, out.recv = cl.Direct, cl.Recv
				break
			}
			if cl.GoP != nil {
				out.natP = func(p *panicState) { cl.GoP(p, args) }
				break
			}
			if cl.Go != nil { // VM intrinsic (sync.OnceFunc, iter.Pull's stop...)
				out.nat = func() { cl.Go(args) }
				break
			}
			out.fn, out.env = cl.Fn, cl.Env
		case reflect.Value:
			if !cl.IsValid() || cl.IsNil() {
				out.nat = nilFuncCall(d)
				break
			}
			out.nat = func() { m.callNativeValue(cl, d.sig, args, d.ident) }
		default:
			out.nat = nilFuncCall(d)
		}
	default:
		switch d.builtin {
		case "close":
			out.nat = func() { closeChan(args[0].R) }
		case "print", "println":
			ln := d.builtin == "println"
			w := m.printOut()
			out.nat = func() { writePrint(w, args, nil, d.types, ln) }
		case "recover":
			// defer recover(): recovers when the deferring function was
			// itself called directly by a panic (see runDefers).
			out.nat, out.recover = func() {}, true
		case "panic":
			out.nat = func() { panic(m.newPanic(args[0])) }
		case "copy":
			et := sliceElem(d.types[0])
			out.nat = func() { m.execCopy(args[0].R, args[1].R, et) }
		case "clear":
			t := d.types[0]
			out.nat = func() { m.execClear(args[0].R, t) }
		case "delete":
			kt := d.types[0].Underlying().(*types.Map).Key()
			out.nat = func() { m.execDelete(args[0], args[1], kt) }
		default:
			unsupported("deferred builtin %s", d.builtin)
		}
	}
	return out
}

// errGoNil is raised by "go f()" when f is nil. gc stops the program
// (fatal error); the VM panics in the goroutine that runs the go
// statement, so a plugin cannot stop the host this way.
var errGoNil = plainError("go of nil func value")

// nilFuncCall returns the deferred call of a nil func value. For a defer
// statement, the deferred call (not the statement) panics. A go statement
// panics now, in the calling goroutine, like gc.
func nilFuncCall(d *callDesc) func() {
	if d.goStmt {
		panic(errGoNil)
	}
	return func() { panic(errNilDeref) }
}

// goStarted and goEnded count the goroutines of the program.
func (m *Machine) goStarted() {
	m.liveMu.Lock()
	m.live++
	m.liveMu.Unlock()
}

func (m *Machine) goEnded() {
	m.liveMu.Lock()
	if m.live--; m.live == 0 && m.liveCond != nil {
		m.liveCond.Broadcast()
	}
	m.liveMu.Unlock()
}

// waitGoroutines blocks until no goroutine started by the program runs.
func (m *Machine) waitGoroutines() {
	m.liveMu.Lock()
	if m.liveCond == nil {
		m.liveCond = sync.NewCond(&m.liveMu)
	}
	for m.live > 0 {
		m.liveCond.Wait()
	}
	m.liveMu.Unlock()
}

func (cx *ctx) spawn(d deferred) {
	m := cx.m
	m.goReserve()
	m.goStarted()
	// The goroutine uses the account of the call that starts it, while
	// that call runs, then the background rule (budget.go). It has a new
	// Go stack (its depth starts at zero). The callbacks it gets from
	// native code continue from it (nest.go), also when it runs a native
	// function.
	sub := m.goCtx(cx)
	go func() {
		defer m.goEnded()
		defer m.goRelease()
		m.runGoroutine(d, sub)
	}()
}

// runGoroutine runs d as the body of a goroutine of the program, on the
// current goroutine, which must run no interpreted code yet, with the ctx
// sub (nil: a new background ctx, budget.go): d has a new Go stack, and a
// panic that leaves d (also ErrBudget and ErrStackOverflow) goes to GoPanic
// and never further. Without GoPanic, it ends the process as in Go. The go
// statement uses it, and the callbacks that the runtime calls on a
// goroutine of its own (runtime.AddCleanup).
func (m *Machine) runGoroutine(d deferred, sub *ctx) {
	if sub == nil {
		sub = m.goCtx(nil)
	}
	defer func() {
		if r := recover(); r != nil {
			if r == errGoexit || quietStop(r) {
				return // runtime.Goexit, or the call was canceled: the deferred calls ran
			}
			if m.GoPanic != nil {
				m.GoPanic(r)
				return
			}
			fmt.Fprintln(os.Stderr, panicLine(r))
			fmt.Fprint(os.Stderr, "\n", sub.traceback())
			os.Exit(2)
		}
	}()
	defer sub.giveBack()
	// The callbacks that d gets from native code continue from sub
	// (nest.go), also when d is a native function.
	g := getg()
	sub.gs, _ = enterG(g, sub)
	if d.natP != nil || d.nat != nil {
		gs := sub.gs
		gs.cur = sub
		defer leaveG(g, gs)
		if d.natP != nil {
			d.natP(nil)
		} else {
			d.nat()
		}
		sub.checkFault()
		return
	}
	args := d.args
	if d.recv != nil {
		args = append([]Value{*d.recv}, args...)
	}
	sub.callIn(g, nil, d.fn, args, d.env)
}

var emptyIface = types.NewInterfaceType(nil, nil)

func (m *Machine) panicValue(r any) Value {
	if p, ok := r.(*VMPanic); ok {
		return p.V
	}
	return m.fromNative(reflect.ValueOf(&r).Elem(), emptyIface)
}

// ---- select ----

type selectState struct {
	send  bool
	ch    int32
	val   int32
	index int        // index among recv states (for the result tuple)
	elem  types.Type // element type (native channels)
}

type selectSite struct {
	states   []selectState
	blocking bool
	nrecv    int
}

func (c *compiler) selectSite(v *ssa.Select) *selectSite {
	s := &selectSite{blocking: v.Blocking}
	for _, st := range v.States {
		ss := selectState{ch: c.reg(st.Chan), val: -1, index: -1, elem: chanElem(st.Chan.Type())}
		if st.Dir == types.SendOnly {
			ss.send, ss.val = true, c.reg(st.Send)
		} else {
			ss.index = s.nrecv
			s.nrecv++
		}
		s.states = append(s.states, ss)
	}
	return s
}

var chanValueType = reflect.TypeOf((chan Value)(nil))

// chanElem returns the element type of a channel type.
func chanElem(t types.Type) types.Type {
	return t.Underlying().(*types.Chan).Elem()
}

// closeChan closes an interpreted or a native channel.
func closeChan(ch any) {
	if rv, ok := ch.(reflect.Value); ok {
		rv.Close()
		return
	}
	c, _ := ch.(chan Value)
	if c == nil {
		panic(errCloseNilChan)
	}
	close(c)
}

func (cx *ctx) execSelect(s *selectSite, regs []Value) Value {
	m := cx.m
	cases := make([]reflect.SelectCase, 0, len(s.states)+2)
	for _, st := range s.states {
		var rc reflect.Value
		switch ch := regs[st.ch].R.(type) {
		case chan Value:
			rc = reflect.ValueOf(ch)
		case reflect.Value: // native channel
			rc = ch
		default:
			rc = reflect.Zero(chanValueType)
		}
		if st.send {
			sv := reflect.ValueOf(regs[st.val])
			if rc.Type() != chanValueType {
				sv = m.toNative(regs[st.val], st.elem)
			}
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectSend, Chan: rc, Send: sv})
		} else {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: rc})
		}
	}
	if !s.blocking {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	} else {
		// In a call from the host, the wait ends when the account stops
		// (budget.go); always when the machine closes (close.go).
		as, ms := cx.stopChans()
		cases = append(cases,
			reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(as)},
			reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ms)})
	}
	chosen, recv, ok := reflect.Select(cases)
	if s.blocking {
		if chosen >= len(s.states) {
			cx.halted()
		}
		cx.blocked()
	}
	t := make([]Value, 2+s.nrecv)
	if chosen == len(s.states) {
		t[0].N = -1
		return Value{R: t}
	}
	t[0].N = int64(chosen)
	t[1].N = b2i(ok)
	if st := s.states[chosen]; !st.send {
		switch {
		case !ok:
			t[2+st.index] = deepCopy(m.zero(st.elem), st.elem)
		case recv.Type() == reflect.TypeFor[Value]():
			t[2+st.index] = recv.Interface().(Value)
		default:
			t[2+st.index] = m.fromNative(recv, st.elem)
		}
	}
	return Value{R: t}
}

// ---- maps ----

type ifKey struct {
	T *RType
	K any
}

// tupleKey is the key of a struct or an array: the list of the keys of its
// elements. Go's == on it compares the elements one by one, with the rules
// of the interpreted ==: +0 equals -0, and NaN equals nothing.
type tupleKey struct {
	head, tail any
}

// nativeKey is the key of a value of a native type kept as a reflect.Value
// (time.Time, for example).
type nativeKey struct {
	V any
}

// keyOf turns an interpreted key into a comparable Go value.
func keyOf(v Value, t types.Type) any {
	if rv, ok := v.R.(reflect.Value); ok {
		// reflect.Value.Pointer of a native value keeps the value in R
		// (vrPointer): the address in N is the key, and the value may be a map.
		if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.Uintptr && rv.Kind() != reflect.Uintptr {
			return v.N
		}
		// Native value: its Go value is the key, and Go's == on it is the
		// right equality.
		if !rv.IsValid() {
			return nil
		}
		return nativeKey{rv.Interface()}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			if _, ok := v.R.(string); ok {
				return v.R // already boxed: boxing the string again allocates
			}
			return ""
		case u.Info()&types.IsFloat != 0:
			return math.Float64frombits(uint64(v.N))
		case u.Info()&types.IsComplex != 0:
			return cplx(v)
		case u.Kind() == types.UnsafePointer:
			return addrOf(v)
		}
		return v.N
	case *types.Interface:
		iv, _ := v.R.(*Iface)
		if iv == nil {
			return nil
		}
		if iv.T.Native != nil {
			return ifKey{iv.T, iv.V.R.(reflect.Value).Interface()}
		}
		if nk := iv.T.nat; nk != nil {
			// a named native type: the key of the same value from native code
			return ifKey{nk.rt, nk.key(iv.V)}
		}
		if vt, ok := iv.V.R.(*vtype); ok {
			return ifKey{iv.T, vt}
		}
		if iv.T.noCmp {
			panic(runtimeError("hash of unhashable type " + typeString(iv.T.T)))
		}
		if pt, ok := iv.T.T.Underlying().(*types.Pointer); ok {
			return ifKey{iv.T, ptrKeyOf(iv.V, iv.T.ptrInfo(pt), pt)}
		}
		return ifKey{iv.T, keyOf(iv.V, iv.T.T)}
	case *types.Struct:
		fs, _ := v.R.([]Value)
		var k any
		for i := len(fs) - 1; i >= 0; i-- {
			var fk any // blank fields are not compared
			if u.Field(i).Name() != "_" {
				fk = keyOf(fs[i], u.Field(i).Type())
			}
			k = tupleKey{fk, k}
		}
		return k
	case *types.Array:
		es, _ := v.R.([]Value)
		var k any
		for i := len(es) - 1; i >= 0; i-- {
			k = tupleKey{keyOf(es[i], u.Elem()), k}
		}
		return k
	case *types.Pointer:
		return ptrKey(v, u)
	}
	return v.R // chans: identity
}

type mapSite struct {
	key, elem types.Type
	ptr       ptrInfo // for a pointer key: computed once, at compile time
	nan       bool    // a key may be NaN or hold a NaN (see storeKey)
	// Range sites only: the loop never reads the key (the value).
	noKey, noElem bool
}

// keyOf is keyOf(v, s.key), without the type walk for a pointer key. A
// store must still pass the result to storeKey when s.nan is set.
func (s *mapSite) keyOf(v Value) any {
	if s.ptr != 0 {
		return ptrKeyOf(v, s.ptr, nil)
	}
	return keyOf(v, s.key)
}

// mayNaN reports whether a value of type t may be a NaN or hold one (a
// float or complex, an interface, or a struct or array of them).
func mayNaN(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&(types.IsFloat|types.IsComplex) != 0
	case *types.Interface:
		return true
	case *types.Array:
		return mayNaN(u.Elem())
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if mayNaN(u.Field(i).Type()) {
				return true
			}
		}
	}
	return false
}

// mapIter ranges over a map.
//
// A small VM map (at most smallRange entries) copies its keys at the
// start. A larger one is walked live with a reflect.MapIter: a copy of all
// the keys made "range m { delete(m, k); break }" quadratic (GOROOT test
// maplinear.go), and the Go runtime gives the semantics of the spec for
// entries deleted, added or cleared during the range.
//
// Entries with a NaN key (nanKey) are given after the others, from a copy
// (keys and values paired) and only while no clear happened: a native call
// that writes the map back stores them under new nanKeys. The live walk
// skips them and copies them when it ends.
type mapIter struct {
	m      *MapV
	keys   []any
	buf    [smallRange]any
	nan    [][2]Value // entries with a NaN key (nanKey)
	i      int
	nat    *reflect.MapIter
	nk, nv reflect.Value // nat: reused slots for scalar keys and values
	// nkBuf and nvBuf keep the slots of nk and nv for the next use of the
	// iterator (natSlot), zeroed so that they keep nothing alive.
	nkBuf, nvBuf reflect.Value
	site         *mapSite
	clears       int // m.clears at the start
	tup          tuple

	live     *reflect.MapIter // larger maps: live walk over m.M
	liveBuf  *reflect.MapIter // the MapIter of live, kept for the next use of the iterator
	nanLater bool             // the live walk skips nanKey entries
	k        any              // key slot for live.SetIterKey
	v        Value            // value slot for live.SetIterValue
	kv       reflect.Value    // addressable view of k
	vv       reflect.Value    // addressable view of v
}

const smallRange = 8

type strIter struct {
	s   string
	pos int
	tup tuple
}

// tuple is the result of the OpNext of one iterator, reused at each turn.
// The result of a Next is only read by the Extract instructions of the same
// turn of the loop, before the next Next of this iterator, so one buffer per
// iterator is enough. r holds v[:] boxed once: a turn allocates nothing.
type tuple struct {
	v [3]Value
	r any
}

func (t *tuple) set(ok int64, k, v Value) Value {
	if t.r == nil {
		t.r = t.v[:]
	}
	t.v = [3]Value{{N: ok}, k, v}
	return Value{R: t.r}
}

// done is the last result of an iterator: a shared tuple that nothing
// writes, so that the iterator can go back to its pool before the caller
// reads the result.
func (t *tuple) done() Value { return doneTuple }

var doneTuple = Value{R: make([]Value, 3)}

// The iterators of the range loops over interpreted maps and strings go
// back to a pool when the loop has seen all the entries (their last Next):
// ssa never calls Next again after it said done, and the register that
// still holds the iterator is dead. A loop left early (break, return,
// panic) leaves its iterator to the GC. VM_OPT=-iterpool turns it off.
var (
	mapIterPool = sync.Pool{New: func() any { return new(mapIter) }}
	strIterPool = sync.Pool{New: func() any { return new(strIter) }}
)

func newMapIter() *mapIter {
	if optIterPool {
		return mapIterPool.Get().(*mapIter)
	}
	return new(mapIter)
}

// release gives the iterator of a map back to the pool. It keeps the
// reusable parts: the MapIter of the live walk (or of the native map) and
// its slots, and the slice of its tuple.
func (it *mapIter) release() {
	if !optIterPool {
		return
	}
	lb, kv, vv, r, nkb, nvb := it.liveBuf, it.kv, it.vv, it.tup.r, it.nkBuf, it.nvBuf
	if lb != nil {
		lb.Reset(reflect.Value{}) // do not keep the map alive
	}
	for _, b := range [2]reflect.Value{nkb, nvb} {
		if b.IsValid() {
			b.SetZero()
		}
	}
	*it = mapIter{}
	it.liveBuf, it.kv, it.vv, it.tup.r, it.nkBuf, it.nvBuf = lb, kv, vv, r, nkb, nvb
	mapIterPool.Put(it)
}

// natSlot returns the slot *buf when it holds a variable of type t, else a
// new variable of type t, kept in *buf.
func natSlot(buf *reflect.Value, t reflect.Type) reflect.Value {
	if !buf.IsValid() || buf.Type() != t {
		*buf = reflect.New(t).Elem()
	}
	return *buf
}

func newStrIter(s string) *strIter {
	if optIterPool {
		it := strIterPool.Get().(*strIter)
		it.s = s
		return it
	}
	return &strIter{s: s}
}

func (it *strIter) release() {
	if !optIterPool {
		return
	}
	r := it.tup.r
	*it = strIter{}
	it.tup.r = r
	strIterPool.Put(it)
}

func (m *Machine) execRange(x any, site *mapSite) Value {
	switch x := x.(type) {
	case string:
		return Value{R: newStrIter(x)}
	case *MapV:
		if m.panicOnFatal {
			x.beginRead(errMapIterWrite)
			defer x.endRead()
		}
		it := newMapIter()
		it.m, it.site, it.clears = x, site, x.clears
		if len(x.M) > smallRange {
			if it.liveBuf != nil {
				it.liveBuf.Reset(reflect.ValueOf(x.M))
			} else {
				it.liveBuf = reflect.ValueOf(x.M).MapRange()
			}
			it.live = it.liveBuf
			it.nanLater = site != nil && site.nan
			if !it.kv.IsValid() {
				it.kv = reflect.ValueOf(&it.k).Elem()
				it.vv = reflect.ValueOf(&it.v).Elem()
			}
			return Value{R: it}
		}
		it.keys = it.buf[:0]
		for k, v := range x.M {
			if _, ok := k.(nanKey); ok {
				it.nan = append(it.nan, [2]Value{x.key(k), v})
				continue
			}
			it.keys = append(it.keys, k)
		}
		return Value{R: it}
	case reflect.Value:
		// The iterator, its reflect.MapIter and its slots come from the
		// pool, as for an interpreted map (release).
		it := newMapIter()
		if it.liveBuf != nil {
			it.liveBuf.Reset(x)
		} else {
			it.liveBuf = x.MapRange()
		}
		it.nat, it.site = it.liveBuf, site
		// Key() and Value() copy each entry to the heap; a scalar, or a
		// slice of scalars (fromNative gives a view with its own header),
		// can be read through one slot instead.
		if site != nil && x.Len() > 0 {
			if t := x.Type().Key(); !site.noKey && scalarKind(t.Kind()) {
				it.nk = natSlot(&it.nkBuf, t)
			}
			if t := x.Type().Elem(); !site.noElem && (scalarKind(t.Kind()) || t.Kind() == reflect.Slice && scalarKind(t.Elem().Kind())) {
				it.nv = natSlot(&it.nvBuf, t)
			}
		}
		return Value{R: it}
	case nil:
		if site == nil {
			return Value{R: newStrIter("")}
		}
		it := newMapIter()
		it.site = site
		return Value{R: it}
	}
	unsupported("range over %T", x)
	return Value{}
}

func (m *Machine) execNext(it any) Value {
	switch it := it.(type) {
	case *strIter:
		if it.pos >= len(it.s) {
			it.release()
			return doneTuple
		}
		r, size := utf8.DecodeRuneInString(it.s[it.pos:])
		pos := it.pos
		it.pos += size
		return it.tup.set(1, Value{N: int64(pos)}, Value{N: int64(r)})
	case *mapIter:
		if m.panicOnFatal && it.m != nil {
			return m.guardedMapNext(it)
		}
		return m.mapNext(it)
	}
	panic("bad iterator")
}

// mapNext is execNext for the range it over a map.
func (m *Machine) mapNext(it *mapIter) Value {
	if it.nat != nil {
		if !it.nat.Next() {
			it.release()
			return doneTuple
		}
		return it.tup.set(1, m.natIterKey(it), m.natIterElem(it))
	}
	if it.live != nil {
		for it.live.Next() {
			it.kv.SetIterKey(it.live)
			it.vv.SetIterValue(it.live)
			k, v := it.k, it.v
			it.k, it.v = nil, Value{} // do not keep the entry alive
			if _, ok := k.(nanKey); ok && it.nanLater {
				continue
			}
			return it.tup.set(1, it.m.key(k), v)
		}
		it.live = nil
		if it.nanLater && it.m.clears == it.clears {
			for k, v := range it.m.M {
				if _, ok := k.(nanKey); ok {
					it.nan = append(it.nan, [2]Value{it.m.key(k), v})
				}
			}
		}
	}
	for it.i < len(it.keys) {
		k := it.keys[it.i]
		it.i++
		if v, ok := it.m.M[k]; ok {
			return it.tup.set(1, it.m.key(k), v)
		}
	}
	if j := it.i - len(it.keys); j < len(it.nan) && it.m.clears == it.clears {
		it.i++
		return it.tup.set(1, it.nan[j][0], it.nan[j][1])
	}
	it.release()
	return doneTuple
}

// natIterKey returns the key of the current entry of the native map range
// it, or nothing when the loop does not read it.
func (m *Machine) natIterKey(it *mapIter) Value {
	switch {
	case it.site.noKey:
		return Value{}
	case it.nk.IsValid():
		it.nk.SetIterKey(it.nat)
		return m.fromNative(it.nk, it.site.key)
	}
	return m.fromNative(it.nat.Key(), it.site.key)
}

// natIterElem is natIterKey for the value.
func (m *Machine) natIterElem(it *mapIter) Value {
	switch {
	case it.site.noElem:
		return Value{}
	case it.nv.IsValid():
		it.nv.SetIterValue(it.nat)
		return m.fromNative(it.nv, it.site.elem)
	}
	return m.fromNative(it.nat.Value(), it.site.elem)
}
