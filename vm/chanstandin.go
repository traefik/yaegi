package vm

import (
	"go/types"
	htmltemplate "html/template"
	"reflect"
	"runtime"
	"sync"
	"text/template"
	"unsafe"
	"weak"
)

// An interpreted channel is a chan Value: native code cannot send to it or
// receive from it. When native code gets such a channel in an interface
// (json.Marshal(ch), a struct field of a value passed as any, a map of
// channels in fmt.Sprint...), it only looks at it with reflect, or stores it
// (a context value, a sync.Map). Then the channel gets a stand-in: a native
// channel of the same type and capacity, so that native code sees the right
// kind and type (json, xml and gob report "unsupported type: chan int", or
// skip a field with `json:"-"`, as in go run), and a stand-in that native
// code gives back converts back to the interpreted channel.
//
// The stand-in is closed: a native receive gets the zero value at once and
// a native send panics, instead of a silent hang on a channel that nothing
// else uses. A channel passed as a typed parameter (signal.Notify(c, ...))
// is still refused: native code would use it. The data of text/template
// and html/template gets no stand-in either: a template ranges over a
// channel and takes its length ({{range .C}}, {{len .C}}), and an empty
// stand-in would give a wrong result with no error (see readsChans).

// chanStandIn is the record of a stand-in, by its native address: what it
// gives back. It keeps the interpreted channel alive as long as native code
// keeps the stand-in.
type chanStandIn struct {
	nat weak.Pointer[byte] // the native stand-in
	ch  chan Value
	t   types.Type // the type the channel was converted as
}

// chanStandInKey finds the stand-in of an interpreted channel converted as
// a given type: the same channel gives the same stand-in (a native map key
// or == in native code), while the stand-in lives.
type chanStandInKey struct {
	ch uintptr
	t  *RType
}

// weakStandInKey is a chanStandInKey for a cleanup: its type is weak (see
// standIn).
type weakStandInKey struct {
	ch uintptr
	t  weak.Pointer[RType]
}

// chanStandInRef is the entry of chanStandInKey: weak in both directions,
// so that the table keeps neither channel alive.
type chanStandInRef struct {
	interp weak.Pointer[byte]
	nat    weak.Pointer[byte]
}

func chanWord(rv reflect.Value) *byte { return (*byte)(rv.UnsafePointer()) }

// chanFromWord returns the channel of type rt whose runtime pointer is p.
func chanFromWord(p *byte, rt reflect.Type) reflect.Value {
	w := unsafe.Pointer(p)
	return reflect.NewAt(rt, unsafe.Pointer(&w)).Elem()
}

// standIn returns the native stand-in of type rt for the interpreted
// channel ch converted as type t.
func (m *Machine) standIn(ch chan Value, t types.Type, rt reflect.Type) reflect.Value {
	ip := chanWord(reflect.ValueOf(ch))
	key := chanStandInKey{uintptr(unsafe.Pointer(ip)), m.rtypeOf(t)}
	if r, ok := m.chanStandIns.Load(key); ok {
		ref := r.(chanStandInRef)
		if ref.interp.Value() == ip {
			if np := ref.nat.Value(); np != nil {
				return chanFromWord(np, rt)
			}
		}
	}
	bt := rt
	if rt.ChanDir() != reflect.BothDir {
		bt = reflect.ChanOf(reflect.BothDir, rt.Elem()) // MakeChan and Close need both directions
	}
	nat := reflect.MakeChan(bt, cap(ch))
	nat.Close()
	if bt != rt {
		nat = nat.Convert(rt)
	}
	np := chanWord(nat)
	nk := uintptr(unsafe.Pointer(np))
	// The cleanups must not reach the Machine: a cleanup is a GC root
	// while its object lives, and the channel or the stand-in can live as
	// long as the Machine (a plugin global, a global sync.Map). So they do
	// not capture m, and they hold the tables and the type only weakly:
	// the record in a table holds the channel (its buffered values can
	// point to the Machine), and the methods of an RType hold Funcs, which
	// point to the Machine. The Machine holds the tables and its RTypes,
	// so when one of them is dead, the Machine is dead and there is
	// nothing to clean.
	sic, csi := m.standInChans, m.chanStandIns
	wsic, wcsi := weak.Make(sic), weak.Make(csi)
	sic.Store(nk, &chanStandIn{nat: weak.Make(np), ch: ch, t: t})
	runtime.AddCleanup(np, func(k uintptr) {
		sic := wsic.Value()
		if sic == nil {
			return
		}
		if r, ok := sic.Load(k); ok && r.(*chanStandIn).nat.Value() == nil {
			sic.CompareAndDelete(k, r)
		}
	}, nk)
	csi.Store(key, chanStandInRef{interp: weak.Make(ip), nat: weak.Make(np)})
	runtime.AddCleanup(ip, func(wk weakStandInKey) {
		csi, rt := wcsi.Value(), wk.t.Value()
		if csi == nil || rt == nil {
			return
		}
		k := chanStandInKey{wk.ch, rt}
		if r, ok := csi.Load(k); ok && r.(chanStandInRef).interp.Value() == nil {
			csi.CompareAndDelete(k, r)
		}
	}, weakStandInKey{key.ch, weak.Make(key.t)})
	return nat
}

