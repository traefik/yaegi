package interp

//go:generate go run ../internal/cmd/genop/genop.go

import (
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"reflect"
	"regexp"
	"strings"
	"unsafe"
)

// bltn type defines functions which run at CFG execution.
type bltn func(f *frame) bltn

// bltnGenerator type defines a builtin generator function.
type bltnGenerator func(n *node)

type callOwnerBinder struct {
	f        *frame
	cancel   <-chan struct{}
	bindArgs bool
}

// maxBoundWrapperCache bounds the per-group memoized host-bound wrappers;
// activations with distinct cancel channels would otherwise accumulate one
// entry per run on groups that survive sweeps.
const maxBoundWrapperCache = 1024

func newCallOwnerBinder(f *frame, cancel <-chan struct{}) *callOwnerBinder {
	return &callOwnerBinder{f: f, cancel: cancel, bindArgs: true}
}

func (b *callOwnerBinder) value(v reflect.Value) reflect.Value {
	bound, _ := b.bind(v, false)
	return bound
}

func (b *callOwnerBinder) argument(v reflect.Value) reflect.Value {
	if !b.bindArgs {
		return v
	}
	bound, _ := b.bind(v, true)
	return bound
}

func (b *callOwnerBinder) bind(v reflect.Value, hostBoundary bool) (reflect.Value, bool) {
	if !v.IsValid() {
		return v, false
	}
	switch v.Kind() {
	case reflect.Func:
		if v.IsNil() {
			return v, false
		}
		// Argument snapshots are settable reflect values. Canonicalize through
		// Interface so metadata registered for the original MakeFunc value remains
		// discoverable after the function value has been copied into a snapshot.
		target := reflect.ValueOf(v.Interface())
		target = b.f.interp.activateDirectFuncFromExec(b.f, target, b.cancel)
		typ := v.Type()
		targetKey, meta, interpreted := b.f.interp.lookupInterpretedFunc(target)
		if !interpreted {
			return v, false
		}
		root, cancel := b.f.root, b.cancel
		var bound reflect.Value
		cacheKey := boundWrapperKey{target: targetKey, root: root, cancel: cancel, typ: typ, hostBoundary: hostBoundary}
		if meta.group != nil {
			b.f.interp.funcMu.RLock()
			cached, cachedOK := meta.group.bound[cacheKey]
			b.f.interp.funcMu.RUnlock()
			if cachedOK {
				// A sweep may have deleted the alias while the wrapper stayed
				// cached; registration is idempotent and restores metadata
				// discoverability for interpreted re-entry.
				if hostBoundary {
					b.f.interp.registerInterpretedFuncAlias(cached, meta, b.f)
				}
				return cached, true
			}
		}
		bound = reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
			if hostBoundary {
				return invokeInterpretedHostBoundary(b.f.interp, typ, meta.invoke, root, cancel, in, true)
			}
			if funcOwnerCanceled(cancel) {
				return zeroFuncResults(typ)
			}
			return meta.invoke(in, root, cancel)
		})
		if meta.group != nil {
			b.f.interp.funcMu.Lock()
			if meta.group.bound == nil {
				meta.group.bound = map[boundWrapperKey]reflect.Value{}
			}
			if len(meta.group.bound) >= maxBoundWrapperCache {
				// Bound the cache: activations with distinct cancel channels
				// would otherwise accumulate one entry per run on groups that
				// survive sweeps.
				meta.group.bound = map[boundWrapperKey]reflect.Value{}
			}
			meta.group.bound[cacheKey] = bound
			b.f.interp.funcMu.Unlock()
		}
		if hostBoundary {
			b.f.interp.registerInterpretedFuncAlias(bound, meta, b.f)
		}
		return bound, true
	case reflect.Interface:
		if v.IsNil() || v.Elem().Kind() != reflect.Func {
			return v, false
		}
		bound, changed := b.bind(v.Elem(), hostBoundary)
		if !changed {
			return v, false
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(bound)
		return out, true
	default:
		return v, false
	}
}

func markNativeCallOwnedValues(f *frame, binder *callOwnerBinder, target reflect.Value, values []reflect.Value) {
	if binder.bindArgs && target.IsValid() && (target.Kind() != reflect.Func || !target.IsNil()) {
		f.interp.markOwnedValuesHostSharedFromExec(values...)
	}
}

func setOwnedValueOutput(f *frame, destination, value reflect.Value) {
	f.interp.markOwnedCellWriteFromExec(destination, value)
	destination.Set(value)
}

func activateDirectFuncResultsFromExec(f *frame, values []reflect.Value) []reflect.Value {
	for index, value := range values {
		values[index] = activateDirectFuncValueFromExec(f, value)
	}
	return values
}

func activateDirectFuncValueFromExec(f *frame, value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Type() == valueInterfaceType && value.CanInterface() {
		wrapped := value.Interface().(valueInterface)
		wrapped.value = activateDirectFuncValueFromExec(f, wrapped.value)
		return reflect.ValueOf(wrapped)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return value
		}
		active := activateDirectFuncValueFromExec(f, value.Elem())
		if sameFuncvalKey(active, value.Elem()) {
			return value
		}
		out := reflect.New(value.Type()).Elem()
		out.Set(active)
		return out
	}
	if value.Kind() == reflect.Func {
		return f.interp.activateDirectFuncFromExec(f, value, f.cancel)
	}
	return value
}

var builtin = [...]bltnGenerator{
	aNop:          nop,
	aAddr:         addr,
	aAssign:       assign,
	aAdd:          add,
	aAddAssign:    addAssign,
	aAnd:          and,
	aAndAssign:    andAssign,
	aAndNot:       andNot,
	aAndNotAssign: andNotAssign,
	aBitNot:       bitNot,
	aCall:         call,
	aCallSlice:    call,
	aCase:         _case,
	aCompositeLit: arrayLit,
	aDec:          dec,
	aEqual:        equal,
	aGetFunc:      getFunc,
	aGreater:      greater,
	aGreaterEqual: greaterEqual,
	aInc:          inc,
	aLand:         land,
	aLor:          lor,
	aLower:        lower,
	aLowerEqual:   lowerEqual,
	aMul:          mul,
	aMulAssign:    mulAssign,
	aNeg:          neg,
	aNot:          not,
	aNotEqual:     notEqual,
	aOr:           or,
	aOrAssign:     orAssign,
	aPos:          pos,
	aQuo:          quo,
	aQuoAssign:    quoAssign,
	aRange:        _range,
	aRecv:         recv,
	aRem:          rem,
	aRemAssign:    remAssign,
	aReturn:       _return,
	aSend:         send,
	aShl:          shl,
	aShlAssign:    shlAssign,
	aShr:          shr,
	aShrAssign:    shrAssign,
	aSlice:        slice,
	aSlice0:       slice0,
	aStar:         deref,
	aSub:          sub,
	aSubAssign:    subAssign,
	aTypeAssert:   typeAssertShort,
	aXor:          xor,
	aXorAssign:    xorAssign,
}

var receiverStripperRxp *regexp.Regexp

func init() {
	re := `func\(((.*?(, |\)))(.*))`
	var err error
	receiverStripperRxp, err = regexp.Compile(re)
	if err != nil {
		panic(err)
	}
}

type valueInterface struct {
	node  *node
	value reflect.Value
}

var floatType, complexType reflect.Type

func init() {
	floatType = reflect.ValueOf(0.0).Type()
	complexType = reflect.ValueOf(complex(0, 0)).Type()
}

func (interp *Interpreter) run(n *node, cf *frame) {
	if n == nil {
		return
	}
	var f *frame
	if cf == nil {
		// No in-tree caller passes a nil frame, but read interp.frame under
		// its mutex so a future caller cannot race prepareExecutionFrame's
		// root replacement.
		interp.mutex.RLock()
		f = interp.frame
		interp.mutex.RUnlock()
	} else {
		f = newFrame(cf, len(n.types), cf.runid())
	}
	interp.runOnFrame(n, f)
}

// runOnFrame executes n using the supplied frame as its exact execution root.
// Execute uses this to keep a canceled activation separate from the durable
// interpreter frame used by later evaluations.
func (interp *Interpreter) runOnFrame(n *node, f *frame) {
	if n == nil {
		return
	}
	// The fence is held only while the frame is being initialized: execution
	// re-acquires it per step. Use a scoped release so a panic during setup
	// cannot leak the read lock and wedge later sweeps.
	interp.funcSweepMu.RLock()
	func() {
		defer interp.funcSweepMu.RUnlock()
		if !f.done.Chan.IsValid() {
			interp.mutex.RLock()
			cancel := interp.done
			c := reflect.ValueOf(cancel)
			interp.mutex.RUnlock()

			f.mutex.Lock()
			f.done = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: c}
			f.cancel = cancel
			f.mutex.Unlock()
		}

		for i, t := range n.types {
			f.data[i] = reflect.New(t).Elem()
		}
	}()
	runCfg(n.start, f, n, nil)
}

func isExecNode(n *node, exec bltn) bool {
	if n == nil || n.exec == nil || exec == nil {
		return false
	}

	a1 := reflect.ValueOf(n.exec).Pointer()
	a2 := reflect.ValueOf(exec).Pointer()
	return a1 == a2
}

// execIdentity returns the funcval address of a builtin closure: the single
// pointer word of a func variable. Unlike reflect.Value.Pointer(), which
// returns the shared code pointer for closures created by the same generator,
// the funcval address is unique per closure instance and identifies the exact
// node exec.
func execIdentity(exec bltn) uintptr {
	if exec == nil {
		return 0
	}
	return *(*uintptr)(unsafe.Pointer(&exec))
}

// originalExecNode looks in the tree of nodes for the node which has exec,
// aside from n, in order to know where n "inherited" that exec from.
func originalExecNode(n *node, exec bltn) *node {
	execAddr := execIdentity(exec)
	var originalNode *node
	seen := make(map[int64]struct{})
	root := n
	for {
		root = root.anc
		if root == nil {
			break
		}
		if _, ok := seen[root.index]; ok {
			continue
		}

		root.Walk(func(wn *node) bool {
			if _, ok := seen[wn.index]; ok {
				return true
			}
			seen[wn.index] = struct{}{}
			if wn.index == n.index {
				return true
			}
			if wn.exec == nil {
				return true
			}
			if execIdentity(wn.exec) == execAddr {
				originalNode = wn
				return false
			}
			return true
		}, nil)

		if originalNode != nil {
			break
		}
	}

	return originalNode
}

// cloned from net/http/server.go , so we can enforce a similar behavior:
// in the stdlib, this error is used as sentinel in panic triggered e.g. on
// request cancellation, in order to catch it and suppress it in a following defer.
// in yaegi, we use it to suppress a "panic" log message that happens in the
// same circumstances.
var errAbortHandler = errors.New("net/http: abort Handler")

// Functions set to run during execution of CFG.

func panicFunc(s *scope) string {
	if s == nil {
		return ""
	}
	def := s.def
	if def == nil {
		return s.pkgID
	}
	switch def.kind {
	case funcDecl:
		if c := def.child[1]; c.kind == identExpr {
			return s.pkgID + "." + c.ident
		}
	case funcLit:
		if def.anc != nil {
			return panicFunc(def.anc.scope) + ".func"
		}
	}
	return s.pkgID
}

// runCfg executes a node AST by walking its CFG and running node builtin at each step.
//
// runCfg is the ONLY frame activation site (call sites: runOnFrame, the
// declared-function wrapper invoke, the go-statement launch, the interpreted
// call, and the closure-wrapper invoke), so the activeFrames registration
// below covers every execution that can touch frame cells.
func runCfg(n *node, f *frame, funcNode, callNode *node) {
	var exec bltn
	f.interp.funcMu.Lock()
	f.funcState = funcFrameActive
	// Register the activation for the incremental ownership sweep's root set.
	// Refcounted: a root frame can re-enter runCfg via a reentrant Eval while
	// an outer activation on the same frame is still live.
	f.interp.activeFrames[f]++
	f.interp.funcMu.Unlock()
	defer func() {
		recovered := recover()
		f.mutex.Lock()
		f.recovered = recovered
		deferred := f.deferred
		f.deferred = nil
		f.mutex.Unlock()
		// A canceled evaluation's worker unwinds its deferred calls after the
		// API call has returned and released the execution gate, so its
		// interpreted writes could overlap a later evaluation. While its
		// deferred calls run, the funcSweep fence is held exclusively: their
		// interpreted steps exclude every other execution step, while native
		// stretches still release the fence. Only the root's own deferred
		// section toggles the flag; nested frames unwind inside it.
		zombiePhase := f.root != nil && f.root.canceled()
		// The counters are incremented under the exclusive fence so an
		// incremental ownedGC sweep holding the fence reads frameDrains
		// exactly: a draining frame's remaining deferred call values are
		// invisible to the sweep's root set, so no sweep may run while any
		// drain is in progress (the sweep re-checks and stays pending).
		if len(deferred) > 0 || zombiePhase {
			f.interp.funcSweepMu.Lock()
			if zombiePhase {
				f.interp.zombieDefers.Add(1)
			}
			if len(deferred) > 0 {
				f.interp.frameDrains.Add(1)
			}
			f.interp.funcSweepMu.Unlock()
		}
		func() {
			defer func() {
				if zombiePhase {
					f.interp.zombieDefers.Add(-1)
				}
				if len(deferred) > 0 {
					f.interp.frameDrains.Add(-1)
				}
			}()
			for _, deferredCall := range deferred {
				// Cancellation can land while this drain is parked inside an
				// earlier deferred host call — after the one-shot zombiePhase
				// sample above. Re-sample at every deferred-call boundary:
				// once the root's owner has fired, the remaining interpreted
				// deferred steps must hold the exclusive fence (zombieDefers)
				// and the nested gate bypass is refused live via the token's
				// owner channel.
				if !zombiePhase && f.root != nil && f.root.canceled() {
					zombiePhase = true
					f.interp.funcSweepMu.Lock()
					f.interp.zombieDefers.Add(1)
					f.interp.funcSweepMu.Unlock()
				}
				val := deferredCall.values
				func() {
					defer func() {
						if deferredPanic := recover(); deferredPanic != nil {
							f.mutex.RLock()
							previous := f.recovered
							f.mutex.RUnlock()
							if previous != nil {
								f.interp.rollbackInterpretedFuncPanicEscape(previous)
							}
							f.mutex.Lock()
							f.recovered = deferredPanic
							f.mutex.Unlock()
						}
					}()
					if invoke, ok := f.interp.interpretedFunc(val[0]); ok {
						invoke(val[1:], f.root, nil)
					} else if deferredCall.exclusive {
						// Only internally generated pure-reflect deferred
						// closures set exclusive; they cannot re-enter
						// interpreted code or fence bookkeeping, so this
						// bracket stays outside the capture protocol.
						f.interp.funcSweepMu.Lock()
						func() {
							defer f.interp.funcSweepMu.Unlock()
							val[0].Call(val[1:])
						}()
					} else {
						val[0].Call(val[1:])
					}
				}()
			}
		}()
		f.mutex.Lock()
		recovered = f.recovered
		f.mutex.Unlock()
		// Revert the function literal slots when this frame exits. This
		// bracket's own restore is the only frame-cell write the exit code
		// itself performs (the deferred drain can still reach the closure
		// invoker's restore in buildClosureWrapper — a second, master-parity
		// unfenced frame-cell writer this bracket does not cover), so the
		// restore holds the funcSweep fence (read mode): the incremental
		// sweep reads capture cells unfenced, trusting the fence for
		// exclusivity against this write. The fence cannot be held here (the
		// last step released it and the drain counters are closed), so the
		// read lock is safe, and it is released by defer like every other
		// fence bracket. Skipped when another activation shares this frame's
		// cells through a lexical clone (a goroutine or closure activation
		// whose cloneOf chain points at this frame): the shared cell may
		// still be read through the clone, and such an entry simply stays
		// sweep/purge reclaimable. Frames that executed no literal pay only
		// the funcSlots check: the unlocked slice-header read below is safe
		// because a frame's slots are written only by its own executing
		// goroutine (getFunc runs as a step of the frame's single runCfg
		// activation, and root frames never record), the same single-writer
		// property the exec loop relies on for f.data.
		if len(f.funcSlots) > 0 {
			f.mutex.RLock()
			pending := f.funcSlots
			f.mutex.RUnlock()
			if len(pending) > 0 {
				f.interp.funcMu.Lock()
				lastActivation := f.interp.activeFrames[f] <= 1
				for active := range f.interp.activeFrames {
					if active != f && frameSharesSlotsWith(active, f) {
						lastActivation = false
						break
					}
				}
				f.interp.funcMu.Unlock()
				if lastActivation {
					func() {
						f.interp.funcSweepMu.RLock()
						defer f.interp.funcSweepMu.RUnlock()
						applyFuncSlotRestores(f)
					}()
				}
			}
		}
		// Deregister the activation only AFTER the full release sequence and
		// BEFORE the repanic: while this frame's deferred calls drain —
		// including a canceled worker's zombie phase, whose interpreted
		// deferred bodies re-enter runCfg as ordinary activations — the frame
		// must stay in the incremental sweep's root set, because its cells pin
		// state the drain can still touch. The defer keeps the registry exact
		// even if the release sequence itself panics.
		func() {
			defer func() {
				f.interp.funcMu.Lock()
				if count := f.interp.activeFrames[f]; count <= 1 {
					delete(f.interp.activeFrames, f)
				} else {
					f.interp.activeFrames[f] = count - 1
				}
				f.interp.funcMu.Unlock()
			}()
			f.interp.releaseUnreachableChannelSends(f, funcNode)
			f.interp.releaseInterpretedFuncs(f, funcNode, recovered)
			f.interp.releaseOwnedObjects(f, funcNode)
		}()
		if recovered != nil {
			oNode := originalExecNode(n, exec)
			if oNode == nil {
				oNode = n
			}
			panicValue, _ := splitInterpretedPanic(recovered)
			errorer, ok := panicValue.(error)
			// in this specific case, the stdlib would/will suppress the panic, so we
			// suppress the logging here accordingly, to get a similar and consistent
			// behavior.
			if !ok || errorer.Error() != errAbortHandler.Error() {
				fmt.Fprintln(n.interp.stderr, oNode.cfgErrorf("panic: %s(...)", panicFunc(oNode.scope)))
			}
			panic(recovered)
		}
	}()

	dbg := n.interp.debugger
	if dbg == nil {
		// Assign to the outer exec, not a shadow: the deferred recovery above
		// reads it to locate the failing node for the panic position.
		for exec = n.exec; exec != nil && !f.canceled(); {
			exec = execWithFuncSweepFence(exec, f)
		}
		return
	}

	if n.exec == nil {
		return
	}

	dbg.enterCall(funcNode, callNode, f)
	defer dbg.exitCall(funcNode, callNode, f)

	for m, exec := n, n.exec; !f.canceled(); {
		if dbg.exec(m, f) {
			break
		}

		exec = execWithFuncSweepFence(exec, f)
		if exec == nil {
			break
		}

		if m == nil {
			m = originalExecNode(n, exec)
			continue
		}

		switch {
		case isExecNode(m.tnext, exec):
			m = m.tnext
		case isExecNode(m.fnext, exec):
			m = m.fnext
		default:
			m = originalExecNode(m, exec)
		}
	}
}

func execWithFuncSweepFence(exec bltn, f *frame) (next bltn) {
	// Consume a pending incremental ownership sweep BEFORE any acquisition:
	// this goroutine holds no locks here, so the sweep can take the fence
	// exclusively via TryLock and then funcMu — the universal order. A TryLock
	// loss (a zombie deferred step holds the fence) leaves the request pending
	// and retries on the next step.
	f.interp.maybeRunOwnedGCSweep()
	if f.interp.zombieDefers.Load() > 0 {
		// A canceled worker's deferred step runs with the fence held
		// exclusively: its interpreted writes must not overlap another
		// execution's steps on shared containers. Interpreted panics unwind
		// through here, so the release must be panic-safe.
		f.interp.funcSweepMu.Lock()
		f.interp.funcSweepExclusive.Add(1)
		previous := f.fenceExclusive.Swap(true)
		defer func() {
			f.fenceExclusive.Store(previous)
			f.interp.funcSweepExclusive.Add(-1)
			f.interp.funcSweepMu.Unlock()
		}()
		return exec(f)
	}
	f.interp.funcSweepMu.RLock()
	previous := f.fenceExclusive.Swap(false)
	defer func() {
		f.fenceExclusive.Store(previous)
		f.interp.funcSweepMu.RUnlock()
	}()
	return exec(f)
}

