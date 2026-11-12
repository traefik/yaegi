package vm

// Callbacks on goroutines that native code starts.
//
// Native code can run a func value of the program on a goroutine of its
// own: time.AfterFunc, context.AfterFunc, a finalizer (on the finalizer
// goroutine of the runtime), a goroutine of a library. In Go, a panic that
// nobody recovers on such a goroutine ends the process. The goroutines that
// the program starts with a go statement send their panics to
// Machine.GoPanic instead (spawn), so that a plugin cannot end the host. The
// callbacks of native goroutines need the same rule: a plugin panic, a
// runtime error, ErrBudget, ErrStackOverflow, or a panic of the VM itself.
//
// Such a callback is the outermost run of interpreted code on its goroutine
// (the goroutine has no gstate, nest.go). When the Machine has a GoPanic
// (plugins), this outermost run recovers the panic that leaves it and gives
// it to GoPanic. What the native code that called it gets then is in
// afterPanic: zero results are not safe when the callback has results.
// Some outermost runs keep the Go semantics, because their caller expects
// the panic:
//   - the calls from the host (Machine.top: init, New, each ServeHTTP...),
//     which return the panic to the host, as before. Only the host calls
//     top directly: a plugin gets the handler of the next middleware (maybe
//     another plugin, whose ServeHTTP calls top) through nativeNext;
//   - the sequence of iter.Pull, which runs on its own goroutine and gives
//     its panic to the caller of next (iterpull.go, through recovering);
//   - the handlers that net/http runs on goroutines of its own and whose
//     panics it recovers (http.TimeoutHandler, the servers: nativeRecovers).
//
// Callbacks that native code calls on a goroutine that already runs
// interpreted code (sort.Slice, strings.Map, a String method called by fmt)
// are nested runs, not outermost ones: their panics go up to the
// interpreted caller, as in Go.
//
// runtime.Goexit in such a callback is not a panic: the goroutine ends, as
// in Go. ErrCanceled is not a panic of the plugin either: it comes from a
// call from the host below the callback (next, the handler of another
// plugin, called with a request whose context is done). The callback ends
// as after a panic (afterPanic), but GoPanic does not get it, as for the
// goroutines of a go statement (runGoroutine).
//
// Budget (budget.go). Such a callback is a call of its own: when the
// goroutine runs no interpreted code and has no pending account of
// Machine.top, the outermost run opens a new account with a full Budget and
// no context (callCtx). This holds also when the native goroutine was
// started during a call from the host that still runs (time.AfterFunc in a
// ServeHTTP), unless the func or value carries the account of that call
// (withOrigin, budget.go). With a MaxGoroutines, the callback counts as a
// goroutine of the plugin while it runs (nativeCallback,
// countNativeCallback). The runs nested in the callback share its account, and a
// call from the host inside it (next, through nativeNext or Machine.top)
// has an account of its own, as everywhere.
//
// The next handler of a plugin A can be the handler of a plugin B (the next
// middleware, or the same plugin twice in a chain), so it can call
// Machine.top. If native code that A starts calls next on a goroutine of
// its own (A gives next to fcgi.Serve), top is on the stack, but no host
// called it and nothing below recovers: the panic of B would end the
// process. So Plugin.Call and Object.Call give next to A inside a nativeNext. When it runs
// on a goroutine without interpreted code, nativeNext acts like
// callOutermost: a panic that leaves next goes to the GoPanic of A (the
// plugin that gave next to native code), unless a catcher is below it.
// The same holds for a panic of a host handler there.
//
// Two native handlers recover a panic and panic again on the goroutine of
// their ServeHTTP: Machine.top (topHandler) and http.TimeoutHandler. They
// are catchers only when that goroutine is safe. A plugin can give such a
// handler to native code that calls it on a goroutine of its own (A gives
// http.TimeoutHandler(h, d, msg) to fcgi.Serve, or a topHandler that the
// host put in the context of the request): the panic would leave on that
// goroutine, where nothing recovers. So each such handler that comes into
// the VM (ifaceVMs) is wrapped in a nativeNext of the Machine that gets it
// (guardHandler), as next is.
//
// Native code can also call a VM intrinsic (a Closure with Go, such as the
// stop func of iter.Pull, which gives the panic of the sequence to its
// caller) on a goroutine of its own: callClosure gives its panic to
// GoPanic, as callOutermost does, and also keeps ErrCanceled quiet (stop
// called by context.AfterFunc on the context of a canceled request).
//
// A native func can also panic by itself on such a goroutine, with no
// interpreted code on its stack: sync.OnceFunc calls its func once and then
// panics again with the saved value at every call. For a plugin, the func
// that sync.OnceFunc returns is wrapped (onceFunc): on a goroutine without
// VM state its panic goes to GoPanic.

