package vm

// iter.Pull and iter.Pull2 for interpreted sequences. The standard library
// implementation switches between runtime coroutines (linknamed to the
// runtime), which the VM cannot interpret: the sequence runs in a goroutine
// instead, handing each value over a channel. The semantics a program can
// observe are kept: values come one per next call, stop makes the pending
// yield return false and waits for the sequence to return, next after the
// end or after stop returns zero values, and a panic in the sequence is
// raised again by the next or stop call that was waiting for it. The
// goroutine shares the budget of the caller of the first next and counts in
// MaxGoroutines, as a go statement (budget.go). It also continues the depth
// and the nested runs of that caller (nest.go): the sequence runs while next
// waits, as a call, and a recursion through iter.Pull must reach MaxDepth
// and MaxNest. The hand-offs between next or stop and the sequence are
// cancellation points, as channel operations.

import (
	"go/types"
	"sync"

	"golang.org/x/tools/go/ssa"
)

type pullMsg struct {
	vals []Value // yielded values (nil: the sequence returned)
	pnc  any     // panic of the sequence
}

func (m *Machine) iterPull(f *ssa.Function, args []int32) func([]Value) Value {
	zero := m.pullZeros(f)
	return func(regs []Value) Value {
		seq := regs[args[0]]
		var (
			mu      sync.Mutex
			started bool
			done    bool
			req     = make(chan bool)
			resp    = make(chan pullMsg)
			// quit is closed when one side stops waiting for the other
			// because its call was canceled or ran out of budget (abandon):
			// the other side must not wait for ever.
			quit     = make(chan struct{})
			quitOnce sync.Once
		)
		abandon := func() { quitOnce.Do(func() { close(quit) }) }
		// run runs the sequence on its own goroutine, whose ctx sub
		// shares the account of the caller of the first next and counts
		// in MaxGoroutines (budget.go).
		run := func(sub *ctx) {
			g := getg()
			gs, _ := enterG(g, sub)
			sub.gs, gs.cur = gs, sub
			defer func() {
				p := recover()
				sub.giveBack()
				leaveG(g, gs)
				m.goRelease()
				msg := pullMsg{pnc: p}
				select {
				case resp <- msg:
				case <-quit: // nobody waits for the end
				}
			}()
			if !pullRecv(sub, req, quit, abandon) {
				return
			}
			yield := &Closure{Go: func(a []Value) Value {
				// The goroutine of the sequence may run other code
				// between two yields: the current ctx, not sub.
				cx := sub
				if gs := lookupG(getg()); gs != nil && gs.cur != nil {
					cx = gs.cur
				}
				if !pullSend(cx, resp, pullMsg{vals: append([]Value(nil), a...)}, quit, abandon) {
					return Value{N: 0}
				}
				return Value{N: b2i(pullRecv(cx, req, quit, abandon))}
			}}
			// The panics of seq go to the caller of next or stop, not to
			// GoPanic (nativego.go).
			recovering(func() { m.callValue(seq, []Value{{R: yield}}) })
		}
		// caller is the ctx of the goroutine that calls next or stop (nil:
		// a goroutine without interpreted code).
		caller := func() *ctx {
			if gs := lookupG(getg()); gs != nil {
				return gs.cur
			}
			return nil
		}
		// wait waits for the next message of the sequence. A wait is a
		// cancellation point, as a channel receive: when the call of cx
		// is canceled or out of budget, the sequence is abandoned and cx
		// stops.
		wait := func(cx *ctx) pullMsg {
			var msg pullMsg // after quit: the end of the sequence
			if cx == nil {
				select {
				case msg = <-resp:
				case <-quit:
				}
			} else {
				as, ms := cx.stopChans()
				select {
				case msg = <-resp:
				case <-quit:
				case <-as:
					done = true
					abandon()
					cx.halted()
				case <-ms:
					done = true
					abandon()
					cx.halted()
				}
				cx.blocked()
			}
			if msg.vals == nil {
				done = true
			}
			if msg.pnc != nil {
				panic(msg.pnc)
			}
			return msg
		}
		// send sends a request to the sequence. It reports false when the
		// sequence was abandoned (it stopped waiting).
		send := func(cx *ctx, v bool) bool {
			if !pullSend(cx, req, v, quit, abandon) {
				done = true
				return false
			}
			return true
		}
		next := func([]Value) Value {
			mu.Lock()
			defer mu.Unlock()
			if done {
				return Value{R: append(append([]Value(nil), zero...), Value{N: 0})}
			}
			cx := caller()
			if !started {
				// The sequence runs while next waits, as a call: its
				// goroutine continues the depth and the nested runs of
				// the caller of the first next (nest.go). Without this, a
				// recursion through iter.Pull never reached MaxDepth nor
				// MaxNest.
				sub := m.goCtx(cx)
				if cx != nil && cx.m == m {
					sub.depth = cx.depth + len(cx.frames) + 1
					sub.nest = cx.nest + cx.aotNest + 1
					sub.maxFrames = m.MaxDepth - sub.depth
					sub.regsBelow = cx.belowVals()
					sub.fitStack()
					if sub.nest > m.MaxNest || sub.maxFrames <= 0 ||
						m.MaxStack > 0 && int64(sub.regsBelow)*valueSize >= m.MaxStack {
						cx.overflow()
					}
				}
				m.goReserve()
				started = true
				go run(sub)
			}
			if !send(cx, true) {
				return Value{R: append(append([]Value(nil), zero...), Value{N: 0})}
			}
			msg := wait(cx)
			if msg.vals == nil {
				return Value{R: append(append([]Value(nil), zero...), Value{N: 0})}
			}
			return Value{R: append(msg.vals, Value{N: 1})}
		}
		stop := func([]Value) Value {
			mu.Lock()
			defer mu.Unlock()
			if done {
				return Value{}
			}
			if !started {
				done = true
				return Value{}
			}
			cx := caller()
			if !send(cx, false) {
				return Value{}
			}
			for !done {
				msg := wait(cx)
				if msg.vals != nil && !send(cx, false) { // the sequence ignored false: keep refusing
					return Value{}
				}
			}
			return Value{}
		}
		return Value{R: []Value{{R: &Closure{Go: next}}, {R: &Closure{Go: stop}}}}
	}
}