// The fence release helpers below must unlock in the mode that was acquired
// for the enclosing step, not in the mode they would re-derive now: the
// zombieDefers counter can flip mid-step while a canceled worker starts or
// finishes its deferred unwind on another goroutine, and releasing in a
// re-derived mode corrupts the RWMutex fatally. The mode acquired by
// execWithFuncSweepFence is captured in f.fenceExclusive and restored when
// the same frame nests another acquisition (a reentrant Eval on the paused
// execution runs its steps on the same global frame).
//
// Invariant: the helpers must only be called from exec-step bodies running
// under an enclosing execWithFuncSweepFence on the same frame. A caller that
// holds no fence would read a stale mode and unlock an unheld lock.

func callWithFuncSweepFenceReleased(f *frame, call func() []reflect.Value) (out []reflect.Value) {
	if f.fenceExclusive.Load() {
		f.interp.funcSweepMu.Unlock()
		defer f.interp.funcSweepMu.Lock()
	} else {
		f.interp.funcSweepMu.RUnlock()
		defer f.interp.funcSweepMu.RLock()
	}
	return call()
}

func runWithFuncSweepFenceReleased(f *frame, run func()) {
	if f.fenceExclusive.Load() {
		f.interp.funcSweepMu.Unlock()
		defer f.interp.funcSweepMu.Lock()
	} else {
		f.interp.funcSweepMu.RUnlock()
		defer f.interp.funcSweepMu.RLock()
	}
	run()
}

func selectWithFuncSweepFenceReleased(f *frame, cases []reflect.SelectCase) (chosen int, recv reflect.Value, recvOK bool) {
	if f.fenceExclusive.Load() {
		f.interp.funcSweepMu.Unlock()
		defer f.interp.funcSweepMu.Lock()
	} else {
		f.interp.funcSweepMu.RUnlock()
		defer f.interp.funcSweepMu.RLock()
	}
	return reflect.Select(cases)
}

func recvWithFuncSweepFenceReleased(f *frame, channel reflect.Value) (value reflect.Value, ok bool) {
	if f.fenceExclusive.Load() {
		f.interp.funcSweepMu.Unlock()
		defer f.interp.funcSweepMu.Lock()
	} else {
		f.interp.funcSweepMu.RUnlock()
		defer f.interp.funcSweepMu.RLock()
	}
	return channel.Recv()
}

func sendWithFuncSweepFenceReleased(f *frame, channel, value reflect.Value) {
	if f.fenceExclusive.Load() {
		f.interp.funcSweepMu.Unlock()
		defer f.interp.funcSweepMu.Lock()
	} else {
		f.interp.funcSweepMu.RUnlock()
		defer f.interp.funcSweepMu.RLock()
	}
	channel.Send(value)
}

func stripReceiverFromArgs(signature string) (string, error) {
	fields := receiverStripperRxp.FindStringSubmatch(signature)
	if len(fields) < 5 {
		return "", errors.New("error while matching method signature")
	}
	if fields[3] == ")" {
		return fmt.Sprintf("func()%s", fields[4]), nil
	}
	return fmt.Sprintf("func(%s", fields[4]), nil
}

func typeAssertShort(n *node) {
	typeAssert(n, true, false)
}

func typeAssertLong(n *node) {
	typeAssert(n, true, true)
}

func typeAssertStatus(n *node) {
	typeAssert(n, false, true)
}

func typeAssert(n *node, withResult, withOk bool) {
	c0, c1 := n.child[0], n.child[1]
	value := genValue(c0) // input value
	var value0, value1 func(*frame) reflect.Value
	setStatus := false
	switch {
	case withResult && withOk:
		value0 = genValue(n.anc.child[0])       // returned result
		value1 = genValue(n.anc.child[1])       // returned status
		setStatus = n.anc.child[1].ident != "_" // do not assign status to "_"
	case withResult && !withOk:
		value0 = genValue(n) // returned result
	case !withResult && withOk:
		value1 = genValue(n.anc.child[1])       // returned status
		setStatus = n.anc.child[1].ident != "_" // do not assign status to "_"
	}

	typ := c1.typ // type to assert or convert to
	typID := typ.id()
	rtype := typ.refType(nil) // type to assert
	next := getExec(n.tnext)
	setResult := func(f *frame, v reflect.Value) {
		destination := value0(f)
		f.interp.markOwnedCellWriteFromExec(destination, v)
		destination.Set(v)
	}
	finishStatus := func(f *frame, ok *bool) {
		if withResult && withOk && !*ok {
			destination := value0(f)
			zero := reflect.Zero(destination.Type())
			f.interp.markOwnedCellWriteFromExec(destination, zero)
			destination.Set(zero)
		}
		if setStatus {
			value1(f).SetBool(*ok)
		}
	}

	switch {
	case isInterfaceSrc(typ):
		n.exec = func(f *frame) bltn {
			valf := value(f)
			v, ok := valf.Interface().(valueInterface)
			if withOk {
				defer finishStatus(f, &ok)
			}
			if !ok {
				if !withOk {
					panic(n.cfgErrorf("interface conversion: nil is not %v", typID))
				}
				return next
			}
			if c0.typ.cat == valueT {
				valf = reflect.ValueOf(v)
			}
			if v.node.typ.id() == typID {
				if withResult {
					setResult(f, valf)
				}
				return next
			}
			m0 := v.node.typ.methods()
			m1 := typ.methods()
			if len(m0) < len(m1) {
				ok = false
				if !withOk {
					panic(n.cfgErrorf("interface conversion: %v is not %v", v.node.typ.id(), typID))
				}
				return next
			}

			for k, meth1 := range m1 {
				var meth0 string
				meth0, ok = m0[k]
				if !ok {
					return next
				}
				// As far as we know this equality check can fail because they are two ways to
				// represent the signature of a method: one where the receiver appears before the
				// func keyword, and one where it is just a func signature, and the receiver is
				// seen as the first argument. That's why if that equality fails, we try harder to
				// compare them afterwards. Hopefully that is the only reason this equality can fail.
				if meth0 == meth1 {
					continue
				}
				tm := lookupFieldOrMethod(v.node.typ, k)
				if tm == nil {
					ok = false
					return next
				}

				var err error
				meth0, err = stripReceiverFromArgs(meth0)
				if err != nil {
					ok = false
					return next
				}

				if meth0 != meth1 {
					ok = false
					return next
				}
			}

			if withResult {
				setResult(f, valf)
			}
			return next
		}
	case isInterface(typ):
		n.exec = func(f *frame) bltn {
			var leftType reflect.Type
			v := value(f)
			val, ok := v.Interface().(valueInterface)
			if withOk {
				defer finishStatus(f, &ok)
			}
			if ok && val.node.typ.cat != valueT {
				m0 := val.node.typ.methods()
				m1 := typ.methods()
				if len(m0) < len(m1) {
					ok = false
					return next
				}

				for k, meth1 := range m1 {
					var meth0 string
					meth0, ok = m0[k]
					if !ok {
						return next
					}
					if meth0 != meth1 {
						ok = false
						return next
					}
				}

				if withResult {
					setResult(f, genInterfaceWrapper(val.node, rtype)(f))
				}
				ok = true
				return next
			}

			if ok {
				v = val.value
				leftType = val.node.typ.rtype
			} else {
				v = v.Elem()
				leftType = v.Type()
				ok = true
			}
			ok = v.IsValid()
			if !ok {
				if !withOk {
					panic(n.cfgErrorf("interface conversion: interface {} is nil, not %s", rtype.String()))
				}
				return next
			}
			ok = canAssertTypes(leftType, rtype)
			if !ok {
				if !withOk {
					method := firstMissingMethod(leftType, rtype)
					panic(n.cfgErrorf("interface conversion: %s is not %s: missing method %s", leftType.String(), rtype.String(), method))
				}
				return next
			}
			if withResult {
				setResult(f, v)
			}
			return next
		}
	case isEmptyInterface(n.child[0].typ):
		n.exec = func(f *frame) bltn {
			var ok bool
			if withOk {
				defer finishStatus(f, &ok)
			}
			val := value(f)
			concrete := val.Interface()
			ctyp := reflect.TypeOf(concrete)

			if vv, ok := concrete.(valueInterface); ok {
				ctyp = vv.value.Type()
				concrete = vv.value.Interface()
			}
			ok = canAssertTypes(ctyp, rtype)
			if !ok {
				if !withOk {
					// TODO(mpl): think about whether this should ever happen.
					if ctyp == nil {
						panic(n.cfgErrorf("interface conversion: interface {} is nil, not %s", rtype.String()))
					}
					panic(n.cfgErrorf("interface conversion: interface {} is %s, not %s", ctyp.String(), rtype.String()))
				}
				return next
			}
			if withResult {
				if isInterfaceSrc(typ) {
					// TODO(mpl): this requires more work. the wrapped node is not complete enough.
					setResult(f, reflect.ValueOf(valueInterface{n.child[0], reflect.ValueOf(concrete)}))
				} else {
					setResult(f, reflect.ValueOf(concrete))
				}
			}
			return next
		}
	case n.child[0].typ.cat == valueT || n.child[0].typ.cat == errorT:
		n.exec = func(f *frame) bltn {
			v := value(f).Elem()
			ok := v.IsValid()
			if withOk {
				defer finishStatus(f, &ok)
			}
			if !ok {
				if !withOk {
					panic(n.cfgErrorf("interface conversion: interface {} is nil, not %s", rtype.String()))
				}
				return next
			}
			v = valueInterfaceValue(v)
			if vt := v.Type(); vt.Kind() == reflect.Struct && vt.Field(0).Name == "IValue" {
				// Value is retrieved from an interface wrapper.
				v = v.Field(0).Elem()
			}
			ok = canAssertTypes(v.Type(), rtype)
			if !ok {
				if !withOk {
					method := firstMissingMethod(v.Type(), rtype)
					panic(n.cfgErrorf("interface conversion: %s is not %s: missing method %s", v.Type().String(), rtype.String(), method))
				}
				return next
			}
			if withResult {
				setResult(f, v)
			}
			return next
		}
	default:
		n.exec = func(f *frame) bltn {
			v, ok := value(f).Interface().(valueInterface)
			if withOk {
				defer finishStatus(f, &ok)
			}
			if !ok || !v.value.IsValid() {
				ok = false
				if !withOk {
					panic(n.cfgErrorf("interface conversion: interface {} is nil, not %s", rtype.String()))
				}
				return next
			}

			ok = canAssertTypes(v.value.Type(), rtype)
			if !ok {
				if !withOk {
					panic(n.cfgErrorf("interface conversion: interface {} is %s, not %s", v.value.Type().String(), rtype.String()))
				}
				return next
			}
			if withResult {
				setResult(f, v.value)
			}
			return next
		}
	}
}

func canAssertTypes(src, dest reflect.Type) bool {
	if dest == nil {
		return false
	}
	if src == dest {
		return true
	}
	if dest.Kind() == reflect.Interface && src.Implements(dest) {
		return true
	}
	if src == nil {
		return false
	}
	if src.AssignableTo(dest) {
		return true
	}
	return false
}

func firstMissingMethod(src, dest reflect.Type) string {
	for i := 0; i < dest.NumMethod(); i++ {
		m := dest.Method(i).Name
		if _, ok := src.MethodByName(m); !ok {
			return m
		}
	}
	return ""
}

func convert(n *node) {
	dest := genValue(n)
	c := n.child[1]
	typ := n.child[0].typ.frameType()
	next := getExec(n.tnext)

	if c.isNil() { // convert nil to type
		// TODO(mpl): Try to completely remove, as maybe frameType already does the job for interfaces.
		if isInterfaceSrc(n.child[0].typ) && !isEmptyInterface(n.child[0].typ) {
			typ = valueInterfaceType
		}
		n.exec = func(f *frame) bltn {
			setOwnedValueOutput(f, dest(f), reflect.New(typ).Elem())
			return next
		}
		return
	}

	doConvert := true
	var value func(*frame) reflect.Value
	switch {
	case isFuncSrc(c.typ):
		value = genFunctionWrapper(c)
	default:
		value = genValue(c)
	}

	for _, hook := range n.interp.hooks.convert {
		if c.typ.rtype == nil {
			continue
		}

		fn := hook.fn(c.typ.rtype, typ)
		if fn == nil {
			continue
		}
		n.exec = func(f *frame) bltn {
			source := cloneCallArgValue(value(f))
			source = newCallOwnerBinder(f, f.cancel).argument(source)
			target := dest(f)
			converted := reflect.New(target.Type()).Elem()
			unsafeIdentity := hook.ownedUnsafeIdentity && (source.Kind() == reflect.UnsafePointer || target.Kind() == reflect.UnsafePointer)
			// unsafe.Pointer conversions are interpreter identity views, not a
			// native publication boundary. The allocation remains owned until the
			// converted value itself is returned or passed to native code.
			if !unsafeIdentity {
				f.interp.markOwnedValuesHostSharedFromExec(source)
			}
			runWithFuncSweepFenceReleased(f, func() { fn(source, converted) })
			converted = activateDirectFuncValueFromExec(f, converted)
			if !unsafeIdentity {
				f.interp.registerNativeResultValuesFromExec(f, converted)
			}
			if f.canceled() {
				return nil
			}
			setOwnedValueOutput(f, target, converted)
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		destination := dest(f)
		if doConvert {
			source := value(f)
			converted := source.Convert(typ)
			f.interp.registerOwnedAppend(converted, source, f)
			setOwnedValueOutput(f, destination, converted)
		} else {
			setOwnedValueOutput(f, destination, value(f))
		}
		return next
	}
}

// assignFromCall assigns values from a function call.
func assignFromCall(n *node) {
	ncall := n.lastChild()
	l := len(n.child) - 1
	if n.anc.kind == varDecl && n.child[l-1].isType(n.scope) {
		// Ignore the type in the assignment if it is part of a variable declaration.
		l--
	}
	dvalue := make([]func(*frame) reflect.Value, l)
	for i := range dvalue {
		if n.child[i].ident == "_" {
			continue
		}
		dvalue[i] = genValue(n.child[i])
	}
	next := getExec(n.tnext)
	n.exec = func(f *frame) bltn {
		for i, v := range dvalue {
			if v == nil {
				continue
			}
			s := f.data[ncall.findex+i]
			c := n.child[i]
			if n.kind == defineXStmt && !c.redeclared {
				// Recreate destination value in case of define statement,
				// to preserve previous value possibly in use by a closure.
				data := getFrame(f, c.level).data
				data[c.findex] = reflect.New(data[c.findex].Type()).Elem()
				data[c.findex].Set(s)
				continue
			}
			destination := v(f)
			f.interp.markOwnedCellWriteFromExec(destination, s)
			destination.Set(s)
		}
		return next
	}
}

func assign(n *node) {
	next := getExec(n.tnext)
	dvalue := make([]func(*frame) reflect.Value, n.nleft)
	ivalue := make([]func(*frame) reflect.Value, n.nleft)
	svalue := make([]func(*frame) reflect.Value, n.nleft)
	var sbase int
	if n.nright > 0 {
		sbase = len(n.child) - n.nright
	}

	for i := 0; i < n.nleft; i++ {
		dest, src := n.child[i], n.child[sbase+i]
		if len(n.assignTmp) == n.nright {
			index := n.assignTmp[i]
			svalue[i] = func(f *frame) reflect.Value { return f.data[index] }
		} else if isNamedFuncSrc(src.typ) {
			svalue[i] = genFuncValue(src)
		} else {
			svalue[i] = genDestValue(dest.typ, src)
		}
		if isMapEntry(dest) {
			// A snapshotted receiver or key (non-passive operand) is read
			// from its stable slot; passive ones are read when the
			// assignment is applied, matching gc.
			if len(dest.assignTmp) == 2 && dest.assignTmp[0] >= 0 {
				receiverIndex := dest.assignTmp[0]
				dvalue[i] = func(f *frame) reflect.Value { return f.data[receiverIndex] }
			} else {
				dvalue[i] = genValue(dest.child[0])
			}
			switch {
			case len(dest.assignTmp) == 2 && dest.assignTmp[1] >= 0:
				keyIndex := dest.assignTmp[1]
				ivalue[i] = func(f *frame) reflect.Value { return f.data[keyIndex] }
			case isInterfaceSrc(dest.child[1].typ): // key
				ivalue[i] = genValueInterface(dest.child[1])
			default:
				ivalue[i] = genValue(dest.child[1])
			}
		} else {
			dvalue[i] = genValue(dest)
		}
	}

	if n.nleft == 1 {
		// Single assign operation.
		switch s, d, i := svalue[0], dvalue[0], ivalue[0]; {
		case n.child[0].ident == "_":
			n.exec = func(f *frame) bltn {
				return next
			}
		case i != nil:
			n.exec = func(f *frame) bltn {
				destination, source := d(f), s(f)
				key := i(f)
				f.interp.markOwnedWriteFromExec(destination, key, source)
				destination.SetMapIndex(key, source)
				return next
			}
		case n.kind == defineStmt:
			l := n.level
			ind := n.findex
			n.exec = func(f *frame) bltn {
				data := getFrame(f, l).data
				data[ind] = reflect.New(data[ind].Type()).Elem()
				data[ind].Set(s(f))
				return next
			}
		default:
			n.exec = func(f *frame) bltn {
				destination, source := d(f), s(f)
				f.interp.markOwnedCellWriteFromExec(destination, source)
				destination.Set(source)
				return next
			}
		}
		return
	}

	if n.kind == defineStmt {
		// Handle a multiple var declararation / assign. It cannot be a swap.
		n.exec = func(f *frame) bltn {
			for i, s := range svalue {
				if n.child[i].ident == "_" {
					continue
				}
				data := getFrame(f, n.child[i].level).data
				j := n.child[i].findex
				data[j] = reflect.New(data[j].Type()).Elem()
				data[j].Set(s(f))
			}
			return next
		}
		return
	}

	if len(n.assignTmp) == n.nright {
		// The right-hand sides already live in stable, destination-typed frame
		// slots. Assign them directly without a second set of temporaries.
		n.exec = func(f *frame) bltn {
			for i, d := range dvalue {
				if n.child[i].ident == "_" {
					continue
				}
				s := svalue[i](f)
				if j := ivalue[i]; j != nil {
					destination := d(f)
					key := j(f)
					f.interp.markOwnedWriteFromExec(destination, key, s)
					destination.SetMapIndex(key, s)
				} else {
					destination := d(f)
					f.interp.markOwnedCellWriteFromExec(destination, s)
					destination.Set(s)
				}
			}
			return next
		}
		return
	}

	// Multi assign operation.
	types := make([]reflect.Type, n.nright)
	for i := range types {
		var t reflect.Type
		switch typ := n.child[sbase+i].typ; {
		case isInterfaceSrc(typ):
			t = valueInterfaceType
		default:
			t = typ.TypeOf()
		}
		types[i] = t
	}

	// To handle possible swap in multi-assign:
	// evaluate and copy all values in assign right hand side into temporary
	// then evaluate assign left hand side and copy temporary into it
	n.exec = func(f *frame) bltn {
		t := make([]reflect.Value, len(svalue))
		for i, s := range svalue {
			if n.child[i].ident == "_" {
				continue
			}
			t[i] = reflect.New(types[i]).Elem()
			t[i].Set(s(f))
		}
		for i, d := range dvalue {
			if n.child[i].ident == "_" {
				continue
			}
			if j := ivalue[i]; j != nil {
				destination := d(f)
				key := j(f)
				f.interp.markOwnedWriteFromExec(destination, key, t[i])
				destination.SetMapIndex(key, t[i]) // Assign a map entry
			} else {
				destination := d(f)
				f.interp.markOwnedCellWriteFromExec(destination, t[i])
				destination.Set(t[i]) // Assign a var or array/slice entry
			}
		}
		return next
	}
}

// snapshotAssign stores one right-hand-side value of a multi-assignment in a
// temporary frame slot. The synthetic snapshot nodes are sequenced between
// operand CFGs, preserving Go's left-to-right evaluation before assign updates
// any destination.
func snapshotAssign(n *node) {
	value := genDestValue(n.typ, n.assignSrc)
	index := n.findex
	next := getExec(n.tnext)
	n.exec = func(f *frame) bltn {
		f.data[index].Set(value(f))
		return next
	}
}

func snapshotCallArg(n *node) {
	value := n.callValue
	next := getExec(n.tnext)
	if value == nil {
		// cfg guarantees one snapshot node per call argument; a generator
		// mismatch must degrade to a direct evaluation instead of calling a
		// nil value func at runtime.
		n.exec = func(f *frame) bltn { return next }
		return
	}
	n.exec = func(f *frame) bltn {
		v := value(f)
		if v.IsValid() {
			if wrapped, ok := v.Interface().(valueInterface); ok {
				wrapped.value = cloneCallArgValue(wrapped.value)
				v = reflect.ValueOf(wrapped)
			} else {
				v = cloneCallArgValue(v)
			}
		}
		f.setCallArg(n, v)
		return next
	}
}

func cloneCallArgValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	copy := reflect.New(v.Type()).Elem()
	copy.Set(v)
	return copy
}