import (
	"errors"
	"go/types"
	"net"
	"net/http"
	"net/http/fcgi"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// recovering runs f, whose caller recovers the panics of f: an outermost
// run inside f keeps its panic (see callOutermost).
func recovering(f func()) { f() }

// callerRecovers lists the functions of the VM that recover the panics of
// the outermost runs they make (see underCatcher). nativeRecovers holds the
// catchers of the standard library.
var callerRecovers = []string{
	goFuncName((*Machine).top),
	goFuncName(recovering),
}

// nativeCatcher is a function of the standard library that recovers the
// panics of some of the calls it makes (see nativeRecovers).
type nativeCatcher struct {
	name string
	// covers reports whether a panic of callee, the function that the
	// catcher calls (the frame just above it on the Go stack), reaches the
	// deferred recover of the catcher.
	covers func(callee string) bool
}

// calleeIn returns a covers func that accepts only the callees in names.
func calleeIn(names ...string) func(string) bool {
	return func(callee string) bool { return slices.Contains(names, callee) }
}

// calleeNotIn returns a covers func that accepts every callee except the
// deferred closures of catcher and names. The closures of a function are
// named catcher.func1, catcher.func2..., and those of a closure catcher.1,
// catcher.2... (catcher is then itself a closure, such as
// "net/http.(*timeoutHandler).ServeHTTP.func1"): every name that starts
// with catcher and a dot. Before, only the first form was left out.
func calleeNotIn(catcher string, names ...string) func(string) bool {
	return func(callee string) bool {
		return !strings.HasPrefix(callee, catcher+".") && !slices.Contains(names, callee)
	}
}

// nativeRecovers lists the functions of the standard library that start a
// goroutine, run a handler of the program on it, and recover its panic.
// When one of them is on the Go stack of an outermost run, and the call
// that leads to the outermost run is in the range of its recover, the panic
// is left to it, so the program behaves as with go run:
//   - net/http.TimeoutHandler runs the handler on a goroutine of its own,
//     recovers its panic and panics again on the goroutine of its
//     ServeHTTP (a plugin can recover it there, or the host gets it);
//   - the HTTP/1 and HTTP/2 servers (http.Serve, httptest) recover the
//     panic of a handler, abort the response (http.ErrAbortHandler) and
//     log other panics; the process does not end.
//
// A name on the stack is not enough: the recover of a function covers only
// the calls that it makes after its defer statement, and not the calls of
// its deferred funcs. (*conn).serve calls RemoteAddr, String and LocalAddr
// of the net.Conn before its defer, and Close in its deferred func: when
// the net.Conn is a value of the plugin, a panic there ends the process in
// Go. So each catcher says which of its callees it covers (covers), and
// underCatcher does not count a catcher whose deferred calls are running
// (a frame runtime.gopanic, runtime.Goexit or runtime.deferreturn between
// it and the outermost run). When a catcher does not count, the panic goes
// to GoPanic and afterPanic gives the result: this is always safe, the list
// only keeps the Go semantics where the standard library recovers.
//
// There is no rule without names: Go cannot tell whether a native frame has
// a deferred call that recovers, and recovering only on a list of known
// goroutines that do not recover (timers, finalizers...) would let any
// other native goroutine end the host. Plugins vendor their dependencies
// (interpreted, so their recovers work as in Go): only the standard library
// matters here.
//
// To add a catcher, add the name of the frame that holds the deferred
// recover, as runtime.Frame.Function gives it (a closure is
// "pkg.(*T).Method.func1"), the callees that its recover covers, and a case
// in TestPluginNativeRecover and a probe in TestCatcherProbes. The names
// come from the Go source and may change with a Go version: these tests
// fail then (they also run in the "next Go" job of CI).
var nativeRecovers = []nativeCatcher{
	// func1 defers its recover, then calls the handler.
	{"net/http.(*timeoutHandler).ServeHTTP.func1", calleeNotIn("net/http.(*timeoutHandler).ServeHTTP.func1")},
	// serve calls the conn methods before its defer: only the calls of the
	// request loop are covered.
	{"net/http.(*conn).serve", calleeIn(
		"net/http.serverHandler.ServeHTTP",
		"net/http.(*conn).readRequest",
		"net/http.(*response).finishRequest",
	)},
	// runHandler defers its recover, then calls the handler.
	{"net/http.(*http2serverConn).runHandler", calleeNotIn(
		"net/http.(*http2serverConn).runHandler",
		"net/http.(*http2serverConn).sendServeMsg",
	)},
	// The same function since Go 1.27, where the HTTP/2 server moved from
	// the bundle (h2_bundle.go) to net/http/internal/http2.
	{"net/http/internal/http2.(*serverConn).runHandler", calleeNotIn(
		"net/http/internal/http2.(*serverConn).runHandler",
		"net/http/internal/http2.(*serverConn).sendServeMsg",
	)},
}

// deferFrames are the runtime frames that run deferred calls: a native
// catcher below one of them is running its deferred calls, or has already
// run them. At a normal return, the function calls its deferred calls
// itself (runtime.deferreturn is not on the stack with Go 1.26): the
// covers func of each catcher leaves them out by name. TestCatcherProbes
// checks these frames and names on the Go release of the build.
var deferFrames = []string{"runtime.gopanic", "runtime.Goexit", "runtime.deferreturn"}

// The names of the frames above which underCatcher does not look (set by
// init: the functions refer to them).
var callOutermostName, nativeNextName, goOutermostName, nativeGoName string

func init() {
	callOutermostName = goFuncName((*ctx).callOutermost)
	nativeNextName = goFuncName((*nativeNext).ServeHTTP)
	goOutermostName = goFuncName((*Machine).goOutermost)
	nativeGoName = goFuncName((*Machine).nativeGo)
}

func goFuncName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// callOutermost runs fn on cx, the outermost run of goroutine g, for a
// Machine with a GoPanic. A panic that leaves fn goes to GoPanic, unless
// the caller recovers it (underCatcher); fn then returns what afterPanic
// gives and caught is true.
func (cx *ctx) callOutermost(g uintptr, fn *Func, args, env []Value) (rv Value, caught bool) {
	m := cx.m
	ok := false
	defer func() {
		if ok {
			return
		}
		// A run that Close refused or stopped follows the rules of
		// closedResult also under a native catcher (net/http recovers the
		// panics of a handler): a func() error of the plugin returns
		// ErrClosed to the handler of the host, as the comment of close.go
		// says. So when the machine is closed, the panic is recovered to look
		// at it, and a panic that is not ErrClosed goes on to the catcher.
		// A catcher of the VM (top, recovering) handles ErrClosed itself:
		// the panic goes on to it, as before (top aborts a ServeHTTP).
		catcher := underCatcher(callOutermostName)
		if catcher && (!m.closed.Load() || below(callOutermostName, callerRecovers)) {
			return
		}
		r := recover()
		if r == nil {
			return // runtime.Goexit: the goroutine ends, as in Go
		}
		if isClosed(r) {
			rv, caught = m.closedResult(fn), true
			return
		}
		if catcher {
			panic(r)
		}
		if !quietStop(r) {
			// A call from the host below (next, another plugin) whose
			// context is done stops with ErrCanceled, or gives
			// http.ErrAbortHandler: its deferred calls ran, and it is not a
			// panic of the plugin (quietStop, as in runGoroutine).
			m.GoPanic(r)
		}
		if fn != nil && fn.ssa != nil && isStringMethod(fn.ssa.Name(), fn.ssa.Signature) {
			rv, caught = m.fromNative(reflect.ValueOf(panicString), types.Typ[types.String]), true
			return
		}
		var res *types.Tuple
		if fn != nil && fn.ssa != nil {
			res = fn.ssa.Signature.Results()
		}
		rv, caught = m.afterPanic(res, callOutermostName), true
	}()
	cx.refuseClosed(g)
	if release := cx.countNativeCallback(g); release != nil {
		defer release()
	}
	rv = cx.callIn(g, nil, fn, args, env)
	ok = true
	return rv, false
}

// refuseClosed panics ErrClosed before cx, the outermost run of goroutine
// g, starts when its machine is closed: a func value or a method of the
// plugin that the host (or native code) calls after Close does not run, as
// Plugin.Call returns ErrClosed. The run is forgotten as callIn would.
func (cx *ctx) refuseClosed(g uintptr) {
	if !cx.m.closed.Load() {
		return
	}
	leaveG(g, cx.gs)
	cx.owned.close()
	panic(ErrClosed)
}

// closedResult is what an outermost run of fn that Close stopped or refused
// gives to its caller (callOutermost). It is not a panic of the plugin: it
// does not go to GoPanic, and it never ends the goroutine of the caller
// with runtime.Goexit, which would end a goroutine of the host without an
// error:
//   - no results: the run returns (a time.AfterFunc callback that fires
//     after Close);
//   - the last result is an error: zero values and ErrClosed, as
//     Plugin.Call returns it;
//   - a nativeNext is below: errAbortCallback (see afterPanic);
//   - on the finalizer or a cleanup goroutine: zero results;
//   - else the panic ErrClosed goes on to the caller.
func (m *Machine) closedResult(fn *Func) Value {
	var res *types.Tuple
	if fn != nil && fn.ssa != nil {
		res = fn.ssa.Signature.Results()
	}
	if res == nil || res.Len() == 0 {
		return Value{}
	}
	if last := res.At(res.Len() - 1).Type(); types.Identical(last, errorTypeV) {
		out := make([]Value, res.Len())
		for i := range res.Len() - 1 {
			out[i] = m.zero(res.At(i).Type())
		}
		out[len(out)-1] = m.fromNative(reflect.ValueOf(&ErrClosed).Elem(), last)
		if len(out) == 1 {
			return out[0]
		}
		return Value{R: out}
	}
	if below(callOutermostName, []string{nativeNextName}) {
		panic(errAbortCallback)
	}
	if below(callOutermostName, noGoexit) {
		return m.zeroTuple(res)
	}
	panic(ErrClosed)
}

// panicString is what native code gets from a String or Error method of
// the program whose panic went to GoPanic (callOutermost). In the style of
// fmt, which prints "%!v(PANIC=String method: ...)" for such a panic.
const panicString = "%!(PANIC=the method panicked, the panic went to GoPanic)"

// isStringMethod reports whether the function name with the signature sig
// is a String or Error method (or its method value: ssa names it
// String$bound) that returns a string. Native code calls such methods to
// print a value (fmt, logs, net/http with a net.Addr), and a string is
// always safe to give: callOutermost gives panicString after a panic,
// instead of ending the goroutine (afterPanic).
func isStringMethod(name string, sig *types.Signature) bool {
	name, suffix, _ := strings.Cut(name, "$")
	if sig.Recv() == nil && suffix != "bound" || name != "String" && name != "Error" {
		return false
	}
	if sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return false
	}
	b, ok := sig.Results().At(0).Type().(*types.Basic)
	return ok && b.Kind() == types.String
}