// standInOf returns the record of rv when it is the stand-in of an
// interpreted channel, or nil.
func (m *Machine) standInOf(rv reflect.Value) *chanStandIn {
	if !rv.IsValid() || rv.Kind() != reflect.Chan || rv.IsNil() {
		return nil
	}
	np := chanWord(rv)
	r, ok := m.standInChans.Load(uintptr(unsafe.Pointer(np)))
	if !ok || r.(*chanStandIn).nat.Value() != np {
		return nil
	}
	return r.(*chanStandIn)
}

// standInIface returns the interpreted interface value that the native
// stand-in rv gives back (its channel, with the type it was converted as).
func (m *Machine) standInIface(rv reflect.Value) (*Iface, bool) {
	s := m.standInOf(rv)
	if s == nil {
		return nil, false
	}
	return &Iface{T: m.rtypeOf(s.t), V: Value{R: s.ch}}, true
}

// chanReaders are the native methods that receive from the channels of
// their data and take their length: the templates ({{range .C}},
// {{len .C}}). An empty closed stand-in would give them a wrong result with
// no error.
var chanReaders = map[methodKey]bool{
	{reflect.TypeFor[*template.Template](), "Execute"}:             true,
	{reflect.TypeFor[*template.Template](), "ExecuteTemplate"}:     true,
	{reflect.TypeFor[*htmltemplate.Template](), "Execute"}:         true,
	{reflect.TypeFor[*htmltemplate.Template](), "ExecuteTemplate"}: true,
}

// readsChans reports whether the native method name of t reads the
// channels of its arguments. While such a call runs on a goroutine, the
// interpreted channels converted on that goroutine get no stand-in: the
// data of the call, and what the funcs that the template calls return
// (a FuncMap func, {{call .F}}), made before the call or during it. The
// conversion fails (an opaque value), as for a channel that native code
// would use, so the template gives an error.
func readsChans(t reflect.Type, name string) bool {
	return chanReaders[methodKey{t, name}]
}

// enterChanReader marks the start of a call of a chanReaders method on the
// goroutine, and returns the func that marks its end.
func enterChanReader() func() {
	gs := lookupG(getg())
	if gs == nil {
		return func() {}
	}
	gs.chanReaders++
	return func() { gs.chanReaders-- }
}

// inChanReader reports whether a call of a chanReaders method runs on the
// goroutine.
func inChanReader() bool {
	gs := lookupG(getg())
	return gs != nil && gs.chanReaders > 0
}

// The twin of reflect gives plugins the methods of chanReaders as func
// values (Value.MethodByName, Type.MethodByName(...).Func). Their callers
// (RV.call, callNativeValue) convert the arguments themselves, so such a
// func is marked: chanReaderFunc wraps it, and the callers check the mark
// (isChanReaderFunc) before they convert the arguments.
//
// The key is a weak pointer to the wrapper, not its address: a cleanup runs
// after its object is freed, so a new func can get the address of a dead
// wrapper before the cleanup removes it. A new object never gets the weak
// pointer of an old one, so the keys of a dead wrapper and of a new func at
// the same address are different.
var chanReaderFuncs sync.Map // weak.Pointer[byte] of a wrapper -> struct{}

// chanReaderFunc returns fn, the method name of t, marked when it is a
// method of chanReaders.
func chanReaderFunc(t reflect.Type, name string, fn reflect.Value) reflect.Value {
	if !fn.IsValid() || !readsChans(t, name) {
		return fn
	}
	ft := fn.Type()
	w := reflect.MakeFunc(ft, func(in []reflect.Value) []reflect.Value {
		defer enterChanReader()() // a native caller (callNativeSlow marks it too)
		if ft.IsVariadic() {
			return fn.CallSlice(in)
		}
		return fn.Call(in)
	})
	if p, ok := funcWord(w); ok {
		k := weak.Make((*byte)(p))
		chanReaderFuncs.Store(k, struct{}{})
		runtime.AddCleanup((*byte)(p), func(k weak.Pointer[byte]) { chanReaderFuncs.Delete(k) }, k)
	}
	return w
}

// makeFuncPC is the code pointer of the funcs made by reflect.MakeFunc.
var makeFuncPC = reflect.MakeFunc(reflect.TypeFor[func()](), nil).Pointer()

// isChanReaderFunc reports whether fn is marked by chanReaderFunc.
func isChanReaderFunc(fn reflect.Value) bool {
	// Only a func made by reflect.MakeFunc can be a wrapper: they all have
	// the code pointer of makeFuncStub. This keeps weak.Make (a weak handle
	// for each func object) off the other funcs.
	if fn.Kind() != reflect.Func || fn.IsNil() || fn.Pointer() != makeFuncPC {
		return false
	}
	p, ok := funcWord(fn)
	if !ok {
		return false
	}
	_, ok = chanReaderFuncs.Load(weak.Make((*byte)(p)))
	return ok
}