func snapshotMethodReceiver(src reflect.Value, destType reflect.Type) reflect.Value {
	for src.IsValid() {
		wrapped, ok := src.Interface().(valueInterface)
		if !ok {
			break
		}
		src = wrapped.value
	}
	switch {
	case src.Kind() == reflect.Ptr && destType.Kind() != reflect.Ptr:
		// A value-receiver method selected through a pointer saves *src now.
		return cloneCallArgValue(src.Elem())
	case src.Kind() != reflect.Ptr && destType.Kind() == reflect.Ptr:
		// A pointer-receiver method selected from an addressable value saves its
		// address, so later mutations through that value remain visible.
		return src.Addr()
	default:
		return cloneCallArgValue(src)
	}
}

func useCallArgSnapshots(n *node, values []func(*frame) reflect.Value) {
	if len(n.callSnaps) != len(values) {
		return
	}
	for i, snapshot := range n.callSnaps {
		snapshot.callValue = values[i]
		snap := snapshot
		values[i] = func(f *frame) reflect.Value {
			return f.callArg(snap)
		}
	}
}

func useCallFuncSnapshot(n *node, value *func(*frame) reflect.Value) {
	if n.callFunc == nil {
		return
	}
	n.callFunc.callValue = *value
	snapshot := n.callFunc
	*value = func(f *frame) reflect.Value {
		return f.callArg(snapshot)
	}
}

func not(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if !value(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			dest(f).SetBool(!value(f).Bool())
			return tnext
		}
	}
}

func addr(n *node) {
	dest := genValue(n)
	next := getExec(n.tnext)
	c0 := n.child[0]
	value := genValue(c0)

	if isInterfaceSrc(c0.typ) || isPtrSrc(c0.typ) {
		i := n.findex
		l := n.level
		n.exec = func(f *frame) bltn {
			address := value(f).Addr()
			f.interp.registerOwnedAddress(address, f)
			getFrame(f, l).data[i] = address
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		address := value(f).Addr()
		f.interp.registerOwnedAddress(address, f)
		dest(f).Set(address)
		return next
	}
}

func deref(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)
	i := n.findex
	l := n.level

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			r := value(f).Elem()
			if r.Bool() {
				getFrame(f, l).data[i] = r
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem()
			return tnext
		}
	}
}

func _print(n *node) {
	child := n.child[1:]
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}
	useCallArgSnapshots(n, values)
	out := n.interp.stdout

	genBuiltinDeferWrapper(n, values, true, false, func(args []reflect.Value) []reflect.Value {
		for i, value := range args {
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
			fmt.Fprintf(out, "%v", value)
		}
		return nil
	})
}

func _println(n *node) {
	child := n.child[1:]
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		values[i] = genValue(c)
	}
	useCallArgSnapshots(n, values)
	out := n.interp.stdout

	genBuiltinDeferWrapper(n, values, true, false, func(args []reflect.Value) []reflect.Value {
		for i, value := range args {
			if i > 0 {
				fmt.Fprintf(out, " ")
			}
			fmt.Fprintf(out, "%v", value)
		}
		fmt.Fprintln(out, "")
		return nil
	})
}

func _recover(n *node) {
	tnext := getExec(n.tnext)
	dest := genValue(n)

	n.exec = func(f *frame) bltn {
		if f.anc.recovered == nil {
			// TODO(mpl): maybe we don't need that special case, and we're just forgetting to unwrap the valueInterface somewhere else.
			if isEmptyInterface(n.typ) {
				return tnext
			}
			dest(f).Set(reflect.ValueOf(valueInterface{}))
			return tnext
		}
		recovered, token := splitInterpretedPanic(f.anc.recovered)
		if value, ok := recovered.(reflect.Value); ok && value.IsValid() && value.CanInterface() {
			recovered = value.Interface()
		}
		recoveredValue := f.interp.adoptInterpretedFuncPanicValue(f, token, reflect.ValueOf(recovered))
		if recoveredValue.IsValid() && recoveredValue.CanInterface() {
			recovered = recoveredValue.Interface()
		}

		if isEmptyInterface(n.typ) {
			dest(f).Set(reflect.ValueOf(recovered))
		} else {
			dest(f).Set(reflect.ValueOf(valueInterface{n, reflect.ValueOf(recovered)}))
		}
		f.anc.recovered = nil
		return tnext
	}
}

func _panic(n *node) {
	value := genValue(n.child[1])

	n.exec = func(f *frame) bltn {
		v := value(f)
		panic(f.interp.beginInterpretedFuncPanic(v))
	}
}

func genBuiltinDeferWrapper(n *node, in []func(*frame) reflect.Value, releaseFence, exclusive bool, fn func([]reflect.Value) []reflect.Value) {
	next := getExec(n.tnext)

	if n.anc.kind == deferStmt {
		n.exec = func(f *frame) bltn {
			val := make([]reflect.Value, len(in)+1)
			inTypes := make([]reflect.Type, len(in))
			for i, v := range in {
				val[i+1] = v(f)
				inTypes[i] = val[i+1].Type()
			}
			if releaseFence {
				f.interp.markOwnedValuesHostSharedFromExec(val[1:]...)
			}

			funcType := reflect.FuncOf(inTypes, nil, false)
			val[0] = reflect.MakeFunc(funcType, fn)
			f.deferred = append([]deferredCall{{values: val, exclusive: exclusive}}, f.deferred...)
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		val := make([]reflect.Value, len(in))
		for i, v := range in {
			val[i] = v(f)
		}
		if releaseFence {
			f.interp.markOwnedValuesHostSharedFromExec(val...)
		}

		if releaseFence {
			runWithFuncSweepFenceReleased(f, func() { fn(val) })
		} else {
			fn(val)
		}
		return next
	}
}

func genFunctionWrapper(n *node) func(*frame) reflect.Value {
	var def *node
	var ok bool

	if def, ok = n.val.(*node); !ok {
		return genValueAsFunctionWrapper(n)
	}
	numRet := len(def.typ.ret)
	var rcvr func(*frame) reflect.Value

	// A receiver with no node and no value marks an unbound receiver (method
	// expression): the receiver is provided at call time as first argument,
	// there is nothing to read from the frame.
	if n.recv != nil && (n.recv.node != nil || n.recv.val.IsValid()) {
		rcvr = genValueRecv(n)
	}
	value := genValue(n)
	var isDefer bool
	if n.anc != nil && n.anc.anc != nil && n.anc.anc.kind == deferStmt {
		isDefer = true
	}

	return func(f *frame) reflect.Value {
		v := value(f)
		if !isDefer && v.Kind() == reflect.Func {
			// fixes #1634, if v is already a func, then don't re-wrap
			// because original wrapping cloned the frame but this doesn't
			return v
		}
		var receiver reflect.Value
		if rcvr != nil {
			receiver = snapshotMethodReceiver(rcvr(f), def.types[numRet])
		}
		build := buildDeclaredFunctionWrapper(n, def, f, receiver, f.root, f.cancel)
		n.interp.registerInterpretedFuncWithRebinder(build.value, build.invoke, build.rebind, f, build.captures)
		return build.value
	}
}

func makeInterpretedHostWrapper(interp *Interpreter, typ reflect.Type, invoke interpretedFuncInvoker, root *frame, cancel <-chan struct{}) reflect.Value {
	return reflect.MakeFunc(typ, func(in []reflect.Value) []reflect.Value {
		return invokeInterpretedHostBoundary(interp, typ, invoke, root, cancel, in, false)
	})
}

func funcOwnerCanceled(cancel <-chan struct{}) bool {
	if cancel == nil {
		return false
	}
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

func zeroFuncResults(typ reflect.Type) []reflect.Value {
	out := make([]reflect.Value, typ.NumOut())
	for index := range out {
		out[index] = reflect.Zero(typ.Out(index))
	}
	return out
}

func invokeInterpretedHostBoundary(interp *Interpreter, typ reflect.Type, invoke interpretedFuncInvoker, root *frame, cancel <-chan struct{}, in []reflect.Value, rejectAlreadyCanceled bool) (out []reflect.Value) {
	canceledAtEntry := funcOwnerCanceled(cancel)
	if canceledAtEntry && rejectAlreadyCanceled {
		return zeroFuncResults(typ)
	}
	fenceCancellation := !canceledAtEntry
	defer func() {
		if recovered := recover(); recovered != nil {
			if fenceCancellation && funcOwnerCanceled(cancel) {
				interp.rollbackOwnedPublishedPanic(recovered)
				out = zeroFuncResults(typ)
				return
			}
			panic(interp.publishHostPanic(recovered))
		}
		if fenceCancellation && funcOwnerCanceled(cancel) {
			out = zeroFuncResults(typ)
			return
		}
		interp.publishHostValues(out...)
	}()
	out = invoke(in, root, cancel)
	return out
}

func buildDeclaredFunctionWrapper(n, def *node, base *frame, receiver reflect.Value, root *frame, cancel <-chan struct{}) interpretedFuncBuild {
	numRet := len(def.typ.ret)
	start := def.child[3].start
	// An unbound method wrapper (from a method expression) receives the method
	// receiver as its first call argument, before the method own arguments.
	unbound := !receiver.IsValid() && def.typ.recv != nil
	argTypes := def.typ.arg
	if unbound {
		argTypes = append([]*itype{def.typ.recv}, def.typ.arg...)
	}
	invoke := func(in []reflect.Value, root *frame, cancel <-chan struct{}) []reflect.Value {
		// Allocate and init local frame. All values to be settable and addressable.
		fr := newFrame(base, len(def.types), base.runid())
		if root != nil {
			fr.root = root
		}
		fr.cancel = cancel
		fr.done = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(cancel)}
		d := fr.data
		for i, t := range def.types {
			d[i] = reflect.New(t).Elem()
		}

		if !receiver.IsValid() {
			d = d[numRet:]
		} else {
			// Copy method receiver as first argument.
			src, dest := receiver, d[numRet]
			sk, dk := src.Kind(), dest.Kind()
			for {
				vs, ok := src.Interface().(valueInterface)
				if !ok {
					break
				}
				src = vs.value
				sk = src.Kind()
			}
			switch {
			case sk == reflect.Ptr && dk != reflect.Ptr:
				dest.Set(src.Elem())
			case sk != reflect.Ptr && dk == reflect.Ptr:
				dest.Set(src.Addr())
			default:
				dest.Set(src)
			}
			d = d[numRet+1:]
		}

		// Copy function input arguments in local frame.
		for i, arg := range in {
			if i >= len(d) {
				// In case of unused arg, there may be not even a frame entry allocated, just skip.
				break
			}
			if unbound && i == 0 {
				// The first argument is the method receiver: project it onto
				// the embedded receiver of a promoted method if any, and
				// accommodate a pointer or value base in the expression.
				arg = unboundMethodReceiver(n, arg, d[i].Kind())
			}
			typ := argTypes[i]
			// Note: for an unbound wrapper, argTypes was built with the
			// receiver prepended, so argTypes[i] is the type of the same
			// argument as d[i] (the receiver at position 0).
			switch {
			case isEmptyInterface(typ) || typ.TypeOf() == valueInterfaceType:
				d[i].Set(arg)
			case isInterfaceSrc(typ):
				d[i].Set(reflect.ValueOf(valueInterface{value: arg.Elem()}))
			default:
				d[i].Set(arg)
			}
		}

		// Interpreter code execution.
		runCfg(start, fr, def, n)

		return fr.data[:numRet]
	}
	funcType := n.typ.TypeOf()
	boundRoot, boundCancel := root, cancel
	wrapper := makeInterpretedHostWrapper(n.interp, funcType, invoke, boundRoot, boundCancel)
	rebind := func(c *detachedRootCloner) interpretedFuncBuild {
		clonedBase := c.cloneFrame(base)
		clonedReceiver := receiver
		if receiver.IsValid() {
			clonedReceiver = c.cloneValue(receiver, false)
		}
		return buildDeclaredFunctionWrapper(n, def, clonedBase, clonedReceiver, c.newRoot, c.cancel)
	}
	return interpretedFuncBuild{value: wrapper, invoke: invoke, rebind: rebind}
}

// unboundMethodReceiver adapts the first call argument of an unbound method
// (a method expression) to the method receiver expected by the method frame:
// it projects an outer receiver onto the embedded field declaring a promoted
// method, and accommodates a value or pointer base in the expression.
func unboundMethodReceiver(n *node, src reflect.Value, destKind reflect.Kind) reflect.Value {
	if n.recv != nil {
		for _, i := range n.recv.index {
			if src.Kind() == reflect.Ptr {
				src = src.Elem()
			}
			src = src.Field(i)
			if vi, ok := src.Interface().(valueInterface); ok {
				src = vi.value
			}
		}
	}
	switch {
	case src.Kind() == reflect.Ptr && destKind != reflect.Ptr:
		// A value-receiver method selected through a pointer base: the
		// receiver is a copy of the pointed value, as in native Go.
		return src.Elem()
	case src.Kind() != reflect.Ptr && destKind == reflect.Ptr:
		// A pointer-receiver method with a value base can only occur on an
		// addressable projection of the receiver argument.
		if !src.CanAddr() {
			c := reflect.New(src.Type()).Elem()
			c.Set(src)
			src = c
		}
		return src.Addr()
	}
	return src
}

func genInterfaceWrapper(n *node, typ reflect.Type) func(*frame) reflect.Value {
	value := genValue(n)
	if typ == nil || typ.Kind() != reflect.Interface || typ.NumMethod() == 0 || n.typ.cat == valueT {
		return value
	}
	tc := n.typ.cat
	if tc != structT {
		// Always force wrapper generation for struct types, as they may contain
		// embedded interface fields which require wrapping, even if reported as
		// implementing typ by reflect.
		if nt := n.typ.frameType(); nt != nil && nt.Implements(typ) {
			return value
		}
	}

	// Retrieve methods from the interface wrapper, which is a struct where all fields
	// except the first define the methods to implement.
	// As the field name was generated with a prefixed first character (in order to avoid
	// collisions with method names), this first character is ignored in comparisons.
	wrap := getWrapper(n, typ)
	mn := wrap.NumField() - 1
	names := make([]string, mn)
	methods := make([]*node, mn)
	indexes := make([][]int, mn)
	for i := 0; i < mn; i++ {
		names[i] = wrap.Field(i + 1).Name[1:]
		methods[i], indexes[i] = n.typ.lookupMethod(names[i])
		if methods[i] == nil && n.typ.cat != nilT {
			// interpreted method not found, look for binary method, possibly embedded
			_, indexes[i], _, _ = n.typ.lookupBinMethod(names[i])
		}
	}

	return func(f *frame) reflect.Value {
		v := value(f)
		if tc != structT && v.Type().Implements(typ) {
			return v
		}
		switch v.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			if v.IsNil() {
				return reflect.New(typ).Elem()
			}
		}
		var n2 *node
		if vi, ok := v.Interface().(valueInterface); ok {
			n2 = vi.node
		}
		// Keep the whole concrete value in the wrapper first field, so it can
		// be retrieved intact by binary code reflecting over the wrapper (i.e.
		// for marshalling) or by type assertions. Only valueInterface layers
		// are unwrapped here.
		v = valueInterfaceValue(v)
		w := reflect.New(wrap).Elem()
		w.Field(0).Set(v)
		for i, m := range methods {
			if m == nil {
				// First direct method lookup on field.
				if r := methodByName(v, names[i], indexes[i]); r.IsValid() {
					w.Field(i + 1).Set(r)
					continue
				}
				if n2 == nil {
					panic(n.cfgErrorf("method not found: %s", names[i]))
				}
				// Method lookup in embedded valueInterface.
				m2, i2 := n2.typ.lookupMethod(names[i])
				if m2 != nil {
					nod := *m2
					nod.recv = &receiver{n, v, i2}
					w.Field(i + 1).Set(genFunctionWrapper(&nod)(f))
					continue
				}
				panic(n.cfgErrorf("method not found: %s", names[i]))
			}
			nod := *m
			nod.recv = &receiver{n, v, indexes[i]}
			w.Field(i + 1).Set(genFunctionWrapper(&nod)(f))
		}
		return w
	}
}

// methodByName returns the method corresponding to name on value, or nil if not found.
// The search is extended on valueInterface wrapper if present.
// If valid, the returned value is a method function with the receiver already set
// (no need to pass it at call).
func methodByName(value reflect.Value, name string, index []int) (v reflect.Value) {
	if vi, ok := value.Interface().(valueInterface); ok {
		if v = getConcreteValue(vi.value).MethodByName(name); v.IsValid() {
			return
		}
	}
	if v = value.MethodByName(name); v.IsValid() {
		return
	}
	if value.Kind() == reflect.Struct {
		// The method may be promoted from an embedded struct field: get it
		// from the field at the index path computed at compile time.
		if checkFieldIndex(value.Type(), index) {
			v = methodFromField(value.FieldByIndex(index), name)
		}
		return
	}
	for value.Kind() == reflect.Ptr {
		value = value.Elem()
		if checkFieldIndex(value.Type(), index) {
			value = value.FieldByIndex(index)
			// The promoted method may have a pointer receiver: get it from
			// the address of the embedded field.
			if v = methodFromField(value, name); v.IsValid() {
				return
			}
			continue
		}
		if v = value.MethodByName(name); v.IsValid() {
			return
		}
	}
	return
}

// methodFromField returns the method name of a struct field, with either a
// value receiver or, if the field is addressable, a pointer receiver.
func methodFromField(field reflect.Value, name string) reflect.Value {
	if v := field.MethodByName(name); v.IsValid() {
		return v
	}
	if field.CanAddr() {
		if v := field.Addr().MethodByName(name); v.IsValid() {
			return v
		}
	}
	return reflect.Value{}
}

func checkFieldIndex(typ reflect.Type, index []int) bool {
	if len(index) == 0 {
		return false
	}
	t := typ
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	i := index[0]
	if i >= t.NumField() {
		return false
	}
	if len(index) > 1 {
		return checkFieldIndex(t.Field(i).Type, index[1:])
	}
	return true
}