// zeroTuple returns zero values for the types of res, as a call returns
// them.
func (m *Machine) zeroTuple(res *types.Tuple) Value {
	switch res.Len() {
	case 0:
		return Value{}
	case 1:
		return m.zero(res.At(0).Type())
	}
	out := make([]Value, res.Len())
	for i := range out {
		out[i] = m.zero(res.At(i).Type())
	}
	return Value{R: out}
}

// callClosure calls the func value cl for native code (a Go func made by
// toNativeW), with the signature sig. A VM intrinsic (cl.Go, cl.Fn is nil)
// runs as Go code: on a goroutine without VM state, for a Machine with a
// GoPanic, its panic goes to GoPanic (goOutermost).
func (m *Machine) callClosure(cl *Closure, args []Value, sig *types.Signature) Value {
	if cl.Go == nil {
		return m.Call(cl.Fn, args, cl.Env)
	}
	if m.GoPanic == nil || lookupG(getg()) != nil {
		return cl.Go(args)
	}
	return m.goOutermost(cl, args, sig)
}

// goOutermost runs the VM intrinsic cl on a goroutine without VM state. A
// panic that leaves it goes to GoPanic, unless the caller recovers it
// (underCatcher), and the results are the ones of afterPanic.
func (m *Machine) goOutermost(cl *Closure, args []Value, sig *types.Signature) (rv Value) {
	ok := false
	defer func() {
		if ok || underCatcher(goOutermostName) {
			return
		}
		r := recover()
		if r == nil {
			return // runtime.Goexit: the goroutine ends, as in Go
		}
		if !quietStop(r) {
			// The stop func of iter.Pull gives the panic of the sequence
			// again. A sequence on a canceled account stops with
			// ErrCanceled: not a panic of the plugin (as in callOutermost).
			m.GoPanic(r)
		}
		rv = m.afterPanic(sig.Results(), goOutermostName)
	}()
	rv = cl.Go(args)
	ok = true
	return rv
}