// pullSend sends v on c for cx (nil: no account). It reports false when
// the other side abandoned (quit is closed). When the call of cx is
// canceled or out of budget, it abandons and cx stops.
func pullSend[T any](cx *ctx, c chan T, v T, quit chan struct{}, abandon func()) bool {
	if cx == nil {
		select {
		case c <- v:
			return true
		case <-quit:
			return false
		}
	}
	as, ms := cx.stopChans()
	select {
	case c <- v:
	case <-quit:
		return false
	case <-as:
		abandon()
		cx.halted()
	case <-ms:
		abandon()
		cx.halted()
	}
	cx.blocked()
	return true
}

// pullRecv receives a request on c for cx, as pullSend. It returns false
// when the other side abandoned.
func pullRecv(cx *ctx, c chan bool, quit chan struct{}, abandon func()) bool {
	if cx == nil {
		select {
		case v := <-c:
			return v
		case <-quit:
			return false
		}
	}
	as, ms := cx.stopChans()
	var v bool
	select {
	case v = <-c:
	case <-quit:
		return false
	case <-as:
		abandon()
		cx.halted()
	case <-ms:
		abandon()
		cx.halted()
	}
	cx.blocked()
	return v
}

// pullZeros returns the zero values next returns after the end: one per
// result of next but the final bool.
func (m *Machine) pullZeros(f *ssa.Function) []Value {
	res := f.Signature.Results().At(0).Type().Underlying().(*types.Signature).Results()
	zs := make([]Value, res.Len()-1)
	for i := range zs {
		zs[i] = m.zero(res.At(i).Type())
	}
	return zs
}