func call(n *node) {
	goroutine := n.anc.kind == goStmt
	c0 := n.child[0]
	value := genValue(c0)
	if n.anc.kind == deferStmt {
		// Deferred interpreted calls need a reflected wrapper, but the wrapper is
		// still the call target and must therefore be evaluated before arguments.
		value = genFunctionWrapper(c0)
	}
	useCallFuncSnapshot(n, &value)
	var values []func(*frame) reflect.Value

	numRet := len(c0.typ.ret)
	variadic := variadicPos(n)
	child := n.child[1:]
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
	hasVariadicArgs := n.action == aCallSlice // callSlice implies variadic call with ellipsis.

	// Compute input argument value functions.
	for i, c := range child {
		var arg *itype
		if variadic >= 0 && i >= variadic {
			arg = c0.typ.arg[variadic].val
		} else {
			arg = c0.typ.arg[i]
		}
		switch {
		case isBinCall(c, c.scope):
			// Handle nested function calls: pass returned values as arguments.
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				if hasVariadicArgs || !isInterfaceSrc(arg) || isEmptyInterface(arg) {
					values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
					continue
				}
				values = append(values, func(f *frame) reflect.Value {
					return reflect.ValueOf(valueInterface{value: f.data[ind]})
				})
			}
		case isRegularCall(c):
			// Arguments are return values of a nested function call.
			cc0 := c.child[0]
			for j := range cc0.typ.ret {
				ind := c.findex + j
				if hasVariadicArgs || !isInterfaceSrc(arg) || isEmptyInterface(arg) {
					values = append(values, func(f *frame) reflect.Value { return f.data[ind] })
					continue
				}
				values = append(values, func(f *frame) reflect.Value {
					return reflect.ValueOf(valueInterface{node: cc0.typ.ret[j].node, value: f.data[ind]})
				})
			}
		default:
			if c.kind == basicLit || c.rval.IsValid() {
				argType := arg.TypeOf()
				convertLiteralValue(c, argType)
			}
			switch {
			case hasVariadicArgs:
				values = append(values, genValue(c))
			case isInterfaceSrc(arg) && (!isEmptyInterface(arg) || len(c.typ.method) > 0):
				values = append(values, genValueInterface(c))
			case isInterfaceBin(arg):
				values = append(values, genInterfaceWrapper(c, arg.rtype))
			case isFuncSrc(arg):
				values = append(values, genFuncValue(c))
			default:
				values = append(values, genValue(c))
			}
		}
	}
	useCallArgSnapshots(n, values)

	// Compute output argument value functions.
	rtypes := c0.typ.ret
	rvalues := make([]func(*frame) reflect.Value, len(rtypes))
	switch n.anc.kind {
	case defineXStmt, assignXStmt:
		l := n.level
		for i := range rvalues {
			c := n.anc.child[i]
			switch {
			case c.ident == "_":
				// Skip assigning return value to blank var.
			case isInterfaceSrc(c.typ) && !isEmptyInterface(c.typ) && !isInterfaceSrc(rtypes[i]):
				rvalues[i] = genValueInterfaceValue(c)
			default:
				j := n.findex + i
				rvalues[i] = func(f *frame) reflect.Value { return getFrame(f, l).data[j] }
			}
		}
	case returnStmt:
		// Function call from a return statement: forward return values (always at frame start).
		for i := range rtypes {
			j := n.findex + i
			// Set the return value location in return value of caller frame.
			rvalues[i] = func(f *frame) reflect.Value { return f.data[j] }
		}
	default:
		// Multiple return values frame index are indexed from the node frame index.
		l := n.level
		for i := range rtypes {
			j := n.findex + i
			rvalues[i] = func(f *frame) reflect.Value { return getFrame(f, l).data[j] }
		}
	}

	if n.anc.kind == deferStmt {
		// Store function call in frame for deferred execution.
		n.exec = func(f *frame) bltn {
			binder := newCallOwnerBinder(f, nil)
			val := make([]reflect.Value, len(values)+1)
			val[0] = value(f)
			if _, interpreted := f.interp.interpretedFunc(val[0]); interpreted {
				binder.bindArgs = false
			}
			for i, v := range values {
				val[i+1] = binder.argument(v(f))
			}
			markNativeCallOwnedValues(f, binder, val[0], val[1:])
			f.deferred = append([]deferredCall{{values: val}}, f.deferred...)
			return tnext
		}
		return
	}

	n.exec = func(f *frame) bltn {
		binder := newCallOwnerBinder(f, f.cancel)
		var bf reflect.Value
		if n.callFunc != nil {
			bf = value(f)
		} else {
			f.mutex.Lock()
			bf = value(f)
			f.mutex.Unlock()
		}
		def, ok := bf.Interface().(*node)
		if ok {
			bf = def.rval
		}
		if _, interpreted := f.interp.interpretedFunc(bf); interpreted {
			// Raw interpreted wrappers are permanently bound to the owner that
			// created them, so values which escape through shared Go references
			// cannot attach to a later Eval. Calls made by interpreted code are
			// rebound here to the active caller while metadata is available.
			binder.bindArgs = false
			bf = binder.value(bf)
		}

		// Call bin func if defined
		if bf.IsValid() {
			var callf func([]reflect.Value) []reflect.Value

			// Lambda definitions are necessary here. Due to reflect internals,
			// having `callf = bf.Call` or `callf = bf.CallSlice` does not work.
			//nolint:gocritic
			if hasVariadicArgs {
				callf = func(in []reflect.Value) []reflect.Value { return bf.CallSlice(in) }
			} else {
				callf = func(in []reflect.Value) []reflect.Value { return bf.Call(in) }
			}

			if goroutine {
				// Goroutine's arguments should be copied.
				in := make([]reflect.Value, len(values))
				for i, v := range values {
					value := binder.argument(v(f))
					in[i] = reflect.New(value.Type()).Elem()
					in[i].Set(value)
				}

				markNativeCallOwnedValues(f, binder, bf, in)
				go func() {
					callf(in)
					// Eval may have ended before this discarded call. Sweep once
					// its native result boundary is complete.
					f.interp.sweepRootInterpretedFuncs(f.root, reflect.Value{})
				}()
				return tnext
			}

			in := make([]reflect.Value, len(values))
			for i, v := range values {
				in[i] = binder.argument(v(f))
			}
			markNativeCallOwnedValues(f, binder, bf, in)
			out := callWithFuncSweepFenceReleased(f, func() []reflect.Value { return callf(in) })
			if binder.bindArgs {
				out = activateDirectFuncResultsFromExec(f, out)
				f.interp.registerNativeResultValuesFromExec(f, out...)
			}
			if f.canceled() {
				return nil
			}
			for i, v := range rvalues {
				if v != nil {
					destination := v(f)
					f.interp.markOwnedCellWriteFromExec(destination, out[i])
					destination.Set(out[i])
				}
			}
			if fnext != nil && !out[0].Bool() {
				return fnext
			}
			return tnext
		}

		nf := newFrame(f, len(def.types), f.runid())
		var vararg reflect.Value

		// Init return values
		for i, v := range rvalues {
			if v != nil {
				nf.data[i] = v(f)
			} else {
				nf.data[i] = reflect.New(def.types[i]).Elem()
			}
		}

		// Init local frame values
		for i, t := range def.types[numRet:] {
			nf.data[numRet+i] = reflect.New(t).Elem()
		}

		// Init variadic argument vector
		if variadic >= 0 {
			vararg = nf.data[numRet+variadic]
		}

		// Copy input parameters from caller
		if dest := nf.data[numRet:]; len(dest) > 0 {
			for i, v := range values {
				switch {
				case variadic >= 0 && i >= variadic:
					if v(f).Type() == vararg.Type() {
						vararg.Set(v(f))
					} else {
						source := vararg
						appended := reflect.Append(source, v(f))
						f.interp.registerOwnedAppend(appended, source, nf)
						vararg.Set(appended)
					}
				default:
					val := v(f)
					if val.IsZero() && dest[i].Kind() != reflect.Interface {
						// Work around a recursive struct zero interface issue.
						// Once there is a better way to handle this case, the dest can just be set.
						continue
					}
					if nod, ok := val.Interface().(*node); ok && nod.recv != nil {
						// An interpreted method is passed as value in a function call.
						// It must be wrapped now, otherwise the receiver will be missing
						// at the method call (#1332).
						// TODO (marc): wrapping interpreted functions should be always done
						// everywhere at runtime to simplify the whole code,
						// but it requires deeper refactoring.
						dest[i] = genFunctionWrapper(nod)(f)
						continue
					}
					dest[i].Set(val)
				}
			}
		}

		// Execute function body
		if goroutine {
			go runCfg(def.child[3].start, nf, def, n)
			return tnext
		}
		runWithFuncSweepFenceReleased(f, func() { runCfg(def.child[3].start, nf, def, n) })
		if f.canceled() {
			return nil
		}

		// Set return values
		for i, v := range rvalues {
			if v != nil {
				destination, source := v(f), nf.data[i]
				f.interp.markOwnedCellWriteFromExec(destination, source)
				destination.Set(source)
			}
		}

		// Handle branching according to boolean result
		if fnext != nil && !nf.data[0].Bool() {
			return fnext
		}
		return tnext
	}
}

func getFrame(f *frame, l int) *frame {
	switch l {
	case globalFrame:
		return f.root
	case 0:
		return f
	case 1:
		return f.anc
	case 2:
		return f.anc.anc
	}
	for ; l > 0; l-- {
		f = f.anc
	}
	return f
}

// Callbin calls a function from a bin import, accessible through reflect.
func callBin(n *node) {
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
	child := n.child[1:]
	c0 := n.child[0]
	value := genValue(c0)
	useCallFuncSnapshot(n, &value)
	activeValue := func(f *frame, binder *callOwnerBinder) reflect.Value {
		v := value(f)
		if _, interpreted := f.interp.interpretedFunc(v); interpreted {
			binder.bindArgs = false
			return binder.value(v)
		}
		return v
	}
	var values []func(*frame) reflect.Value
	funcType := c0.typ.rtype
	wt := wrappedType(c0)
	variadic := -1
	if funcType.IsVariadic() {
		variadic = funcType.NumIn() - 1
	}
	// A method signature obtained from reflect.Type includes receiver as 1st arg, except for interface types.
	rcvrOffset := 0
	if recv := c0.recv; recv != nil && !isInterface(recv.node.typ) {
		if variadic > 0 || funcType.NumIn() > len(child) {
			rcvrOffset = 1
		}
	}

	// getMapType returns a reflect type suitable for interface wrapper for functions
	// with some special processing in case of interface{} argument, i.e. fmt.Printf.
	var getMapType func(*itype) reflect.Type
	if lr, ok := n.interp.mapTypes[c0.rval]; ok {
		getMapType = func(typ *itype) reflect.Type {
			for _, rt := range lr {
				if typ.implements(&itype{cat: valueT, rtype: rt}) {
					return rt
				}
			}
			return nil
		}
	}

	// Determine if we should use `Call` or `CallSlice` on the function Value.
	callFn := func(v reflect.Value, in []reflect.Value) []reflect.Value { return v.Call(in) }
	if n.action == aCallSlice {
		callFn = func(v reflect.Value, in []reflect.Value) []reflect.Value { return v.CallSlice(in) }
	}

	for i, c := range child {
		switch {
		case isBinCall(c, c.scope):
			// Handle nested function calls: pass returned values as arguments
			numOut := c.child[0].typ.rtype.NumOut()
			for j := 0; j < numOut; j++ {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return valueInterfaceValue(f.data[ind]) })
			}
		case isRegularCall(c):
			// Handle nested function calls: pass returned values as arguments
			for j := range c.child[0].typ.ret {
				ind := c.findex + j
				values = append(values, func(f *frame) reflect.Value { return valueInterfaceValue(f.data[ind]) })
			}
		default:
			if c.kind == basicLit || c.rval.IsValid() {
				// Convert literal value (untyped) to function argument type (if not an interface{})
				var argType reflect.Type
				if variadic >= 0 && i+rcvrOffset >= variadic {
					argType = funcType.In(variadic).Elem()
				} else {
					argType = funcType.In(i + rcvrOffset)
				}
				convertLiteralValue(c, argType)
				if !reflect.ValueOf(c.val).IsValid() { //  Handle "nil"
					c.val = reflect.Zero(argType)
				}
			}

			if wt != nil && isInterfaceSrc(wt.arg[i]) {
				values = append(values, genValueInterface(c))
				break
			}

			// defType is the target type for a potential interface wrapper.
			var defType reflect.Type
			if variadic >= 0 && i+rcvrOffset >= variadic {
				defType = funcType.In(variadic)
			} else {
				defType = funcType.In(rcvrOffset + i)
			}
			// The interface wrapper substitution is only valid for empty
			// interface arguments (including elements of a variadic empty
			// interface parameter): a wrapper type of another interface
			// would not match a non empty interface parameter at call time.
			if getMapType != nil && (defType == emptyInterfaceType ||
				(defType.Kind() == reflect.Slice && defType.Elem() == emptyInterfaceType)) {
				if rt := getMapType(c.typ); rt != nil {
					defType = rt
				}
			}

			// When the target parameter is an empty interface (and not a
			// variadic slice of empty interfaces), pass the concrete value to
			// the binary function rather than an interpreter interface wrapper,
			// so binary code does not reflect over wrapper structs (see for
			// example json.Marshal). Wrappers are preserved for non-empty
			// interface parameters, where method dispatch matters, and for
			// variadic parameters, where values are still re-wrapped at run
			// time if necessary (see getBinValue).
			unwrap := defType.Kind() == reflect.Interface && defType.NumMethod() == 0

			switch {
			case isEmptyInterface(c.typ):
				if unwrap {
					values = append(values, genValueConcrete(getMapType, c))
					break
				}
				values = append(values, genValue(c))
			case isInterfaceSrc(c.typ):
				values = append(values, genValueInterfaceValue(c))
			case isFuncSrc(c.typ):
				values = append(values, genFunctionWrapper(c))
			case c.typ.cat == arrayT || c.typ.cat == variadicT:
				if isEmptyInterface(c.typ.val) {
					values = append(values, genValueArray(c))
				} else {
					values = append(values, genInterfaceWrapper(c, defType))
				}
			case isPtrSrc(c.typ):
				if c.typ.val.cat == valueT {
					values = append(values, genValue(c))
				} else {
					values = append(values, genInterfaceWrapper(c, defType))
				}
			case c.typ.cat == valueT:
				if unwrap {
					values = append(values, genValueConcrete(getMapType, c))
					break
				}
				values = append(values, genValue(c))
			default:
				if unwrap {
					values = append(values, genValueConcrete(getMapType, c))
					break
				}
				values = append(values, genInterfaceWrapper(c, defType))
			}
		}
	}
	useCallArgSnapshots(n, values)
	l := len(values)

	switch {
	case n.anc.kind == deferStmt:
		// Store function call in frame for deferred execution.
		n.exec = func(f *frame) bltn {
			binder := newCallOwnerBinder(f, nil)
			val := make([]reflect.Value, l+1)
			val[0] = value(f)
			if _, interpreted := f.interp.interpretedFunc(val[0]); interpreted {
				binder.bindArgs = false
			}
			for i, v := range values {
				val[i+1] = binder.argument(getBinValue(getMapType, v, f))
			}
			markNativeCallOwnedValues(f, binder, val[0], val[1:])
			f.deferred = append([]deferredCall{{values: val}}, f.deferred...)
			return tnext
		}
	case n.anc.kind == goStmt:
		// Execute function in a goroutine, discard results.
		n.exec = func(f *frame) bltn {
			binder := newCallOwnerBinder(f, f.cancel)
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = binder.argument(getBinValue(getMapType, v, f))
			}
			target := activeValue(f, binder)
			markNativeCallOwnedValues(f, binder, target, in)
			go func() {
				callFn(target, in)
				// Eval may have ended before this discarded call. Sweep once
				// its native result boundary is complete.
				f.interp.sweepRootInterpretedFuncs(f.root, reflect.Value{})
			}()
			return tnext
		}
	case fnext != nil:
		// Handle branching according to boolean result.
		index := n.findex
		level := n.level
		n.exec = func(f *frame) bltn {
			binder := newCallOwnerBinder(f, f.cancel)
			in := make([]reflect.Value, l)
			for i, v := range values {
				in[i] = binder.argument(getBinValue(getMapType, v, f))
			}
			target := activeValue(f, binder)
			markNativeCallOwnedValues(f, binder, target, in)
			res := callWithFuncSweepFenceReleased(f, func() []reflect.Value { return callFn(target, in) })
			if binder.bindArgs {
				res = activateDirectFuncResultsFromExec(f, res)
				f.interp.registerNativeResultValuesFromExec(f, res...)
			}
			if f.canceled() {
				return nil
			}
			b := res[0].Bool()
			getFrame(f, level).data[index].SetBool(b)
			if b {
				return tnext
			}
			return fnext
		}
	default:
		switch n.anc.action {
		case aAssignX:
			// The function call is part of an assign expression, store results direcly
			// to assigned location, to avoid an additional frame copy.
			// The optimization of aAssign is handled in assign(), and should not
			// be handled here.
			rvalues := make([]func(*frame) reflect.Value, funcType.NumOut())
			for i := range rvalues {
				c := n.anc.child[i]
				if c.ident == "_" {
					continue
				}
				if isInterfaceSrc(c.typ) {
					rvalues[i] = genValueInterfaceValue(c)
				} else {
					rvalues[i] = genValue(c)
				}
			}
			n.exec = func(f *frame) bltn {
				binder := newCallOwnerBinder(f, f.cancel)
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = binder.argument(getBinValue(getMapType, v, f))
				}
				target := activeValue(f, binder)
				markNativeCallOwnedValues(f, binder, target, in)
				out := callWithFuncSweepFenceReleased(f, func() []reflect.Value { return callFn(target, in) })
				if binder.bindArgs {
					out = activateDirectFuncResultsFromExec(f, out)
					f.interp.registerNativeResultValuesFromExec(f, out...)
				}
				if f.canceled() {
					return nil
				}
				for i, v := range rvalues {
					if v == nil {
						continue // Skip assign "_".
					}
					c := n.anc.child[i]
					if n.anc.kind == defineXStmt && !c.redeclared {
						// In case of a define statement, the destination value in the frame
						// must be recreated. This is necessary to preserve the previous value
						// which may be still used in a separate closure.
						data := getFrame(f, c.level).data
						data[c.findex] = reflect.New(data[c.findex].Type()).Elem()
						data[c.findex].Set(out[i])
						continue
					}
					destination := v(f)
					f.interp.markOwnedCellWriteFromExec(destination, out[i])
					destination.Set(out[i])
				}
				return tnext
			}
		case aReturn:
			// The function call is part of a return statement, store output results
			// directly in the frame location of outputs of the current function.
			b := childPos(n)
			n.exec = func(f *frame) bltn {
				binder := newCallOwnerBinder(f, f.cancel)
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = binder.argument(getBinValue(getMapType, v, f))
				}
				target := activeValue(f, binder)
				markNativeCallOwnedValues(f, binder, target, in)
				out := callWithFuncSweepFenceReleased(f, func() []reflect.Value { return callFn(target, in) })
				if binder.bindArgs {
					out = activateDirectFuncResultsFromExec(f, out)
					f.interp.registerNativeResultValuesFromExec(f, out...)
				}
				if f.canceled() {
					return nil
				}
				for i, v := range out {
					dest := f.data[b+i]
					if _, ok := dest.Interface().(valueInterface); ok {
						v = reflect.ValueOf(valueInterface{value: v})
					}
					dest.Set(v)
				}
				return tnext
			}
		default:
			n.exec = func(f *frame) bltn {
				binder := newCallOwnerBinder(f, f.cancel)
				in := make([]reflect.Value, l)
				for i, v := range values {
					in[i] = binder.argument(getBinValue(getMapType, v, f))
				}
				target := activeValue(f, binder)
				markNativeCallOwnedValues(f, binder, target, in)
				out := callWithFuncSweepFenceReleased(f, func() []reflect.Value { return callFn(target, in) })
				if binder.bindArgs {
					out = activateDirectFuncResultsFromExec(f, out)
					f.interp.registerNativeResultValuesFromExec(f, out...)
				}
				if f.canceled() {
					return nil
				}
				for i := 0; i < len(out); i++ {
					r := out[i]
					if r.Kind() == reflect.Func {
						getFrame(f, n.level).data[n.findex+i] = r
						continue
					}
					dest := getFrame(f, n.level).data[n.findex+i]
					if _, ok := dest.Interface().(valueInterface); ok {
						r = reflect.ValueOf(valueInterface{value: r})
					}
					f.interp.markOwnedCellWriteFromExec(dest, r)
					dest.Set(r)
				}
				return tnext
			}
		}
	}
}

func getIndexBinMethod(n *node) {
	// dest := genValue(n)
	i := n.findex
	l := n.level
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		// dest(f).Set(value(f).Method(m))
		receiver := cloneCallArgValue(value(f))
		f.interp.markOwnedValuesHostSharedFromExec(receiver)
		getFrame(f, l).data[i] = receiver.Method(m)
		return next
	}
}

func getIndexBinElemMethod(n *node) {
	i := n.findex
	l := n.level
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		receiver := cloneCallArgValue(value(f).Elem())
		f.interp.markOwnedValuesHostSharedFromExec(receiver)
		getFrame(f, l).data[i] = receiver.Method(m)
		return next
	}
}

func getIndexBinPtrMethod(n *node) {
	i := n.findex
	l := n.level
	m := n.val.(int)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		// Can not use .Set() because dest type contains the receiver and source not
		receiver := value(f).Addr()
		f.interp.markOwnedValuesHostSharedFromExec(receiver)
		getFrame(f, l).data[i] = receiver.Method(m)
		return next
	}
}