// The native handler types that panic again on the goroutine of their
// ServeHTTP (see the top of this file).
var (
	topHandlerType     = reflect.TypeFor[topHandler]()
	timeoutHandlerType = reflect.TypeOf(http.TimeoutHandler(http.NotFoundHandler(), time.Second, ""))
)

// guardHandler returns e, the dynamic value of a native interface that
// comes into the VM of m, wrapped in a nativeNext when it is a topHandler
// or the handler of http.TimeoutHandler and m has a GoPanic.
func (m *Machine) guardHandler(e reflect.Value) reflect.Value {
	if m.GoPanic == nil || (e.Type() != topHandlerType && e.Type() != timeoutHandlerType) || !e.CanInterface() {
		return e
	}
	return reflect.ValueOf(&nativeNext{m, e.Interface().(http.Handler)})
}

// underCatcher reports whether a catcher is on the Go stack of the current
// goroutine, below the frame of the function anchor: a function of
// callerRecovers, or a function of nativeRecovers whose recover covers the
// call (see nativeRecovers).
// It is called by the deferred call of anchor while the panic runs, so the
// stack still holds the frames of the panic above anchor (they can hold
// top, when the panic comes from a call of another plugin): only the
// callers of anchor, below it, are looked at. Panics of outermost runs are
// rare, so the walk does not cost anything to the calls that do not panic.
func underCatcher(anchor string) bool {
	inDefer := false
	callee := anchor
	for _, f := range callersBelow(anchor) {
		if slices.Contains(callerRecovers, f) {
			return true
		}
		if slices.Contains(deferFrames, f) {
			inDefer = true
		}
		if !inDefer {
			for _, c := range nativeRecovers {
				if c.name == f && c.covers(callee) {
					return true
				}
			}
		}
		callee = f
	}
	return false
}

