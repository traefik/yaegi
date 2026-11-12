package vm

// Closing a machine (Plugin.Close, Machine.Close).
//
// A host that loads a plugin once and builds many instances of it (Traefik
// calls New at each reload of its dynamic configuration) cannot stop the
// goroutines that an instance started: a cache janitor with a time.Ticker,
// a refresh loop. They run for ever, and they count in MaxGoroutines for
// the whole life of the machine, so a plugin that starts 2 of them in New
// fails at every New after about 500 reloads with MaxGoroutines 1000. Go
// has no way to stop the goroutines of one instance: the host releases a
// plugin as a whole, by loading it again and closing the old Plugin.
//
// Close stops every goroutine of the machine, as a canceled call stops
// (ErrCanceled, budget.go), with ErrClosed:
//   - at its next refill of steps: at most maxChunk steps later in a call
//     from the host, and for a background goroutine when the machine has a
//     Budget or a BackgroundRate (a background goroutine of a machine with
//     neither has no step limit, so a loop without a blocking point is
//     only stopped at its next blocking point);
//   - at once when it waits on a channel, in a select, in time.Sleep or in
//     an iter.Pull sequence, also in the background.
//
// The deferred calls of the stopped frames run, with the grace of a
// cancellation (cancelGrace steps), so a deferred Unlock still unlocks;
// recover cannot stop ErrClosed. A wait in a deferred call panics
// ErrClosed again. Calls from the host that run when Close is called stop
// too: Plugin.Call and Object.Call return an error that wraps ErrClosed, a
// ServeHTTP aborts the response (http.ErrAbortHandler), and a func value or
// a method of the plugin that the host calls returns ErrClosed as its error
// result, or panics ErrClosed when it has no error result (closedResult in
// nativego.go: never runtime.Goexit on a goroutine of the host), also when
// the host recovers the panics of the call (an HTTP handler). After
// Close, a call from the host returns ErrClosed at once (a func value or a
// method in the same way, before it runs), a ServeHTTP answers 503, a go
// statement panics ErrClosed, and a callback that native code runs later
// (time.AfterFunc) returns at once without going to GoPanic.
//
// CloseContext is Close after a drain: it refuses the new calls from the
// host at once (as Close does), lets the calls that run go on until they
// all returned or ctx is done, and then closes. The background goroutines
// run until then. A call from the host nested in a call that runs (the same
// plugin twice in a chain of middlewares: the first instance calls the
// second one through next) is not new: it is still accepted while the
// machine drains. A call that passed the check just before the drain
// started counts as running (Machine.top). The drain also waits for the
// outermost runs of func values and methods of the plugin that the host or
// native code calls (callCtx): it does not refuse them, so a callback of
// time.AfterFunc still runs while the machine drains.
//
// What Close does not stop: native code that blocks (a network read, a
// mutex wait, WaitGroup.Wait), and native resources that the plugin
// opened (a listener of an http.Server, a file). A time.Ticker or a
// time.Timer that no stopped goroutine uses any more is collected by the Go
// runtime (Go 1.23 and later); a time.AfterFunc still fires, and its
// callback stops at once.

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrClosed is the error of a call into a closed machine, and the panic
// that stops its goroutines (Plugin.Close).
var ErrClosed = errors.New("vm: plugin closed")

// errClosedHard is ErrClosed once the deferred calls have used their grace:
// no deferred call runs any more (isFatal).
var errClosedHard error = closedHard{}

type closedHard struct{}

func (closedHard) Error() string        { return ErrClosed.Error() }
func (closedHard) Is(target error) bool { return target == ErrClosed }

// closedGrace marks, in gstate.graceAcct, the grace of a goroutine stopped
// by Close (an account of no call).
var closedGrace = &account{}

// isClosed reports whether r stops a goroutine because its machine closed.
func isClosed(r any) bool { return r == ErrClosed || r == errClosedHard }