// getIndexArray returns array value from index.
func getIndexArray(n *node) {
	tnext := getExec(n.tnext)
	value0 := genValueArray(n.child[0]) // array
	i := n.findex
	l := n.level

	if n.child[1].rval.IsValid() { // constant array index
		ai := int(vInt(n.child[1].rval))
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				r := value0(f).Index(ai)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value0(f).Index(ai)
				return tnext
			}
		}
	} else {
		value1 := genValueInt(n.child[1]) // array index

		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				r := value0(f).Index(int(vi))
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				_, vi := value1(f)
				getFrame(f, l).data[i] = value0(f).Index(int(vi))
				return tnext
			}
		}
	}
}

// getIndexMap retrieves map value from index.
func getIndexMap(n *node) {
	dest := genValue(n)
	value0 := genValue(n.child[0]) // map
	tnext := getExec(n.tnext)
	z := reflect.New(n.child[0].typ.frameType().Elem()).Elem()

	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval

		switch {
		case n.fnext != nil:
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() && v.Bool() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).Set(z)
				return fnext
			}
		default:
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(mi); v.IsValid() {
					dest(f).Set(v)
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		}
	} else {
		value1 := genValue(n.child[1]) // map index

		switch {
		case n.fnext != nil:
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() && v.Bool() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).Set(z)
				return fnext
			}
		default:
			n.exec = func(f *frame) bltn {
				if v := value0(f).MapIndex(value1(f)); v.IsValid() {
					dest(f).Set(v)
				} else {
					dest(f).Set(z)
				}
				return tnext
			}
		}
	}
}

// getIndexMap2 retrieves map value from index and set status.
func getIndexMap2(n *node) {
	dest := genValue(n.anc.child[0])   // result
	value0 := genValue(n.child[0])     // map
	value2 := genValue(n.anc.child[1]) // status
	next := getExec(n.tnext)
	doValue := n.anc.child[0].ident != "_"
	doStatus := n.anc.child[1].ident != "_"

	if !doValue && !doStatus {
		nop(n)
		return
	}
	if n.child[1].rval.IsValid() { // constant map index
		mi := n.child[1].rval
		switch {
		case !doValue:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(mi)
				value2(f).SetBool(v.IsValid())
				return next
			}
		default:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(mi)
				destination := dest(f)
				if v.IsValid() {
					f.interp.markOwnedCellWriteFromExec(destination, v)
					destination.Set(v)
				} else {
					zero := reflect.Zero(destination.Type())
					f.interp.markOwnedCellWriteFromExec(destination, zero)
					destination.Set(zero)
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		}
	} else {
		value1 := genValue(n.child[1]) // map index
		switch {
		case !doValue:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(value1(f))
				value2(f).SetBool(v.IsValid())
				return next
			}
		default:
			n.exec = func(f *frame) bltn {
				v := value0(f).MapIndex(value1(f))
				destination := dest(f)
				if v.IsValid() {
					f.interp.markOwnedCellWriteFromExec(destination, v)
					destination.Set(v)
				} else {
					zero := reflect.Zero(destination.Type())
					f.interp.markOwnedCellWriteFromExec(destination, zero)
					destination.Set(zero)
				}
				if doStatus {
					value2(f).SetBool(v.IsValid())
				}
				return next
			}
		}
	}
}

// getFunc compiles a closure function generator for anonymous functions.
func getFunc(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)
	captureRefs := interpretedFuncCaptureRefs(n)

	n.exec = func(f *frame) bltn {
		fr := f.clone()
		slotOwner := getFrame(f, l)
		o := slotOwner.data[i]
		build := buildClosureWrapper(n, fr, slotOwner, i, o, f.root, f.cancel, captureRefs)
		fct := build.value
		n.interp.registerInterpretedFuncWithRebinder(fct, build.invoke, build.rebind, f, build.captures)
		if ref, ok := funcvalRefOf(fct); ok {
			n.interp.funcMu.Lock()
			fr.funcCarrier = ref.key
			n.interp.funcMu.Unlock()
		}

		f.mutex.Lock()
		slotOwner.data[i] = fct
		// Keep the slot's pre-execution value for the exit-time restore, so a
		// finished frame does not pin the wrapper in its slot. Root-frame slots
		// are durable REPL storage, never restored.
		if slotOwner != slotOwner.root {
			recordFuncSlotRestoreLocked(f, slotOwner, i, o)
		}
		f.mutex.Unlock()

		return next
	}
}

// frameSharesSlotsWith reports whether the active activation a can reach
// frames whose cells are shared with f: f itself (through ancestry), or a
// lexical clone whose header copies f's cells (cloneOf == f). False
// positives are safe: they only skip a slot restore, leaving the entry to
// the sweep and purge reclamation paths.
func frameSharesSlotsWith(a, f *frame) bool {
	for current := a; current != nil; current = current.anc {
		if current == f || current.cloneOf == f {
			return true
		}
	}
	return false
}

// recordFuncSlotRestoreLocked remembers the restore for a literal slot, once
// per (slot, frame): later executions of the same literal overwrite the slot
// but must not displace the original restore value. First-wins is sound
// because nothing reads a literal slot between a second getFunc execution and
// frame exit — every later read of the wrapper goes through the value
// captured at its creation, and the exit restore only serves the registry's
// weak eviction. The caller holds f.mutex.
func recordFuncSlotRestoreLocked(f, slotOwner *frame, index int, value reflect.Value) {
	for _, existing := range f.funcSlots {
		if existing.owner == slotOwner && existing.index == index {
			return
		}
	}
	f.funcSlots = append(f.funcSlots, funcSlotRestore{owner: slotOwner, index: index, value: value})
}

// applyFuncSlotRestores reverts every literal slot recorded by this frame. It
// runs when the frame's last runCfg activation exits: the frame is done
// executing, so its slots must no longer pin wrappers against the registry's
// weak eviction.
func applyFuncSlotRestores(f *frame) {
	f.mutex.Lock()
	restores := f.funcSlots
	f.funcSlots = nil
	f.mutex.Unlock()
	for _, restore := range restores {
		restore.owner.mutex.Lock()
		restore.owner.data[restore.index] = restore.value
		restore.owner.mutex.Unlock()
	}
}

func buildClosureWrapper(n *node, captured, slotOwner *frame, slotIndex int, restore reflect.Value, root *frame, cancel <-chan struct{}, captureRefs []funcMetaCaptureRef) interpretedFuncBuild {
	numRet := len(n.typ.ret)
	invoke := func(in []reflect.Value, root *frame, cancel <-chan struct{}) []reflect.Value {
		// Allocate and init local frame. All values to be settable and addressable.
		fr2 := newFrame(captured, len(n.types), captured.runid())
		// Preserve captured lexical ancestors, but resolve package globals
		// against the activation invoking the closure. A canceled activation's
		// root may have been detached and replaced by a later Eval.
		if root != nil {
			fr2.root = root
		}
		fr2.cancel = cancel
		fr2.done = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(cancel)}
		d := fr2.data
		for i, t := range n.types {
			d[i] = reflect.New(t).Elem()
		}
		d = d[numRet:]

		// Copy function input arguments in local frame.
		for i, arg := range in {
			if i >= len(d) {
				// In case of unused arg, there may be not even a frame entry allocated, just skip.
				break
			}
			typ := n.typ.arg[i]
			switch {
			case isEmptyInterface(typ) || typ.TypeOf() == valueInterfaceType:
				d[i].Set(arg)
			case isInterfaceSrc(typ):
				d[i].Set(reflect.ValueOf(valueInterface{value: arg.Elem()}))
			default:
				d[i].Set(arg)
			}
		}

		// Interpreter code execution.
		runCfg(n.child[3].start, fr2, n, n)

		if !fr2.canceled() {
			slotOwner.mutex.Lock()
			slotOwner.data[slotIndex] = restore
			slotOwner.mutex.Unlock()
		}

		return fr2.data[:numRet]
	}
	boundRoot, boundCancel := root, cancel
	wrapper := makeInterpretedHostWrapper(n.interp, n.typ.TypeOf(), invoke, boundRoot, boundCancel)
	rebind := func(c *detachedRootCloner) interpretedFuncBuild {
		clonedCaptured := c.cloneFrame(captured)
		clonedSlotOwner := c.cloneFrame(slotOwner)
		clonedRestore := restore
		if restore.IsValid() {
			clonedRestore = c.cloneValue(restore, true)
		}
		build := buildClosureWrapper(n, clonedCaptured, clonedSlotOwner, slotIndex, clonedRestore, c.newRoot, c.cancel, captureRefs)
		clonedCaptured.funcCarrier, _ = funcvalKeyOf(build.value)
		return build
	}
	return interpretedFuncBuild{
		value: wrapper, invoke: invoke, rebind: rebind,
		captures: resolveInterpretedFuncCaptures(captured, captureRefs),
	}
}

func getMethod(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		nod := *n.val.(*node)
		nod.val = &nod
		nod.recv = n.recv
		getFrame(f, l).data[i] = genFuncValue(&nod)(f)
		return next
	}
}

// getMethodExpr generates the func value of a method expression on an
// interpreted type (for example (*T).M). The value is a func taking the
// receiver as first argument, not a method bound to a receiver (#1499).
func getMethodExpr(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		nod := *n.val.(*node)
		nod.val = &nod
		// Give the method copy the method expression type, where the receiver
		// is the first argument, and keep the unbound receiver marker, so the
		// generated wrapper reads the receiver from the first call argument.
		nod.typ = n.typ
		nod.recv = n.recv
		getFrame(f, l).data[i] = genFuncValue(&nod)(f)
		return next
	}
}

func getMethodByName(n *node) {
	next := getExec(n.tnext)
	value0 := genValue(n.child[0])
	name := n.child[1].ident
	i := n.findex
	l := n.level

	n.exec = func(f *frame) bltn {
		// The interface object must be directly accessible, or embedded in a struct (exported anonymous field).
		val0 := value0(f)
		val, ok := value0(f).Interface().(valueInterface)
		if !ok {
			// Search the first embedded valueInterface.
			for val0.Kind() == reflect.Ptr {
				val0 = val0.Elem()
			}
			for i := 0; i < val0.NumField(); i++ {
				fld := val0.Type().Field(i)
				if !fld.Anonymous || !fld.IsExported() {
					continue
				}
				if val, ok = val0.Field(i).Interface().(valueInterface); ok {
					break
					// TODO: should we keep track of all the vals that are indeed valueInterface,
					// so that later on we can call MethodByName on all of them until one matches?
				}
			}
			if !ok {
				panic(n.cfgErrorf("invalid interface value %v", val0))
			}
		}
		// Traverse nested interface values to get the concrete value.
		for {
			v, ok := val.value.Interface().(valueInterface)
			if !ok {
				break
			}
			val = v
		}

		if met := val.value.MethodByName(name); met.IsValid() {
			getFrame(f, l).data[i] = met
			return next
		}

		typ := val.node.typ
		if typ.node == nil && typ.cat == valueT {
			// It happens with a var of empty interface type, that has value of concrete type
			// from runtime, being asserted to "user-defined" interface.
			if _, ok := typ.rtype.MethodByName(name); !ok {
				panic(n.cfgErrorf("method not found: %s", name))
			}
			return next
		}

		// Finally search method recursively in embedded valueInterfaces.
		r, m, li := lookupMethodValue(val, name)
		if r.IsValid() {
			getFrame(f, l).data[i] = r
			return next
		}
		if m == nil {
			panic(n.cfgErrorf("method not found: %s", name))
		}

		nod := *m
		nod.val = &nod
		nod.recv = &receiver{nil, val.value, li}
		getFrame(f, l).data[i] = genFuncValue(&nod)(f)
		return next
	}
}

// lookupMethodValue recursively looks within val for the method with the given
// name. If a runtime value is found, it is returned in r, otherwise it is returned
// in m, with li as the list of recursive field indexes.
func lookupMethodValue(val valueInterface, name string) (r reflect.Value, m *node, li []int) {
	if r = val.value.MethodByName(name); r.IsValid() {
		return
	}
	if m, li = val.node.typ.lookupMethod(name); m != nil {
		return
	}
	if !isStruct(val.node.typ) {
		return
	}
	v := val.value
	for v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	nf := v.NumField()
	for i := 0; i < nf; i++ {
		vi, ok := v.Field(i).Interface().(valueInterface)
		if !ok {
			continue
		}
		if r, m, li = lookupMethodValue(vi, name); m != nil {
			li = append([]int{i}, li...)
			return
		}
	}
	return
}

func getIndexSeq(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	tnext := getExec(n.tnext)
	i := n.findex
	l := n.level

	// Note:
	// Here we have to store the result using
	//    f.data[i] = value(...)
	// instead of normal
	//    dest(f).Set(value(...)
	// because the value returned by FieldByIndex() must be preserved
	// for possible future Set operations on the struct field (avoid a
	// dereference from Set, resulting in setting a copy of the
	// original field).

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			v := value(f)
			r := v.FieldByIndex(index)
			getFrame(f, l).data[i] = r
			if r.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			v := value(f)
			getFrame(f, l).data[i] = v.FieldByIndex(index)
			return tnext
		}
	}
}

func getPtrIndexSeq(n *node) {
	index := n.val.([]int)
	tnext := getExec(n.tnext)
	value := genValue(n.child[0])
	i := n.findex
	l := n.level

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			r := value(f).Elem().FieldByIndex(index)
			getFrame(f, l).data[i] = r
			if r.Bool() {
				return tnext
			}
			return fnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(index)
			return tnext
		}
	}
}

func getIndexSeqField(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	i := n.findex
	l := n.level
	tnext := getExec(n.tnext)

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
			n.exec = func(f *frame) bltn {
				r := value(f).Elem().FieldByIndex(index)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				r := value(f).FieldByIndex(index)
				getFrame(f, l).data[i] = r
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		}
	} else {
		if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(index)
				return tnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).FieldByIndex(index)
				return tnext
			}
		}
	}
}

func getIndexSeqPtrMethod(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	fi := index[1:]
	mi := index[0]
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		if len(fi) == 0 {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Method(mi)
				return next
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(fi).Addr().Method(mi)
				return next
			}
		}
	} else {
		if len(fi) == 0 {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Addr().Method(mi)
				return next
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).FieldByIndex(fi).Addr().Method(mi)
				return next
			}
		}
	}
}

func getIndexSeqMethod(n *node) {
	value := genValue(n.child[0])
	index := n.val.([]int)
	fi := index[1:]
	mi := index[0]
	i := n.findex
	l := n.level
	next := getExec(n.tnext)

	if n.child[0].typ.TypeOf().Kind() == reflect.Ptr {
		if len(fi) == 0 {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Elem().Method(mi)
				return next
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Elem().FieldByIndex(fi).Method(mi)
				return next
			}
		}
	} else {
		if len(fi) == 0 {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).Method(mi)
				return next
			}
		} else {
			n.exec = func(f *frame) bltn {
				getFrame(f, l).data[i] = value(f).FieldByIndex(fi).Method(mi)
				return next
			}
		}
	}
}

func neg(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface

	switch n.typ.TypeOf().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(-value(f).Int()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetInt(-value(f).Int())
			return next
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(-value(f).Uint()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetUint(-value(f).Uint())
			return next
		}
	case reflect.Float32, reflect.Float64:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(-value(f).Float()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetFloat(-value(f).Float())
			return next
		}
	case reflect.Complex64, reflect.Complex128:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(-value(f).Complex()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetComplex(-value(f).Complex())
			return next
		}
	}
}

func pos(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		dest(f).Set(value(f))
		return next
	}
}

func bitNot(n *node) {
	dest := genValue(n)
	value := genValue(n.child[0])
	next := getExec(n.tnext)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface

	switch typ.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(^value(f).Int()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetInt(^value(f).Int())
			return next
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if isInterface {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(^value(f).Uint()).Convert(typ))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetUint(^value(f).Uint())
			return next
		}
	}
}

func land(n *node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)
	dest := genValue(n)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value0(f).Bool() && value1(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
		return
	}
	if isInterface {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(value0(f).Bool() && value1(f).Bool()).Convert(typ))
			return tnext
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetBool(value0(f).Bool() && value1(f).Bool())
		return tnext
	}
}

func lor(n *node) {
	value0 := genValue(n.child[0])
	value1 := genValue(n.child[1])
	tnext := getExec(n.tnext)
	dest := genValue(n)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface

	if n.fnext != nil {
		fnext := getExec(n.fnext)
		n.exec = func(f *frame) bltn {
			if value0(f).Bool() || value1(f).Bool() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
		return
	}
	if isInterface {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(value0(f).Bool() || value1(f).Bool()).Convert(typ))
			return tnext
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetBool(value0(f).Bool() || value1(f).Bool())
		return tnext
	}
}

func nop(n *node) {
	next := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		return next
	}
}

func branch(n *node) {
	tnext := getExec(n.tnext)
	fnext := getExec(n.fnext)
	value := genValue(n)

	n.exec = func(f *frame) bltn {
		if value(f).Bool() {
			return tnext
		}
		return fnext
	}
}

func _return(n *node) {
	child := n.child
	def := n.val.(*node)
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		switch t := def.typ.ret[i]; t.cat {
		case errorT:
			values[i] = genInterfaceWrapper(c, t.TypeOf())
		case funcT:
			values[i] = genValue(c)
		case valueT:
			switch t.rtype.Kind() {
			case reflect.Interface:
				values[i] = genInterfaceWrapper(c, t.TypeOf())
				continue
			case reflect.Func:
				values[i] = genFunctionWrapper(c)
				continue
			}
			fallthrough
		default:
			switch {
			case isInterfaceSrc(t):
				if len(t.field) == 0 {
					// empty interface case.
					// we can't let genValueInterface deal with it, because we call on c,
					// not on n, which means that the interfaceT knowledge is lost.
					values[i] = genValue(c)
					break
				}
				values[i] = genValueInterface(c)
			case c.typ.untyped:
				values[i] = genValueAs(c, t.TypeOf())
			default:
				values[i] = genValue(c)
			}
		}
	}

	switch len(child) {
	case 0:
		n.exec = nil
	case 1:
		switch {
		case !child[0].rval.IsValid() && child[0].kind == binaryExpr:
			// No additional runtime operation is necessary for constants (not in frame) or
			// binary expressions (stored directly at the right location in frame).
			n.exec = nil
		case isCall(child[0]) && n.child[0].typ.id() == def.typ.ret[0].id():
			// Calls are optmized as long as no type conversion is involved.
			n.exec = nil
		default:
			// Regular return: store the value to return at to start of the frame.
			v := values[0]
			n.exec = func(f *frame) bltn {
				f.data[0].Set(v(f))
				return nil
			}
		}
	case 2:
		v0, v1 := values[0], values[1]
		n.exec = func(f *frame) bltn {
			f.data[0].Set(v0(f))
			f.data[1].Set(v1(f))
			return nil
		}
	default:
		n.exec = func(f *frame) bltn {
			for i, value := range values {
				f.data[i].Set(value(f))
			}
			return nil
		}
	}
}

func arrayLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if n.nleft == 1 {
		child = n.child[1:]
	}

	values := make([]func(*frame) reflect.Value, len(child))
	index := make([]int, len(child))
	var max, prev int

	ntyp := n.typ.resolveAlias()
	for i, c := range child {
		if c.kind == keyValueExpr {
			values[i] = genDestValue(ntyp.val, c.child[1])
			index[i] = int(vInt(c.child[0].rval))
		} else {
			values[i] = genDestValue(ntyp.val, c)
			index[i] = prev
		}
		prev = index[i] + 1
		if prev > max {
			max = prev
		}
	}

	typ := n.typ.frameType()
	kind := typ.Kind()
	n.exec = func(f *frame) bltn {
		var a reflect.Value
		if kind == reflect.Slice {
			a = reflect.MakeSlice(typ, max, max)
			f.interp.registerOwnedValue(a, f)
		} else {
			a, _ = n.typ.zero()
		}
		for i, v := range values {
			a.Index(index[i]).Set(v(f))
		}
		setOwnedValueOutput(f, value(f), a)
		return next
	}
}

func mapLit(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if n.nleft == 1 {
		child = n.child[1:]
	}
	typ := n.typ.frameType()
	keys := make([]func(*frame) reflect.Value, len(child))
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		keys[i] = genDestValue(n.typ.key, c.child[0])
		values[i] = genDestValue(n.typ.val, c.child[1])
	}

	n.exec = func(f *frame) bltn {
		m := reflect.MakeMap(typ)
		f.interp.registerOwnedValue(m, f)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		setOwnedValueOutput(f, value(f), m)
		return next
	}
}

