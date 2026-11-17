package vm

// Instances: the goroutines of one New call (Machine.NewInstance).
//
// Traefik loads a plugin once, at startup, and calls its New function at
// each reload of the dynamic configuration. Many plugins
// start goroutines in New that never end: a cache janitor with a
// time.Ticker, a refresh loop (darkweak/souin starts 2 per New). Under
// yaegi they run for ever. Before instances, they also counted in the
// MaxGoroutines of the machine for ever, so with the cap of 1000 of
// internal/pluginhost every New failed after about 500 reloads.
//
// An Instance groups the goroutines that one call from the host starts:
// the host runs the call with a context from Instance.Context, and every
// goroutine that the call starts (go statements, iter.Pull sequences, the
// callbacks that native code runs on a goroutine of its own, such as
// time.AfterFunc, also when a background goroutine of the instance gives
// the callback after the call returned), the finalizers and cleanups that
// it sets (runtime.SetFinalizer, runtime.AddCleanup), and every goroutine
// that these start in turn, belongs to the instance, also after the call
// returned. Then:
//   - MaxGoroutines bounds the live goroutines of each instance, and not
//     the ones of the machine: the goroutines that the old instances left
//     do not make a new New fail. The goroutines that belong to no instance
//     (a request that ServeHTTP serves, a call without an instance) keep
//     the cap of the machine, together;
//   - Instance.Goroutines reports the live goroutines of the instance, so
//     the host can see a plugin that leaks goroutines at each reload;
//   - Instance.Close stops the goroutines of the instance only, as
//     Machine.Close stops all of them (close.go), when the host knows that
//     the instance is gone. The other instances, the calls from the host
//     and the package variables are not touched.
//
// A request that a handler of the instance serves is not part of the
// instance: the host calls ServeHTTP with the context of the request, and
// its goroutines belong to no instance. Machine.Goroutines counts the
// goroutines of all the instances too.

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// Instance groups the goroutines that the calls from the host made with
// its context (Context) start: see the comment at the top of instance.go.
type Instance struct {
	m      *Machine
	n      atomic.Int64 // live goroutines
	closed atomic.Bool
	mu     sync.Mutex
	done   chan struct{} // made at the first wait, closed by Close or Machine.Close (guarded by mu)
	shut   bool          // done is closed (guarded by mu)
	// bg is a closed account of i: the origin of the callbacks of native
	// code that a background goroutine of i starts (originAcct).
	bg *account
}

// instKey is the key of the Instance in a context.Context.
type instKey struct{}

// NewInstance returns a new Instance of p: see Machine.NewInstance.
func (p *Plugin) NewInstance() *Instance { return p.M.NewInstance() }

// NewInstance returns a new Instance of m. A host that calls New at each
// reload of its configuration runs each New call with the context of a new
// Instance (Instance.Context). The host may drop the Instance: its
// goroutines keep it alive.
func (m *Machine) NewInstance() *Instance {
	i := &Instance{m: m}
	i.bg = &account{inst: i}
	i.bg.state.Store(acctClosed)
	wp := weak.Make(i)
	m.instMu.Lock()
	if m.insts == nil {
		m.insts = map[weak.Pointer[Instance]]struct{}{}
	}
	m.insts[wp] = struct{}{}
	m.instMu.Unlock()
	// Forget the instance when nothing uses it any more: no goroutine, no
	// host, no value of the plugin that native code keeps (originAcct).
	runtime.AddCleanup(i, func(wp weak.Pointer[Instance]) {
		m.instMu.Lock()
		delete(m.insts, wp)
		m.instMu.Unlock()
	}, wp)
	m.instUsed.Store(true)
	return i
}

// Context returns a context with parent's values and deadline that also
// carries i: a call from the host (Plugin.Call, Object.Call) with this
// context runs in i. The plugin cannot see i in the context.
func (i *Instance) Context(parent context.Context) context.Context {
	return context.WithValue(parent, instKey{}, i)
}

// Goroutines returns the number of live goroutines of i.
func (i *Instance) Goroutines() int { return int(i.n.Load()) }

// Closed reports whether Close was called.
func (i *Instance) Closed() bool { return i.closed.Load() }

// Close stops the goroutines of i with ErrClosed, as Machine.Close stops
// the goroutines of the machine (close.go): at their next blocking point,
// or at their next refill of steps (a loop without a blocking point is
// only stopped when the machine has a Budget or a BackgroundRate). A go
// statement in them panics ErrClosed. Close does not wait for them:
// Goroutines reports the ones that did not stop yet. A call from the host
// that runs in i (New) stops in the same way. Close is idempotent.
//
// Close does not touch the handlers that i made: a request that they serve
// later is not part of i (see instance.go). Use it only when the host
// knows that i is gone: a handler of i may need a goroutine of i (a
// channel that a background goroutine serves) and wait for ever without
// it.
//
// A goroutine belongs to the instance that started it, also when it serves
// all the instances: a shared service that the first New starts once
// (sync.Once, a package cache) belongs to that first instance. Close on it
// stops the service for the later instances too, and host.Leaks reports
// it after the first reload.
func (i *Instance) Close() {
	i.closed.Store(true)
	i.closeWaits()
}

// closeWaits closes the channel of the waits of i.
func (i *Instance) closeWaits() {
	i.mu.Lock()
	if i.done != nil && !i.shut {
		close(i.done)
		i.shut = true
	}
	i.mu.Unlock()
}

// stopChan returns the channel that is closed when i or its machine
// closes. Most instances never wait, so it is made at the first wait.
func (i *Instance) stopChan() chan struct{} {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.done == nil {
		i.done = make(chan struct{})
		// Machine.Close sets closed before it walks the instances: either
		// it sees this channel, or this sees closed.
		if i.closed.Load() || i.m.closed.Load() {
			close(i.done)
			i.shut = true
		}
	}
	return i.done
}

// closeInstances closes the channels of the waits of all the instances of
// m (Machine.Close, after closed is set).
func (m *Machine) closeInstances() {
	m.instMu.Lock()
	defer m.instMu.Unlock()
	for wp := range m.insts {
		if i := wp.Value(); i != nil {
			i.closeWaits()
		}
	}
}

// instanceOf returns the Instance of m that ctx carries, or nil.
func (m *Machine) instanceOf(ctx context.Context) *Instance {
	if ctx == nil || !m.instUsed.Load() {
		return nil
	}
	if i, ok := ctx.Value(instKey{}).(*Instance); ok && i.m == m {
		return i
	}
	return nil
}

// stopped reports whether cx must stop with ErrClosed: its machine or its
// instance closed.
func (cx *ctx) stopped() bool {
	return cx.m.closed.Load() || cx.inst != nil && cx.inst.closed.Load()
}

// curInstance returns the instance of the run of m on this goroutine, or
// nil. A finalizer or a cleanup that the run sets keeps it, and runs in it
// (instGoCtx).
func (m *Machine) curInstance() *Instance {
	if !m.instUsed.Load() {
		return nil
	}
	gs := lookupG(getg())
	if gs == nil || gs.cur == nil || gs.cur.m != m {
		return nil
	}
	return gs.cur.inst
}

// instGoCtx returns a new background ctx (runGoroutine) in the instance
// inst (nil: none).
func (m *Machine) instGoCtx(inst *Instance) *ctx {
	sub := m.goCtx(nil)
	sub.inst = inst
	return sub
}