// below reports whether a function of names is on the Go stack of the
// current goroutine, below the frame of the function anchor.
func below(anchor string, names []string) bool {
	return slices.ContainsFunc(callersBelow(anchor), func(f string) bool { return slices.Contains(names, f) })
}

// callersBelow returns the names of the functions on the Go stack of the
// current goroutine below the frame of the function anchor, the callers
// first (the caller of anchor, then its caller...).
func callersBelow(anchor string) []string {
	pcs := make([]uintptr, 64)
	for {
		n := runtime.Callers(0, pcs)
		if n < len(pcs) {
			pcs = pcs[:n]
			break
		}
		pcs = make([]uintptr, 2*len(pcs))
	}
	frames := runtime.CallersFrames(pcs)
	var names []string
	under := false
	for {
		f, more := frames.Next()
		if under {
			names = append(names, f.Function)
		}
		if f.Function == anchor {
			under = true
		}
		if !more {
			return names
		}
	}
}

// nativeNext is the native handler that Plugin.Call and Object.Call give to the plugin m,
// and the wrapper of the handlers that guardHandler gives to m.
// Called by interpreted code, on its goroutine, it is next: a panic goes up
// to that code, as in Go. Called on a goroutine without interpreted code
// (native code that the plugin started), a panic that leaves next goes to
// the GoPanic of m, unless a catcher is below (see the top of this file),
// and ServeHTTP returns.
type nativeNext struct {
	m    *Machine
	next http.Handler
}

func (n *nativeNext) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if lookupG(getg()) != nil {
		n.next.ServeHTTP(w, r)
		return
	}
	ok := false
	defer func() {
		if ok || underCatcher(nativeNextName) {
			return
		}
		p := recover()
		if p == nil {
			return // runtime.Goexit: the goroutine ends, as in Go
		}
		if p == errAbortCallback {
			return // a callback below gave its panic to GoPanic (afterPanic)
		}
		n.m.GoPanic(p)
	}()
	n.next.ServeHTTP(w, r)
	ok = true
}

// errCallbackPanic is the error that a callback with an error result gives
// to the native code that called it, when its panic went to GoPanic
// (afterPanic).
var errCallbackPanic = errors.New("vm: the callback panicked (the panic went to GoPanic)")

// errAbortCallback is the panic of afterPanic that ends the native frames
// up to a nativeNext, which returns.
var errAbortCallback = errors.New("vm: the callback panicked (abort the handler)")

// noGoexit lists the runtime goroutines that run callbacks of the program
// and that must never end: the finalizer goroutine (its results are
// ignored) and the cleanup goroutines (runtime.AddCleanup, no results).
var noGoexit = []string{
	"runtime.runFinalizers",
	"runtime.runfinq", // before Go 1.26
	"runtime.runCleanups",
}