func compositeBinMap(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if n.nleft == 1 {
		child = n.child[1:]
	}
	typ := n.typ.frameType()
	keys := make([]func(*frame) reflect.Value, len(child))
	values := make([]func(*frame) reflect.Value, len(child))
	for i, c := range child {
		convertLiteralValue(c.child[0], typ.Key())
		convertLiteralValue(c.child[1], typ.Elem())
		keys[i] = genValue(c.child[0])

		if isFuncSrc(c.child[1].typ) {
			values[i] = genFunctionWrapper(c.child[1])
		} else {
			values[i] = genValue(c.child[1])
		}
	}

	n.exec = func(f *frame) bltn {
		m := reflect.MakeMap(typ)
		f.interp.registerOwnedValue(m, f)
		for i, k := range keys {
			m.SetMapIndex(k(f), values[i](f))
		}
		setOwnedValueOutput(f, value(f), m)
		return next
	}
}

func compositeBinSlice(n *node) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	child := n.child
	if n.nleft == 1 {
		child = n.child[1:]
	}

	values := make([]func(*frame) reflect.Value, len(child))
	index := make([]int, len(child))
	rtype := n.typ.rtype.Elem()
	var max, prev int

	for i, c := range child {
		if c.kind == keyValueExpr {
			convertLiteralValue(c.child[1], rtype)
			values[i] = genValue(c.child[1])
			index[i] = int(vInt(c.child[0].rval))
		} else {
			convertLiteralValue(c, rtype)
			values[i] = genValue(c)
			index[i] = prev
		}
		prev = index[i] + 1
		if prev > max {
			max = prev
		}
	}

	typ := n.typ.frameType()
	kind := typ.Kind()
	n.exec = func(f *frame) bltn {
		var a reflect.Value
		if kind == reflect.Slice {
			a = reflect.MakeSlice(typ, max, max)
			f.interp.registerOwnedValue(a, f)
		} else {
			a, _ = n.typ.zero()
		}
		for i, v := range values {
			a.Index(index[i]).Set(v(f))
		}
		setOwnedValueOutput(f, value(f), a)
		return next
	}
}

// doCompositeBinStruct creates and populates a struct object from a binary type.
func doCompositeBinStruct(n *node, hasType bool) {
	next := getExec(n.tnext)
	value := valueGenerator(n, n.findex)
	typ := baseType(n.typ).rtype
	child := n.child
	if hasType {
		child = n.child[1:]
	}
	values := make([]func(*frame) reflect.Value, len(child))
	fieldIndex := make([][]int, len(child))
	for i, c := range child {
		if c.kind == keyValueExpr {
			if sf, ok := typ.FieldByName(c.child[0].ident); ok {
				fieldIndex[i] = sf.Index
				convertLiteralValue(c.child[1], sf.Type)
				if isFuncSrc(c.child[1].typ) {
					values[i] = genFunctionWrapper(c.child[1])
				} else {
					values[i] = genValue(c.child[1])
				}
			}
		} else {
			fieldIndex[i] = []int{i}
			if isFuncSrc(c.typ) && len(c.child) > 1 {
				convertLiteralValue(c.child[1], typ.Field(i).Type)
				values[i] = genFunctionWrapper(c.child[1])
			} else {
				convertLiteralValue(c, typ.Field(i).Type)
				values[i] = genValue(c)
			}
		}
	}

	frameIndex := n.findex
	l := n.level

	n.exec = func(f *frame) bltn {
		s := reflect.New(typ).Elem()
		for i, v := range values {
			s.FieldByIndex(fieldIndex[i]).Set(v(f))
		}
		d := value(f)
		switch {
		case d.Kind() == reflect.Ptr:
			setOwnedValueOutput(f, d, s.Addr())
		default:
			if n.anc.kind == assignStmt && n.anc.action == aAssign {
				setOwnedValueOutput(f, d, s)
			} else {
				getFrame(f, l).data[frameIndex] = s
			}
		}
		return next
	}
}

func compositeBinStruct(n *node)       { doCompositeBinStruct(n, true) }
func compositeBinStructNotype(n *node) { doCompositeBinStruct(n, false) }

func destType(n *node) *itype {
	switch n.anc.kind {
	case assignStmt, defineStmt:
		return n.anc.child[0].typ
	default:
		return n.typ
	}
}

func doComposite(n *node, hasType bool, keyed bool) {
	value := valueGenerator(n, n.findex)
	next := getExec(n.tnext)
	typ := baseType(n.typ)
	child := n.child
	if hasType {
		child = n.child[1:]
	}
	destInterface := isInterfaceSrc(destType(n))

	values := make(map[int]func(*frame) reflect.Value)
	for i, c := range child {
		var val *node
		var fieldIndex int
		if keyed {
			val = c.child[1]
			fieldIndex = typ.fieldIndex(c.child[0].ident)
		} else {
			val = c
			fieldIndex = i
		}
		ft := typ.field[fieldIndex].typ
		rft := ft.TypeOf()
		convertLiteralValue(val, rft)
		switch {
		case val.typ.cat == nilT:
			values[fieldIndex] = func(*frame) reflect.Value { return reflect.New(rft).Elem() }
		case isNamedFuncSrc(val.typ):
			values[fieldIndex] = genValueAsFunctionWrapper(val)
		case isInterfaceSrc(ft) && (!isEmptyInterface(ft) || len(val.typ.method) > 0):
			values[fieldIndex] = genValueInterface(val)
		case isInterface(ft):
			values[fieldIndex] = genInterfaceWrapper(val, rft)
		default:
			values[fieldIndex] = genValue(val)
		}
	}

	frameIndex := n.findex
	l := n.level
	rt := typ.TypeOf()

	n.exec = func(f *frame) bltn {
		a := reflect.New(rt).Elem()
		for i, v := range values {
			a.Field(i).Set(v(f))
		}
		d := value(f)
		switch {
		case d.Kind() == reflect.Ptr:
			setOwnedValueOutput(f, d, a.Addr())
		case destInterface && len(destType(n).field) > 0 && d.Type() == valueInterfaceType:
			// The enclosing assignment was inlined into this composite
			// literal: the destination cell is frame-typed as valueInterface,
			// so store the interpreted interface representation directly.
			setOwnedValueOutput(f, d, reflect.ValueOf(valueInterface{n, a}))
		case destInterface:
			// The composite result goes to its own frame slot (the assignment
			// was not inlined, i.e. the destination is a struct field, a
			// dereferenced pointer or a binary interface). Keep the value
			// concrete: the valueInterface wrapper is added later by the
			// assignment itself (see genDestValue).
			setOwnedValueOutput(f, d, a)
		default:
			if n.anc.kind == assignStmt && n.anc.action == aAssign {
				setOwnedValueOutput(f, d, a)
			} else {
				getFrame(f, l).data[frameIndex] = a
			}
		}
		return next
	}
}

// doCompositeLit creates and populates a struct object.
func doCompositeLit(n *node, hasType bool) {
	doComposite(n, hasType, false)
}

func compositeLit(n *node)       { doCompositeLit(n, true) }
func compositeLitNotype(n *node) { doCompositeLit(n, false) }

// doCompositeLitKeyed creates a struct Object, filling fields from sparse key-values.
func doCompositeLitKeyed(n *node, hasType bool) {
	doComposite(n, hasType, true)
}

func compositeLitKeyed(n *node)       { doCompositeLitKeyed(n, true) }
func compositeLitKeyedNotype(n *node) { doCompositeLitKeyed(n, false) }

func empty(n *node) {}

var rat = reflect.ValueOf((*[]rune)(nil)).Type().Elem() // runes array type

func isRangeAssignTarget(target *node) bool {
	for target != nil && target.anc != nil && target.anc.kind == parenExpr {
		target = target.anc
	}
	return target != nil && target.anc != nil && target.anc.kind == rangeStmt && target.anc.meta == token.ASSIGN
}

func rangeTargetPreludeStart(target *node) *node {
	if target == nil {
		return nil
	}
	for _, child := range target.child {
		switch child.kind {
		case arrayType, chanType, chanTypeRecv, chanTypeSend, funcDecl, importDecl, mapType, basicLit, identExpr, typeDecl:
			continue
		}
		if child.start != nil {
			return child.start
		}
		return child
	}
	return nil
}

func runRangeTargetPrelude(target *node, f *frame) {
	start := rangeTargetPreludeStart(target)
	if start == nil || start == target {
		return
	}
	exec := getExec(start)
	for exec != nil && !isExecNode(target, exec) {
		exec = exec(f)
	}
}

func genRangeStore(n *node) func(*frame) func(reflect.Value) {
	if len(n.assignTmp) == 2 {
		return func(f *frame) func(reflect.Value) {
			destination := getFrame(f, n.assignTmp[1]).data[n.assignTmp[0]]
			return func(value reflect.Value) { setOwnedValueOutput(f, destination, value) }
		}
	}
	if isRangeAssignTarget(n) {
		switch n.kind {
		case parenExpr:
			if len(n.child) == 1 {
				return genRangeStore(n.child[0])
			}
		case indexExpr:
			container := genValue(n.child[0])
			index := genValue(n.child[1])
			if isMap(n.child[0].typ) {
				return func(f *frame) func(reflect.Value) {
					runRangeTargetPrelude(n, f)
					m, k := cloneCallArgValue(container(f)), cloneCallArgValue(index(f))
					return func(value reflect.Value) {
						f.interp.markOwnedWriteFromExec(m, k, value)
						m.SetMapIndex(k, value)
					}
				}
			}
			return func(f *frame) func(reflect.Value) {
				runRangeTargetPrelude(n, f)
				sequence := container(f)
				if sequence.Kind() == reflect.Ptr {
					sequence = sequence.Elem()
				}
				destination := sequence.Index(int(vInt(cloneCallArgValue(index(f)))))
				return func(value reflect.Value) { setOwnedValueOutput(f, destination, value) }
			}
		case starExpr:
			pointer := genValue(n.child[0])
			return func(f *frame) func(reflect.Value) {
				runRangeTargetPrelude(n, f)
				destination := pointer(f).Elem()
				return func(value reflect.Value) { setOwnedValueOutput(f, destination, value) }
			}
		case selectorExpr:
			if isBinPkgVar(n) {
				// The destination is the live host cell exposed as the node
				// value; the interpreter frame slot is not its storage.
				return func(f *frame) func(reflect.Value) {
					runRangeTargetPrelude(n, f)
					destination := n.val.(reflect.Value)
					return func(value reflect.Value) { setOwnedValueOutput(f, destination, value) }
				}
			}
			receiver := genValue(n.child[0])
			index, ok := n.val.([]int)
			if ok {
				return func(f *frame) func(reflect.Value) {
					runRangeTargetPrelude(n, f)
					destination := receiver(f)
					for destination.Kind() == reflect.Ptr {
						destination = destination.Elem()
					}
					destination = destination.FieldByIndex(index)
					return func(value reflect.Value) { setOwnedValueOutput(f, destination, value) }
				}
			}
			fallthrough
		default:
			destination := genValue(n)
			return func(f *frame) func(reflect.Value) {
				runRangeTargetPrelude(n, f)
				dest := destination(f)
				return func(value reflect.Value) { setOwnedValueOutput(f, dest, value) }
			}
		}
	}
	return func(f *frame) func(reflect.Value) {
		destination := f.data[n.findex]
		return func(value reflect.Value) { destination.Set(value) }
	}
}

func _range(n *node) {
	index0 := n.child[0].findex // array index location in frame
	index2 := index0 - 1        // shallow array for range, always just behind index0
	index3 := index2 - 1        // additional location to store string char position
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	var value func(*frame) reflect.Value
	var an *node
	storeIndex := genRangeStore(n.child[0])
	if len(n.child) == 4 {
		an = n.child[2]
		valueNode := n.child[1]
		storeValue := genRangeStore(valueNode)
		if isString(an.typ.TypeOf()) {
			// Special variant of "range" for string, where the index indicates the byte position
			// of the rune in the string, rather than the index of the rune in array.
			stringType := reflect.TypeOf("")
			value = genValueAs(an, rat) // range on string iterates over runes
			n.exec = func(f *frame) bltn {
				a := f.data[index2]
				v0 := f.data[index3]
				v0.SetInt(v0.Int() + 1)
				i := int(v0.Int())
				if i >= a.Len() {
					return fnext
				}
				// Compute byte position of the rune in string
				pos := a.Slice(0, i).Convert(stringType).Len()
				rangeValue := cloneCallArgValue(a.Index(i))
				setIndex, setValue := storeIndex(f), storeValue(f)
				setIndex(reflect.ValueOf(pos))
				setValue(rangeValue)
				return tnext
			}
		} else {
			value = genValueRangeArray(an)
			n.exec = func(f *frame) bltn {
				a := f.data[index2]
				v0 := f.data[index0]
				v0.SetInt(v0.Int() + 1)
				i := int(v0.Int())
				if i >= a.Len() {
					return fnext
				}
				rangeValue := cloneCallArgValue(a.Index(i))
				setIndex, setValue := storeIndex(f), storeValue(f)
				setIndex(reflect.ValueOf(i))
				setValue(rangeValue)
				return tnext
			}
		}
	} else {
		an = n.child[1]
		if isString(an.typ.TypeOf()) {
			value = genValueAs(an, rat) // range on string iterates over runes
		} else {
			value = genValueRangeArray(an)
		}
		n.exec = func(f *frame) bltn {
			v0 := f.data[index0]
			v0.SetInt(v0.Int() + 1)
			if int(v0.Int()) >= f.data[index2].Len() {
				return fnext
			}
			storeIndex(f)(reflect.ValueOf(int(v0.Int())))
			return tnext
		}
	}

	// Init sequence
	next := n.exec
	index := index0
	if isString(an.typ.TypeOf()) && len(n.child) == 4 {
		index = index3
	}
	n.child[0].exec = func(f *frame) bltn {
		f.data[index2] = value(f) // set array shallow copy for range
		f.data[index].SetInt(-1)  // assing index value
		return next
	}
}

func rangeInt(n *node) {
	ixn := n.child[0]
	index0 := ixn.findex // array index location in frame
	index2 := index0 - 1 // max
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	var value func(*frame) reflect.Value
	mxn := n.child[1]
	value = genValue(mxn)
	storeIndex := genRangeStore(ixn)
	n.exec = func(f *frame) bltn {
		rv := f.data[index0]
		rv.SetInt(rv.Int() + 1)
		if int(rv.Int()) >= int(f.data[index2].Int()) {
			return fnext
		}
		storeIndex(f)(reflect.ValueOf(int(rv.Int())))
		return tnext
	}

	// Init sequence
	next := n.exec
	index := index0
	ixn.exec = func(f *frame) bltn {
		f.data[index2] = value(f) // set max
		f.data[index].SetInt(-1)  // assing index value
		return next
	}
}

func loopVarKey(n *node) {
	ixn := n.anc.anc.child[0]
	next := getExec(n.tnext)
	n.exec = func(f *frame) bltn {
		rv := f.data[ixn.findex]
		nv := reflect.New(rv.Type()).Elem()
		nv.Set(rv)
		f.data[n.findex] = nv
		return next
	}
}

func loopVarVal(n *node) {
	vln := n.anc.anc.child[1]
	next := getExec(n.tnext)
	n.exec = func(f *frame) bltn {
		rv := f.data[vln.findex]
		nv := reflect.New(rv.Type()).Elem()
		nv.Set(rv)
		f.data[n.findex] = nv
		return next
	}
}

func loopVarFor(n *node) {
	ixn := n.anc.anc.child[0].child[0]
	next := getExec(n.tnext)
	n.exec = func(f *frame) bltn {
		fv := f.data[ixn.findex]
		nv := reflect.New(fv.Type()).Elem()
		nv.Set(fv)
		f.data[n.findex] = nv
		return next
	}
}

func rangeChan(n *node) {
	destNode := n.child[0]
	store := genRangeStore(destNode)
	value := genValue(n.child[1]) // chan
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)

	n.exec = func(f *frame) bltn {
		f.mutex.RLock()
		done := f.done
		f.mutex.RUnlock()

		channel := value(f)
		chosen, v, ok := selectWithFuncSweepFenceReleased(f, []reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: channel}})
		if chosen == 0 {
			return nil
		}
		if !ok {
			return fnext
		}
		v = f.interp.adoptInterpretedFuncValue(f, channel, v)
		store(f)(v)
		return tnext
	}
}

func rangeMap(n *node) {
	index0 := n.child[0].findex // map index location in frame
	index2 := index0 - 1        // iterator for range, always just behind index0
	fnext := getExec(n.fnext)
	tnext := getExec(n.tnext)
	value := genValue(n.child[len(n.child)-2]) // map value
	storeKey := genRangeStore(n.child[0])

	if len(n.child) == 4 && n.child[1].ident != "_" {
		valueNode := n.child[1]
		storeValue := genRangeStore(valueNode)
		n.exec = func(f *frame) bltn {
			iter := f.data[index2].Interface().(*reflect.MapIter)
			if !iter.Next() {
				return fnext
			}
			key, value := cloneCallArgValue(iter.Key()), cloneCallArgValue(iter.Value())
			setKey, setValue := storeKey(f), storeValue(f)
			setKey(key)
			setValue(value)
			return tnext
		}
	} else {
		n.exec = func(f *frame) bltn {
			iter := f.data[index2].Interface().(*reflect.MapIter)
			if !iter.Next() {
				return fnext
			}
			key := iter.Key()
			storeKey(f)(key)
			return tnext
		}
	}

	// Init sequence
	next := n.exec
	n.child[0].exec = func(f *frame) bltn {
		f.data[index2].Set(reflect.ValueOf(value(f).MapRange()))
		return next
	}
}

func _case(n *node) {
	tnext := getExec(n.tnext)

	// TODO(mpl): a lot of what is done in typeAssert should probably be redone/reused here.
	switch {
	case n.anc.anc.kind == typeSwitch:
		fnext := getExec(n.fnext)
		sn := n.anc.anc // switch node
		types := make([]*itype, len(n.child)-1)
		for i := range types {
			types[i] = n.child[i].typ
		}
		srcValue := genValue(sn.child[1].lastChild().child[0])

		if len(sn.child[1].child) != 2 {
			// no assign in switch guard
			if len(n.child) <= 1 {
				n.exec = func(f *frame) bltn { return tnext }
			} else {
				n.exec = func(f *frame) bltn {
					ival := srcValue(f).Interface()
					val, ok := ival.(valueInterface)
					// TODO(mpl): I'm assuming here that !ok means that we're dealing with the empty
					// interface case. But maybe we should make sure by checking the relevant cat
					// instead? later. Use t := v.Type(); t.Kind() == reflect.Interface , like above.
					if !ok {
						var stype string
						if ival != nil {
							stype = strings.ReplaceAll(reflect.TypeOf(ival).String(), " {}", "{}")
						}
						for _, typ := range types {
							// TODO(mpl): we should actually use canAssertTypes, but need to find a valid
							// rtype for typ. Plus we need to refactor with typeAssert().
							// weak check instead for now.
							if ival == nil {
								if typ.cat == nilT {
									return tnext
								}
								continue
							}
							if stype == typ.id() {
								return tnext
							}
						}
						return fnext
					}
					if v := val.node; v != nil {
						for _, typ := range types {
							if v.typ.id() == typ.id() {
								return tnext
							}
						}
					}
					return fnext
				}
			}
			break
		}

		// assign in switch guard
		destValue := genValue(n.lastChild().child[0])
		switch len(types) {
		case 0:
			// default clause: assign var to interface value
			n.exec = func(f *frame) bltn {
				destValue(f).Set(srcValue(f))
				return tnext
			}
		case 1:
			// match against 1 type: assign var to concrete value
			typ := types[0]
			n.exec = func(f *frame) bltn {
				v := srcValue(f)
				if !v.IsValid() {
					// match zero value against nil
					if typ.cat == nilT {
						return tnext
					}
					return fnext
				}
				if t := v.Type(); t.Kind() == reflect.Interface {
					if typ.cat == nilT && v.IsNil() {
						return tnext
					}
					rtyp := typ.TypeOf()
					if rtyp == nil {
						return fnext
					}
					elem := v.Elem()
					if rtyp.String() == t.String() && implementsInterface(v, typ) {
						destValue(f).Set(elem)
						return tnext
					}
					ival := v.Interface()
					if ival != nil && rtyp.String() == reflect.TypeOf(ival).String() {
						destValue(f).Set(elem)
						return tnext
					}
					if typ.cat == valueT && rtyp.Kind() == reflect.Interface && elem.IsValid() && elem.Type().Implements(rtyp) {
						destValue(f).Set(elem)
						return tnext
					}
					return fnext
				}
				if vi, ok := v.Interface().(valueInterface); ok {
					if vi.node != nil {
						if vi.node.typ.id() == typ.id() {
							destValue(f).Set(vi.value)
							return tnext
						}
					}
					return fnext
				}
				if v.Type() == typ.TypeOf() {
					destValue(f).Set(v)
					return tnext
				}
				return fnext
			}

		default:
			n.exec = func(f *frame) bltn {
				val := srcValue(f)
				if t := val.Type(); t.Kind() == reflect.Interface {
					for _, typ := range types {
						if typ.cat == nilT && val.IsNil() {
							return tnext
						}
						rtyp := typ.TypeOf()
						if rtyp == nil {
							continue
						}
						elem := val.Elem()
						if rtyp.String() == t.String() && implementsInterface(val, typ) {
							destValue(f).Set(elem)
							return tnext
						}
						ival := val.Interface()
						if ival != nil && rtyp.String() == reflect.TypeOf(ival).String() {
							destValue(f).Set(elem)
							return tnext
						}
						if typ.cat == valueT && rtyp.Kind() == reflect.Interface && elem.IsValid() && elem.Type().Implements(rtyp) {
							destValue(f).Set(elem)
							return tnext
						}
					}
					return fnext
				}
				if vi, ok := val.Interface().(valueInterface); ok {
					if v := vi.node; v != nil {
						for _, typ := range types {
							if v.typ.id() == typ.id() {
								destValue(f).Set(val)
								return tnext
							}
						}
					}
					return fnext
				}
				vt := val.Type()
				for _, typ := range types {
					if vt == typ.TypeOf() {
						destValue(f).Set(val)
						return tnext
					}
				}
				return fnext
			}
		}

	case len(n.child) <= 1: // default clause
		n.exec = func(f *frame) bltn { return tnext }

	default:
		fnext := getExec(n.fnext)
		l := len(n.anc.anc.child)
		value := genValue(n.anc.anc.child[l-2])
		values := make([]func(*frame) reflect.Value, len(n.child)-1)
		for i := range values {
			values[i] = genValue(n.child[i])
		}
		n.exec = func(f *frame) bltn {
			v0 := value(f)
			for _, v := range values {
				v1 := v(f)
				if !v0.Type().AssignableTo(v1.Type()) {
					v0 = v0.Convert(v1.Type())
				}
				if v0.Interface() == v1.Interface() {
					return tnext
				}
			}
			return fnext
		}
	}
}