// Close stops all the goroutines of m, and makes later calls into m fail
// with ErrClosed (see the comment at the top of close.go). It does not wait
// for the goroutines: Goroutines reports the ones that did not stop yet.
// Close is idempotent and safe to call from any goroutine, also while calls
// run, and also from inside a call (a host func that the plugin calls),
// unlike CloseContext with context.Background().
func (m *Machine) Close() {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if m.closed.Load() {
		return
	}
	m.draining.Store(true)
	m.closed.Store(true)
	if c := m.done.Load(); c != nil {
		close(*c)
	}
	if q := m.mem; q != nil {
		// Nothing of the plugin may allocate any more: forget the counted
		// objects, so that the quota does not keep their weak pointers.
		q.mu.Lock()
		q.objs, q.used = nil, 0
		q.mu.Unlock()
	}
}

// drainPoll is the period at which CloseContext checks the calls that run.
const drainPoll = 5 * time.Millisecond

// CloseContext refuses the new calls from the host, waits until the calls
// that run returned or ctx is done, and then calls Close (see the comment
// at the top of close.go). It returns ctx.Err() when calls still ran at
// the end of the drain (Close stopped them), and nil otherwise.
//
// CloseContext called from inside a call that runs (a host func that the
// plugin calls) counts that call as running: it waits for itself until ctx
// is done, and so for ever with context.Background(). Use Close there, or a
// ctx with a deadline.
func (m *Machine) CloseContext(ctx context.Context) error {
	m.draining.Store(true)
	defer m.Close()
	if m.inflight.Load() == 0 {
		return nil
	}
	t := time.NewTicker(drainPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if m.inflight.Load() == 0 {
				return nil
			}
			return ctx.Err()
		case <-t.C:
			if m.inflight.Load() == 0 {
				return nil
			}
		}
	}
}

// refused reports whether a call from the host must fail with ErrClosed:
// the machine is closed, or it drains and the call is new (no interpreted
// code runs on this goroutine, so it is not nested in a call that runs).
func (m *Machine) refused() bool {
	if m.closed.Load() {
		return true
	}
	return m.draining.Load() && lookupG(getg()) == nil
}

// Closed reports whether Close was called.
func (m *Machine) Closed() bool { return m.closed.Load() }

// closeChan returns the channel that Close closes. Most machines never
// wait, so it is made at the first wait.
func (m *Machine) closeChan() chan struct{} {
	if c := m.done.Load(); c != nil {
		return *c
	}
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if c := m.done.Load(); c != nil {
		return *c
	}
	c := make(chan struct{})
	if m.closed.Load() {
		close(c)
	}
	m.done.Store(&c)
	return c
}

// stopChans returns the channels whose close ends a wait of cx: the stop
// channel of its account (nil without an account: a nil channel is never
// ready) and the channel of Close.
func (cx *ctx) stopChans() (acct, closed chan struct{}) {
	if a := cx.acct; a != nil {
		acct = a.stopChan()
	}
	return acct, cx.m.closeChan()
}

// closeStop stops cx with ErrClosed when its machine is closed (wait: cx
// waited and cannot go on), as cancel does for a canceled account.
func (cx *ctx) closeStop(wait bool) {
	gs := cx.gs
	if gs.graceAcct != closedGrace {
		gs.graceAcct, gs.graceLeft = closedGrace, cancelGrace
		cx.grace(errClosedHard)
		panic(ErrClosed)
	}
	cx.grace(errClosedHard)
	if wait {
		panic(ErrClosed)
	}
}

// serveClosed answers a request to a handler of a closed machine.
func serveClosed(w http.ResponseWriter) {
	http.Error(w, ErrClosed.Error(), http.StatusServiceUnavailable)
}

// Close stops the plugin at once: see Machine.Close. It always returns
// nil.
func (p *Plugin) Close() error {
	p.M.Close()
	return nil
}

// CloseContext stops the plugin after a drain: see Machine.CloseContext.
// Called from inside a call of the plugin (a host func that the plugin
// calls), it waits for that call too: with context.Background(), for ever.
//
// A host that builds a new instance of a plugin at each configuration
// reload (Traefik) releases the old instances this way: at the reload, it
// loads the plugin again (LoadCached reuses the program of a first load),
// calls New on the new Plugin, and makes the new handlers live; then it
// calls CloseContext on the old Plugin with a deadline (docs/RELOAD.md), so
// the requests that the old handlers serve end, and every goroutine of the
// old instances stops.
func (p *Plugin) CloseContext(ctx context.Context) error {
	return p.M.CloseContext(ctx)
}