// afterPanic decides what an outermost run (anchor: callOutermost or
// goOutermost) whose panic went to GoPanic gives to the native code that
// called it on its goroutine. res are the result types of the callback.
// Zero results are not safe in general: http.FileServer uses the nil
// http.File of an Open that returned (nil, nil), io.ReadFull calls a Read
// that returns (0, nil) for ever. So:
//   - no results: the callback returns;
//   - a String or Error method: panicString (callOutermost, before
//     afterPanic);
//   - the last result is an error: zero values and errCallbackPanic, as a
//     failed call returns them (FileServer answers 500, ReverseProxy 502,
//     io.ReadFull and http.Transport return the error);
//   - a nativeNext is below (the handler that fcgi.Serve runs, next): the
//     native frames up to it end with the panic errAbortCallback, and the
//     handler returns (fcgi then ends the response and closes the
//     connection);
//   - log/slog.Value.Resolve is below (the host logs a value of the plugin
//     whose LogValue panicked or ran out of budget): the panic
//     errAbortCallback goes up to Resolve, which recovers it and logs
//     "LogValue panicked", as with go run;
//   - on the finalizer or a cleanup goroutine (noGoexit): zero results,
//     which the runtime ignores;
//   - else runtime.Goexit: the goroutine ends, its deferred calls run, and
//     native code never sees a result it does not expect. A lock that a
//     native frame holds without a deferred unlock stays held; in Go the
//     process would end.
func (m *Machine) afterPanic(res *types.Tuple, anchor string) Value {
	if res == nil || res.Len() == 0 {
		return Value{}
	}
	if last := res.At(res.Len() - 1).Type(); types.Identical(last, errorTypeV) {
		out := make([]Value, res.Len())
		for i := range res.Len() - 1 {
			out[i] = m.zero(res.At(i).Type())
		}
		out[len(out)-1] = m.fromNative(reflect.ValueOf(&errCallbackPanic).Elem(), last)
		if len(out) == 1 {
			return out[0]
		}
		return Value{R: out}
	}
	if below(anchor, []string{nativeNextName, slogResolveName}) {
		panic(errAbortCallback)
	}
	if below(anchor, noGoexit) {
		return m.zeroTuple(res)
	}
	runtime.Goexit()
	return Value{}
}

// slogResolveName is the frame of log/slog.Value.Resolve, which recovers
// the panic of a LogValue method (afterPanic). TestLogValuerPanic checks
// the name on the Go release of the build.
const slogResolveName = "log/slog.Value.Resolve"

// errorTypeV is the type error of go/types.
var errorTypeV = types.Universe.Lookup("error").Type()

// guardNativeGo installs the overrides of the native functions that run
// code of the plugin m on goroutines of their own and that need a guard
// there (see the top of this file).
func (m *Machine) guardNativeGo() {
	m.addOverride("sync.OnceFunc", m.onceFunc)
	m.addOverride("net/http/fcgi.Serve", m.fcgiServe)
}

// onceFunc is sync.OnceFunc for a plugin. The func that sync.OnceFunc
// returns panics again with the saved value at every call, with no
// interpreted code on its stack: called by native code on a goroutine of
// its own (time.AfterFunc(d, f)), that panic would end the host. Here such
// a call gives it to GoPanic (nativeGo); on a goroutine that runs
// interpreted code, the panic goes up to it, as in Go.
func (m *Machine) onceFunc(f func()) func() {
	g := sync.OnceFunc(f)
	return func() {
		if lookupG(getg()) != nil {
			g()
			return
		}
		m.nativeGo(g)
	}
}

// nativeGo runs the native func f on a goroutine without VM state: a panic
// that leaves f goes to GoPanic, unless the caller recovers it
// (underCatcher).
func (m *Machine) nativeGo(f func()) {
	ok := false
	defer func() {
		if ok || underCatcher(nativeGoName) {
			return
		}
		r := recover()
		if r == nil {
			return // runtime.Goexit: the goroutine ends, as in Go
		}
		m.GoPanic(r)
	}()
	f()
	ok = true
}

// fcgiServe is fcgi.Serve for a plugin. fcgi runs the handler on a
// goroutine of its own and does not recover its panics: the handler (or
// http.DefaultServeMux for nil) runs inside a nativeNext of m, so a panic
// goes to GoPanic, and a callback below that panicked ends the handler
// (afterPanic): fcgi then closes the connection.
func (m *Machine) fcgiServe(l net.Listener, h http.Handler) error {
	if h == nil {
		h = http.DefaultServeMux
	}
	return fcgi.Serve(l, &nativeNext{m, h})
}