func implementsInterface(v reflect.Value, t *itype) bool {
	rt := v.Type()
	if t.cat == valueT {
		return rt.Implements(t.rtype)
	}
	vt := &itype{cat: valueT, rtype: rt}
	if vt.methods().contains(t.methods()) {
		return true
	}
	vi, ok := v.Interface().(valueInterface)
	if !ok {
		return false
	}
	return vi.node != nil && vi.node.typ.methods().contains(t.methods())
}

func appendSlice(n *node) {
	dest := genValueOutput(n, n.typ.rtype)
	next := getExec(n.tnext)
	values := []func(*frame) reflect.Value{genValue(n.child[1]), genValue(n.child[2])}
	useCallArgSnapshots(n, values)
	value, value0 := values[0], values[1]

	if isString(n.child[2].typ.TypeOf()) {
		typ := reflect.TypeOf([]byte{})
		n.exec = func(f *frame) bltn {
			source := value(f)
			inserted := value0(f).Convert(typ)
			if source.Len()+inserted.Len() <= source.Cap() {
				f.interp.markOwnedAppendSliceWriteFromExec(source, inserted)
			}
			appended := reflect.AppendSlice(source, inserted)
			f.interp.registerOwnedAppend(appended, source, f)
			setOwnedValueOutput(f, dest(f), appended)
			return next
		}
	} else {
		n.exec = func(f *frame) bltn {
			source := value(f)
			inserted := value0(f)
			if source.Len()+inserted.Len() <= source.Cap() {
				f.interp.markOwnedAppendSliceWriteFromExec(source, inserted)
			}
			appended := reflect.AppendSlice(source, inserted)
			f.interp.registerOwnedAppend(appended, source, f)
			setOwnedValueOutput(f, dest(f), appended)
			return next
		}
	}
}

func _append(n *node) {
	// Slice expansion (appendSlice) is only valid for the ellipsis form
	// `append(s, e...)`, flagged by aCallSlice. Without ellipsis a slice or
	// array argument is appended as a single element, even when the element
	// type of the destination matches the argument type.
	if len(n.child) == 3 && n.action == aCallSlice {
		c1, c2 := n.child[1], n.child[2]
		if (c1.typ.cat == valueT || c2.typ.cat == valueT) && c1.typ.rtype == c2.typ.rtype ||
			isArray(c2.typ) && c2.typ.elem().id() == n.typ.elem().id() ||
			isByteArray(c1.typ.TypeOf()) && isString(c2.typ.TypeOf()) {
			appendSlice(n)
			return
		}
	}

	dest := genValueOutput(n, n.typ.rtype)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	switch l := len(n.child); {
	case l == 2:
		values := []func(*frame) reflect.Value{value}
		useCallArgSnapshots(n, values)
		value = values[0]
		n.exec = func(f *frame) bltn {
			setOwnedValueOutput(f, dest(f), value(f))
			return next
		}
	case l > 3:
		args := n.child[2:]
		l := len(args)
		values := make([]func(*frame) reflect.Value, l)
		for i, arg := range args {
			switch elem := n.typ.elem(); {
			case isInterfaceSrc(elem) && (!isEmptyInterface(elem) || len(arg.typ.method) > 0):
				values[i] = genValueInterface(arg)
			case isInterfaceBin(elem):
				values[i] = genInterfaceWrapper(arg, elem.rtype)
			case arg.typ.untyped:
				values[i] = genValueAs(arg, n.child[1].typ.TypeOf().Elem())
			default:
				values[i] = genValue(arg)
			}
		}
		allValues := append([]func(*frame) reflect.Value{value}, values...)
		useCallArgSnapshots(n, allValues)
		value, values = allValues[0], allValues[1:]

		n.exec = func(f *frame) bltn {
			sl := make([]reflect.Value, l)
			for i, v := range values {
				sl[i] = v(f)
			}
			source := value(f)
			if source.Len()+len(sl) <= source.Cap() {
				f.interp.markOwnedWriteFromExec(source, sl...)
			}
			appended := reflect.Append(source, sl...)
			f.interp.registerOwnedAppend(appended, source, f)
			setOwnedValueOutput(f, dest(f), appended)
			return next
		}
	default:
		var value0 func(*frame) reflect.Value
		switch elem := n.typ.elem(); {
		case isInterfaceSrc(elem) && (!isEmptyInterface(elem) || len(n.child[2].typ.method) > 0):
			value0 = genValueInterface(n.child[2])
		case isInterfaceBin(elem):
			value0 = genInterfaceWrapper(n.child[2], elem.rtype)
		case n.child[2].typ.untyped:
			value0 = genValueAs(n.child[2], n.child[1].typ.TypeOf().Elem())
		default:
			value0 = genValue(n.child[2])
		}
		values := []func(*frame) reflect.Value{value, value0}
		useCallArgSnapshots(n, values)
		value, value0 = values[0], values[1]

		n.exec = func(f *frame) bltn {
			source := value(f)
			inserted := value0(f)
			if source.Len()+1 <= source.Cap() {
				f.interp.markOwnedWriteFromExec(source, inserted)
			}
			appended := reflect.Append(source, inserted)
			f.interp.registerOwnedAppend(appended, source, f)
			setOwnedValueOutput(f, dest(f), appended)
			return next
		}
	}
}

func _cap(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(int(0)))
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	if wantEmptyInterface(n) {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(value(f).Cap()))
			return next
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(value(f).Cap()))
		return next
	}
}

func _copy(n *node) {
	in := []func(*frame) reflect.Value{genValueArray(n.child[1]), genValue(n.child[2])}
	useCallArgSnapshots(n, in)
	out := []func(*frame) reflect.Value{genValueOutput(n, reflect.TypeOf(0))}
	next := getExec(n.tnext)

	if n.anc.kind == deferStmt {
		n.exec = func(f *frame) bltn {
			args := []reflect.Value{in[0](f), in[1](f)}
			funcType := reflect.FuncOf([]reflect.Type{args[0].Type(), args[1].Type()}, []reflect.Type{reflect.TypeOf(0)}, false)
			val := []reflect.Value{reflect.MakeFunc(funcType, func(callArgs []reflect.Value) []reflect.Value {
				f.interp.markOwnedCopyWriteLocked(callArgs[0], callArgs[1])
				return []reflect.Value{reflect.ValueOf(reflect.Copy(callArgs[0], callArgs[1]))}
			}), args[0], args[1]}
			f.deferred = append([]deferredCall{{values: val, exclusive: true}}, f.deferred...)
			return next
		}
		return
	}

	n.exec = func(f *frame) bltn {
		args := []reflect.Value{in[0](f), in[1](f)}
		f.interp.markOwnedCopyWriteFromExec(args[0], args[1])
		out[0](f).Set(reflect.ValueOf(reflect.Copy(args[0], args[1])))
		return next
	}
}

func _close(n *node) {
	in := []func(*frame) reflect.Value{genValue(n.child[1])}

	genBuiltinDeferWrapper(n, in, false, true, func(args []reflect.Value) []reflect.Value {
		args[0].Close()
		return nil
	})
}

func _complex(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(complex(0, 0)))
	c1, c2 := n.child[1], n.child[2]
	convertLiteralValue(c1, floatType)
	convertLiteralValue(c2, floatType)
	value0 := genValue(c1)
	value1 := genValue(c2)
	values := []func(*frame) reflect.Value{value0, value1}
	useCallArgSnapshots(n, values)
	value0, value1 = values[0], values[1]
	next := getExec(n.tnext)

	typ := n.typ.TypeOf()
	if isComplex(typ) {
		if wantEmptyInterface(n) {
			n.exec = func(f *frame) bltn {
				dest(f).Set(reflect.ValueOf(complex(value0(f).Float(), value1(f).Float())))
				return next
			}
			return
		}
		n.exec = func(f *frame) bltn {
			dest(f).SetComplex(complex(value0(f).Float(), value1(f).Float()))
			return next
		}
		return
	}
	// Not a complex type: ignore imaginary part
	n.exec = func(f *frame) bltn {
		dest(f).Set(value0(f).Convert(typ))
		return next
	}
}

func _imag(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(float64(0)))
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	if wantEmptyInterface(n) {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(imag(value(f).Complex())))
			return next
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetFloat(imag(value(f).Complex()))
		return next
	}
}

func _real(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(float64(0)))
	convertLiteralValue(n.child[1], complexType)
	value := genValue(n.child[1])
	next := getExec(n.tnext)

	if wantEmptyInterface(n) {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(real(value(f).Complex())))
			return next
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetFloat(real(value(f).Complex()))
		return next
	}
}

func _delete(n *node) {
	value0 := genValue(n.child[1]) // map
	value1 := genValue(n.child[2]) // key
	in := []func(*frame) reflect.Value{value0, value1}
	useCallArgSnapshots(n, in)
	var z reflect.Value

	genBuiltinDeferWrapper(n, in, false, true, func(args []reflect.Value) []reflect.Value {
		args[0].SetMapIndex(args[1], z)
		return nil
	})
}

func capConst(n *node) {
	// There is no Cap() method for reflect.Type, just return Len() instead.
	lenConst(n)
}

func lenConst(n *node) {
	n.rval = reflect.New(reflect.TypeOf(int(0))).Elem()
	c1 := n.child[1]
	if c1.rval.IsValid() {
		n.rval.SetInt(int64(len(vString(c1.rval))))
		return
	}
	t := c1.typ.TypeOf()
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	n.rval.SetInt(int64(t.Len()))
}

func _len(n *node) {
	dest := genValueOutput(n, reflect.TypeOf(int(0)))
	value := genValue(n.child[1])
	if isPtr(n.child[1].typ) {
		val := value
		value = func(f *frame) reflect.Value {
			v := val(f).Elem()
			for v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			return v
		}
	}
	next := getExec(n.tnext)

	if wantEmptyInterface(n) {
		n.exec = func(f *frame) bltn {
			dest(f).Set(reflect.ValueOf(value(f).Len()))
			return next
		}
		return
	}
	n.exec = func(f *frame) bltn {
		dest(f).SetInt(int64(value(f).Len()))
		return next
	}
}

func _max(n *node) { genMinMax(n, false) }

func _min(n *node) { genMinMax(n, true) }

// genMinMax compiles the min and max builtins (Go 1.21+). Arguments have been
// type checked to a single ordered type by check.builtin; values are compared
// through reflect so numeric kinds of any width and strings are covered.
func genMinMax(n *node, isMin bool) {
	dest := genValueOutput(n, n.typ.TypeOf())
	values := make([]func(*frame) reflect.Value, 0, len(n.child)-1)
	for _, c := range n.child[1:] {
		values = append(values, genValue(c))
	}
	next := getExec(n.tnext)

	if wantEmptyInterface(n) {
		n.exec = func(f *frame) bltn {
			best := values[0](f)
			for _, value := range values[1:] {
				if v := value(f); isMin == valueLess(v, best) {
					best = v
				}
			}
			dest(f).Set(reflect.ValueOf(best.Interface()))
			return next
		}
		return
	}
	n.exec = func(f *frame) bltn {
		best := values[0](f)
		for _, value := range values[1:] {
			v := value(f)
			if valueIsNaN(v) {
				// Per spec, if any argument is NaN the result is NaN.
				best = v
				break
			}
			if !valueIsNaN(best) && isMin == valueLess(v, best) {
				best = v
			}
		}
		d := dest(f)
		// The destination cell type may be narrower than the untyped
		// arguments' default type: convert before storing.
		if !best.Type().AssignableTo(d.Type()) {
			best = best.Convert(d.Type())
		}
		d.Set(best)
		return next
	}
}

// valueLess reports whether a < b for values of the same ordered type.
// It returns false when either floating point value is NaN, in which case
// valueNaN reports whether the NaN must be propagated as the result.
func valueLess(a, b reflect.Value) bool {
	if a.Kind() == reflect.Interface {
		a = a.Elem()
	}
	if b.Kind() == reflect.Interface {
		b = b.Elem()
	}
	switch a.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() < b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() < b.Uint()
	case reflect.Float32, reflect.Float64:
		return a.Float() < b.Float()
	case reflect.String:
		return a.String() < b.String()
	}
	return false
}

// valueIsNaN reports whether v is a floating point NaN value.
func valueIsNaN(v reflect.Value) bool {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float() != v.Float()
	}
	return false
}

// unsafeBuiltin compiles the unsafe.Slice and unsafe.String builtins. The
// destination type is computed at compile time from the pointer argument
// type; the runtime slices the memory range without copying it.
func unsafeBuiltin(n *node) {
	next := getExec(n.tnext)
	dest := genValueOutput(n, n.typ.TypeOf())
	value0 := genValue(n.child[1])
	value1 := genValue(n.child[2])
	isStringB := n.child[0].ident == bltnUnsafeString
	destType := n.typ.TypeOf()

	n.exec = func(f *frame) bltn {
		p := value0(f)
		lv := value1(f)
		var l int
		switch lv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			l = int(lv.Int())
		default:
			l = int(lv.Uint())
		}
		if isStringB {
			if l < 0 {
				panic("unsafe.String: length out of range")
			}
			s := unsafe.String((*byte)(unsafe.Pointer(p.Pointer())), l)
			dest(f).Set(reflect.ValueOf(s).Convert(destType))
			return next
		}
		if p.IsNil() {
			if l != 0 {
				panic("unsafe.Slice: ptr is nil and len is not zero")
			}
			dest(f).Set(reflect.Zero(destType))
			return next
		}
		if l < 0 {
			panic("unsafe.Slice: len out of range")
		}
		s := reflect.New(destType).Elem()
		h := (*reflect.SliceHeader)(unsafe.Pointer(s.UnsafeAddr())) //nolint:gosec,staticcheck
		h.Data = p.Pointer()
		h.Len = l
		h.Cap = l
		dest(f).Set(s)
		return next
	}
}

func _new(n *node) {
	next := getExec(n.tnext)
	t1 := n.child[1].typ
	typ := t1.TypeOf()
	dest := genValueOutput(n, reflect.PtrTo(typ))

	if isInterfaceSrc(t1) && (!isEmptyInterface(t1) || len(t1.method) > 0) {
		typ = zeroInterfaceValue().Type()
	}

	n.exec = func(f *frame) bltn {
		v := reflect.New(typ)
		f.interp.registerOwnedValue(v, f)
		if vi, ok := v.Interface().(*valueInterface); ok {
			vi.node = n
		}
		setOwnedValueOutput(f, dest(f), v)
		return next
	}
}

// _make allocates and initializes a slice, a map or a chan.
func _make(n *node) {
	next := getExec(n.tnext)
	typ := n.child[1].typ.frameType()
	dest := genValueOutput(n, typ)

	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		value := genValue(n.child[2])

		switch len(n.child) {
		case 3:
			values := []func(*frame) reflect.Value{value}
			useCallArgSnapshots(n, values)
			value = values[0]
			n.exec = func(f *frame) bltn {
				length := int(vInt(value(f)))
				slice := reflect.MakeSlice(typ, length, length)
				f.interp.registerOwnedValue(slice, f)
				setOwnedValueOutput(f, dest(f), slice)
				return next
			}
		case 4:
			value1 := genValue(n.child[3])
			values := []func(*frame) reflect.Value{value, value1}
			useCallArgSnapshots(n, values)
			value, value1 = values[0], values[1]
			n.exec = func(f *frame) bltn {
				slice := reflect.MakeSlice(typ, int(vInt(value(f))), int(vInt(value1(f))))
				f.interp.registerOwnedValue(slice, f)
				setOwnedValueOutput(f, dest(f), slice)
				return next
			}
		}

	case reflect.Chan:
		switch len(n.child) {
		case 2:
			n.exec = func(f *frame) bltn {
				channel := reflect.MakeChan(typ, 0)
				f.interp.registerOwnedChannel(channel, f)
				setOwnedValueOutput(f, dest(f), channel)
				return next
			}
		case 3:
			value := genValue(n.child[2])
			values := []func(*frame) reflect.Value{value}
			useCallArgSnapshots(n, values)
			value = values[0]
			n.exec = func(f *frame) bltn {
				channel := reflect.MakeChan(typ, int(vInt(value(f))))
				f.interp.registerOwnedChannel(channel, f)
				setOwnedValueOutput(f, dest(f), channel)
				return next
			}
		}

	case reflect.Map:
		switch len(n.child) {
		case 2:
			n.exec = func(f *frame) bltn {
				value := reflect.MakeMap(typ)
				f.interp.registerOwnedValue(value, f)
				setOwnedValueOutput(f, dest(f), value)
				return next
			}
		case 3:
			value := genValue(n.child[2])
			values := []func(*frame) reflect.Value{value}
			useCallArgSnapshots(n, values)
			value = values[0]
			n.exec = func(f *frame) bltn {
				mapped := reflect.MakeMapWithSize(typ, int(vInt(value(f))))
				f.interp.registerOwnedValue(mapped, f)
				setOwnedValueOutput(f, dest(f), mapped)
				return next
			}
		}
	}
}

func reset(n *node) {
	next := getExec(n.tnext)

	switch l := len(n.child) - 1; l {
	case 1:
		typ := n.child[0].typ.frameType()
		i := n.child[0].findex
		n.exec = func(f *frame) bltn {
			f.data[i] = reflect.New(typ).Elem()
			return next
		}
	case 2:
		c0, c1 := n.child[0], n.child[1]
		i0, i1 := c0.findex, c1.findex
		t0, t1 := c0.typ.frameType(), c1.typ.frameType()
		n.exec = func(f *frame) bltn {
			f.data[i0] = reflect.New(t0).Elem()
			f.data[i1] = reflect.New(t1).Elem()
			return next
		}
	default:
		types := make([]reflect.Type, l)
		index := make([]int, l)
		for i, c := range n.child[:l] {
			index[i] = c.findex
			types[i] = c.typ.frameType()
		}
		n.exec = func(f *frame) bltn {
			for i, ind := range index {
				f.data[ind] = reflect.New(types[i]).Elem()
			}
			return next
		}
	}
}

// recv reads from a channel.
func recv(n *node) {
	value := genValue(n.child[0])
	tnext := getExec(n.tnext)
	i := n.findex
	l := n.level
	store := func(f *frame, v reflect.Value) {
		owner := getFrame(f, l)
		if n.anc != nil && n.anc.kind == assignStmt && n.anc.action == aAssign {
			destination := owner.data[i]
			f.interp.markOwnedCellWriteFromExec(destination, v)
			destination.Set(v)
			return
		}
		// The received value is an unaddressable copy; store an addressable
		// one so later writes into the same frame cell (for example the
		// regular-return Set) keep working.
		nv := reflect.New(v.Type()).Elem()
		nv.Set(v)
		owner.data[i] = nv
	}

	if n.interp.cancelChan {
		// Cancellable channel read
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				// Fast: channel read doesn't block
				ch := value(f)
				if r, ok := ch.TryRecv(); ok {
					r = f.interp.adoptInterpretedFuncValue(f, ch, r)
					store(f, r)
					if r.Bool() {
						return tnext
					}
					return fnext
				}
				// Slow: channel read blocks, allow cancel
				f.mutex.RLock()
				done := f.done
				f.mutex.RUnlock()

				chosen, r, _ := selectWithFuncSweepFenceReleased(f, []reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
				if chosen == 0 {
					return nil
				}
				r = f.interp.adoptInterpretedFuncValue(f, ch, r)
				store(f, r)
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				// Fast: channel read doesn't block
				ch := value(f)
				if r, ok := ch.TryRecv(); ok {
					r = f.interp.adoptInterpretedFuncValue(f, ch, r)
					store(f, r)
					return tnext
				}
				// Slow: channel is blocked, allow cancel
				f.mutex.RLock()
				done := f.done
				f.mutex.RUnlock()

				chosen, r, _ := selectWithFuncSweepFenceReleased(f, []reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
				if chosen == 0 {
					return nil
				}
				r = f.interp.adoptInterpretedFuncValue(f, ch, r)
				store(f, r)
				return tnext
			}
		}
	} else {
		// Blocking channel read (less overhead)
		if n.fnext != nil {
			fnext := getExec(n.fnext)
			n.exec = func(f *frame) bltn {
				ch := value(f)
				r, _ := recvWithFuncSweepFenceReleased(f, ch)
				r = f.interp.adoptInterpretedFuncValue(f, ch, r)
				store(f, r)
				if r.Bool() {
					return tnext
				}
				return fnext
			}
		} else {
			n.exec = func(f *frame) bltn {
				channel := value(f)
				r, _ := recvWithFuncSweepFenceReleased(f, channel)
				r = f.interp.adoptInterpretedFuncValue(f, channel, r)
				store(f, r)
				return tnext
			}
		}
	}
}

func recv2(n *node) {
	vchan := genValue(n.child[0])    // chan
	vres := genValue(n.anc.child[0]) // result
	vok := genValue(n.anc.child[1])  // status
	setStatus := n.anc.child[1].ident != "_"
	tnext := getExec(n.tnext)

	if n.interp.cancelChan {
		// Cancellable channel read
		n.exec = func(f *frame) bltn {
			ch, result, status := vchan(f), vres(f), vok(f)
			//  Fast: channel read doesn't block
			if v, ok := ch.TryRecv(); ok {
				v = f.interp.adoptInterpretedFuncValue(f, ch, v)
				f.interp.markOwnedCellWriteFromExec(result, v)
				result.Set(v)
				if setStatus {
					status.SetBool(true)
				}
				return tnext
			}
			// Slow: channel is blocked, allow cancel
			f.mutex.RLock()
			done := f.done
			f.mutex.RUnlock()

			chosen, v, ok := selectWithFuncSweepFenceReleased(f, []reflect.SelectCase{done, {Dir: reflect.SelectRecv, Chan: ch}})
			if chosen == 0 {
				return nil
			}
			v = f.interp.adoptInterpretedFuncValue(f, ch, v)
			f.interp.markOwnedCellWriteFromExec(result, v)
			result.Set(v)
			if setStatus {
				status.SetBool(ok)
			}
			return tnext
		}
	} else {
		// Blocking channel read (less overhead)
		n.exec = func(f *frame) bltn {
			channel := vchan(f)
			v, ok := recvWithFuncSweepFenceReleased(f, channel)
			v = f.interp.adoptInterpretedFuncValue(f, channel, v)
			result := vres(f)
			f.interp.markOwnedCellWriteFromExec(result, v)
			result.Set(v)
			if setStatus {
				vok(f).SetBool(ok)
			}
			return tnext
		}
	}
}

func convertLiteralValue(n *node, t reflect.Type) {
	switch {
	case n.typ.cat == nilT:
		// Create a zero value of target type.
		n.rval = reflect.New(t).Elem()
	case !(n.kind == basicLit || n.rval.IsValid()) || t == nil || t.Kind() == reflect.Interface || t == valueInterfaceType || t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Interface:
		// Skip non-constant values, undefined target type or interface target type.
	case n.rval.IsValid():
		// Convert constant value to target type.
		convertConstantValue(n)
		n.rval = n.rval.Convert(t)
	default:
		// Create a zero value of target type.
		n.rval = reflect.New(t).Elem()
	}
}

func convertConstantValue(n *node) {
	if !n.rval.IsValid() {
		return
	}
	c, ok := n.rval.Interface().(constant.Value)
	if !ok {
		return
	}

	var v reflect.Value

	switch c.Kind() {
	case constant.Bool:
		v = reflect.ValueOf(constant.BoolVal(c))
	case constant.String:
		v = reflect.ValueOf(constant.StringVal(c))
	case constant.Int:
		i, x := constant.Int64Val(c)
		if !x {
			panic(n.cfgErrorf("constant %s overflows int64", c.ExactString()))
		}
		v = reflect.ValueOf(int(i))
	case constant.Float:
		f, _ := constant.Float64Val(c)
		v = reflect.ValueOf(f)
	case constant.Complex:
		r, _ := constant.Float64Val(constant.Real(c))
		i, _ := constant.Float64Val(constant.Imag(c))
		v = reflect.ValueOf(complex(r, i))
	}

	n.rval = v.Convert(n.typ.TypeOf())
}

// Write to a channel.
func send(n *node) {
	next := getExec(n.tnext)
	c0, c1 := n.child[0], n.child[1]
	value0 := genValue(c0) // Send channel.
	value1 := genDestValue(c0.typ.elem(), c1)

	if !n.interp.cancelChan {
		// Send is non-cancellable, has the least overhead.
		n.exec = func(f *frame) bltn {
			ch, data := value0(f), value1(f)
			token := f.interp.markInterpretedFuncChannelSend(f, ch, data)
			delivered := false
			defer func() {
				if !delivered {
					f.interp.rollbackInterpretedFuncChannelSend(token)
				}
			}()
			sendWithFuncSweepFenceReleased(f, ch, data)
			delivered = true
			f.interp.commitInterpretedFuncChannelSend(token)
			return next
		}
		return
	}

	// Send is cancellable, may have some overhead.
	n.exec = func(f *frame) bltn {
		ch, data := value0(f), value1(f)
		token := f.interp.markInterpretedFuncChannelSend(f, ch, data)
		delivered := false
		defer func() {
			if !delivered {
				f.interp.rollbackInterpretedFuncChannelSend(token)
			}
		}()
		// Fast: send on channel doesn't block.
		if ok := ch.TrySend(data); ok {
			delivered = true
			f.interp.commitInterpretedFuncChannelSend(token)
			return next
		}
		// Slow: send on channel blocks, allow cancel.
		f.mutex.RLock()
		done := f.done
		f.mutex.RUnlock()

		chosen, _, _ := selectWithFuncSweepFenceReleased(f, []reflect.SelectCase{done, {Dir: reflect.SelectSend, Chan: ch, Send: data}})
		if chosen == 0 {
			return nil
		}
		delivered = true
		f.interp.commitInterpretedFuncChannelSend(token)
		return next
	}
}

func clauseChanDir(n *node) (*node, *node, *node, reflect.SelectDir) {
	dir := reflect.SelectDefault
	var nod, assigned, ok *node
	var stop bool

	n.Walk(func(m *node) bool {
		switch m.action {
		case aRecv:
			dir = reflect.SelectRecv
			nod = m.child[0]
			switch m.anc.action {
			case aAssign:
				assigned = m.anc.child[0]
			case aAssignX:
				assigned = m.anc.child[0]
				ok = m.anc.child[1]
			}
			stop = true
		case aSend:
			dir = reflect.SelectSend
			nod = m.child[0]
			assigned = m.child[1]
			stop = true
		}
		return !stop
	}, nil)
	return nod, assigned, ok, dir
}

func _select(n *node) {
	nbClause := len(n.child)
	chans := make([]*node, nbClause)
	assigned := make([]*node, nbClause)
	ok := make([]*node, nbClause)
	clause := make([]bltn, nbClause)
	chanValues := make([]func(*frame) reflect.Value, nbClause)
	assignedValues := make([]func(*frame) reflect.Value, nbClause)
	okValues := make([]func(*frame) reflect.Value, nbClause)
	cases := make([]reflect.SelectCase, nbClause+1)
	next := getExec(n.tnext)

	for i := 0; i < nbClause; i++ {
		cl := n.child[i]
		if cl.kind == commClauseDefault {
			cases[i].Dir = reflect.SelectDefault
			if len(cl.child) == 0 {
				clause[i] = func(*frame) bltn { return next }
			} else {
				clause[i] = getExec(cl.child[0].start)
			}
			continue
		}
		// The comm clause is in send or recv direction.
		switch c0 := cl.child[0]; {
		case len(cl.child) > 1:
			// The comm clause contains a channel operation and a clause body.
			clause[i] = getExec(cl.child[1].start)
			chans[i], assigned[i], ok[i], cases[i].Dir = clauseChanDir(c0)
			chanValues[i] = genValue(chans[i])
			if assigned[i] != nil {
				assignedValues[i] = genValue(assigned[i])
			}
			if ok[i] != nil {
				okValues[i] = genValue(ok[i])
			}
		case c0.kind == exprStmt && len(c0.child) == 1 && c0.child[0].action == aRecv:
			// The comm clause has an empty body clause after channel receive.
			chanValues[i] = genValue(c0.child[0].child[0])
			cases[i].Dir = reflect.SelectRecv
			clause[i] = func(*frame) bltn { return next }
		case c0.action == aAssign || c0.action == aAssignX:
			chans[i], assigned[i], ok[i], cases[i].Dir = clauseChanDir(c0)
			chanValues[i] = genValue(chans[i])
			if assigned[i] != nil {
				assignedValues[i] = genValue(assigned[i])
			}
			if ok[i] != nil {
				okValues[i] = genValue(ok[i])
			}
			clause[i] = func(*frame) bltn { return next }
		case c0.kind == sendStmt:
			// The comm clause as an empty body clause after channel send.
			chanValues[i] = genValue(c0.child[0])
			cases[i].Dir = reflect.SelectSend
			assignedValues[i] = genValue(c0.child[1])
			clause[i] = func(*frame) bltn { return next }
		}
	}

	n.exec = func(f *frame) bltn {
		f.mutex.RLock()
		cases[nbClause] = f.done
		f.mutex.RUnlock()

		pendingSends := make([]*ownedChannelSend, 0, nbClause)
		settled := false
		defer func() {
			if !settled {
				for _, send := range pendingSends {
					f.interp.rollbackInterpretedFuncChannelSend(send)
				}
			}
		}()
		for i := range cases[:nbClause] {
			switch cases[i].Dir {
			case reflect.SelectRecv:
				cases[i].Chan = chanValues[i](f)
			case reflect.SelectSend:
				cases[i].Chan = chanValues[i](f)
				cases[i].Send = assignedValues[i](f)
				pendingSends = append(pendingSends, f.interp.markInterpretedFuncChannelSend(f, cases[i].Chan, cases[i].Send))
			case reflect.SelectDefault:
				// Keep zero values for comm clause
			}
		}
		j, v, s := selectWithFuncSweepFenceReleased(f, cases)
		for i := range cases[:nbClause] {
			if cases[i].Dir == reflect.SelectSend && i != j {
				f.interp.rollbackInterpretedFuncChannelSend(pendingSends[0])
				pendingSends = pendingSends[1:]
			} else if cases[i].Dir == reflect.SelectSend {
				f.interp.commitInterpretedFuncChannelSend(pendingSends[0])
				pendingSends = pendingSends[1:]
			}
		}
		settled = true
		if j == nbClause {
			return nil
		}
		if cases[j].Dir == reflect.SelectRecv && assignedValues[j] != nil {
			v = f.interp.adoptInterpretedFuncValue(f, cases[j].Chan, v)
			destination := assignedValues[j](f)
			f.interp.markOwnedCellWriteFromExec(destination, v)
			destination.Set(v)
			if ok[j] != nil {
				okValues[j](f).SetBool(s)
			}
		}
		if cases[j].Dir == reflect.SelectRecv && assignedValues[j] == nil {
			f.interp.adoptInterpretedFuncValue(f, cases[j].Chan, v)
		}
		return clause[j]
	}
}

// slice expression: array[low:high:max].
func slice(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)
	value0 := genValueArray(n.child[0]) // array
	value1 := genValue(n.child[1])      // low (if 2 or 3 args) or high (if 1 arg)

	switch len(n.child) {
	case 2:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(int(vInt(value1(f))), a.Len())
			return next
		}
	case 3:
		value2 := genValue(n.child[2]) // max

		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(int(vInt(value1(f))), int(vInt(value2(f))))
			return next
		}
	case 4:
		value2 := genValue(n.child[2])
		value3 := genValue(n.child[3])

		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice3(int(vInt(value1(f))), int(vInt(value2(f))), int(vInt(value3(f))))
			return next
		}
	}
}

// slice expression, no low value: array[:high:max].
func slice0(n *node) {
	i := n.findex
	l := n.level
	next := getExec(n.tnext)
	value0 := genValueArray(n.child[0])

	switch len(n.child) {
	case 1:
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(0, a.Len())
			return next
		}
	case 2:
		value1 := genValue(n.child[1])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice(0, int(vInt(value1(f))))
			return next
		}
	case 3:
		value1 := genValue(n.child[1])
		value2 := genValue(n.child[2])
		n.exec = func(f *frame) bltn {
			a := value0(f)
			getFrame(f, l).data[i] = a.Slice3(0, int(vInt(value1(f))), int(vInt(value2(f))))
			return next
		}
	}
}

func isNilChild(child int) func(n *node) {
	return func(n *node) {
		var value func(*frame) reflect.Value
		child := n.child[child]
		value = genValue(child)
		typ := n.typ.concrete().TypeOf()
		isInterface := n.typ.TypeOf().Kind() == reflect.Interface
		tnext := getExec(n.tnext)
		dest := genValue(n)
		if n.fnext == nil {
			if !isInterfaceSrc(child.typ) {
				if isInterface {
					n.exec = func(f *frame) bltn {
						dest(f).Set(reflect.ValueOf(value(f).IsNil()).Convert(typ))
						return tnext
					}
					return
				}
				n.exec = func(f *frame) bltn {
					dest(f).SetBool(value(f).IsNil())
					return tnext
				}
				return
			}
			if isInterface {
				n.exec = func(f *frame) bltn {
					v := value(f)
					var r bool
					if vi, ok := v.Interface().(valueInterface); ok {
						r = (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT)
					} else {
						r = v.IsNil()
					}
					dest(f).Set(reflect.ValueOf(r).Convert(typ))
					return tnext
				}
				return
			}
			n.exec = func(f *frame) bltn {
				v := value(f)
				var r bool
				if vi, ok := v.Interface().(valueInterface); ok {
					r = (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT)
				} else {
					r = v.IsNil()
				}
				dest(f).SetBool(r)
				return tnext
			}
			return
		}

		fnext := getExec(n.fnext)

		if !isInterfaceSrc(child.typ) {
			n.exec = func(f *frame) bltn {
				if value(f).IsNil() {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).SetBool(false)
				return fnext
			}
			return
		}

		n.exec = func(f *frame) bltn {
			v := value(f)
			if vi, ok := v.Interface().(valueInterface); ok {
				if (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT) {
					dest(f).SetBool(true)
					return tnext
				}
				dest(f).SetBool(false)
				return fnext
			}
			if v.IsNil() {
				dest(f).SetBool(true)
				return tnext
			}
			dest(f).SetBool(false)
			return fnext
		}
	}
}

func isNotNil(n *node) {
	var value func(*frame) reflect.Value
	c0 := n.child[0]
	value = genValue(c0)
	typ := n.typ.concrete().TypeOf()
	isInterface := n.typ.TypeOf().Kind() == reflect.Interface
	tnext := getExec(n.tnext)
	dest := genValue(n)

	if n.fnext == nil {
		if isInterfaceSrc(c0.typ) && c0.typ.TypeOf() != valueInterfaceType {
			if isInterface {
				n.exec = func(f *frame) bltn {
					dest(f).Set(reflect.ValueOf(!value(f).IsNil()).Convert(typ))
					return tnext
				}
				return
			}
			n.exec = func(f *frame) bltn {
				dest(f).SetBool(!value(f).IsNil())
				return tnext
			}
			return
		}

		if isInterface {
			n.exec = func(f *frame) bltn {
				v := value(f)
				var r bool
				if vi, ok := v.Interface().(valueInterface); ok {
					r = (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT)
				} else {
					r = v.IsNil()
				}
				dest(f).Set(reflect.ValueOf(!r).Convert(typ))
				return tnext
			}
			return
		}
		n.exec = func(f *frame) bltn {
			v := value(f)
			var r bool
			if vi, ok := v.Interface().(valueInterface); ok {
				r = (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT)
			} else {
				r = v.IsNil()
			}
			dest(f).SetBool(!r)
			return tnext
		}
		return
	}

	fnext := getExec(n.fnext)

	if isInterfaceSrc(c0.typ) && c0.typ.TypeOf() != valueInterfaceType {
		n.exec = func(f *frame) bltn {
			if value(f).IsNil() {
				dest(f).SetBool(false)
				return fnext
			}
			dest(f).SetBool(true)
			return tnext
		}
		return
	}

	n.exec = func(f *frame) bltn {
		v := value(f)
		if vi, ok := v.Interface().(valueInterface); ok {
			if (vi == valueInterface{} || vi.node.kind == basicLit && vi.node.typ.cat == nilT) {
				dest(f).SetBool(false)
				return fnext
			}
			dest(f).SetBool(true)
			return tnext
		}
		if v.IsNil() {
			dest(f).SetBool(false)
			return fnext
		}
		dest(f).SetBool(true)
		return tnext
	}
}

func complexConst(n *node) {
	if v0, v1 := n.child[1].rval, n.child[2].rval; v0.IsValid() && v1.IsValid() {
		n.rval = reflect.ValueOf(complex(vFloat(v0), vFloat(v1)))
		n.gen = nop
	}
}

func imagConst(n *node) {
	if v := n.child[1].rval; v.IsValid() {
		n.rval = reflect.ValueOf(imag(v.Complex()))
		n.gen = nop
	}
}

func realConst(n *node) {
	if v := n.child[1].rval; v.IsValid() {
		n.rval = reflect.ValueOf(real(v.Complex()))
		n.gen = nop
	}
}

// minMaxConst computes the constant value of a min or max call whose
// arguments are all constants, in a constant declaration context.
func minMaxConst(n *node) error {
	isMin := n.child[0].ident == bltnMin
	var best constant.Value
	for _, c := range n.child[1:] {
		if !c.rval.IsValid() {
			return c.cfgErrorf("%s argument is not a constant", n.child[0].ident)
		}
		cv, ok := c.rval.Interface().(constant.Value)
		if !ok {
			if c.rval.Type().Kind() == reflect.Interface {
				return c.cfgErrorf("%s argument is not a constant", n.child[0].ident)
			}
			cv = constant.Make(c.rval.Interface())
		}
		if best == nil {
			best = cv
			continue
		}
		op := token.GTR
		if isMin {
			op = token.LSS
		}
		if constant.Compare(cv, op, best) {
			best = cv
		}
	}
	if best == nil {
		best = constant.MakeInt64(0)
	}
	n.rval = reflect.ValueOf(best)
	return nil
}
