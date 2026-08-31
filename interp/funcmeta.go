package interp

import (
	"reflect"
	"runtime"
	"unsafe"
)

// funcval mirrors the layout of a Go func value: a func variable stores the
// address of a funcval, whose first word is the code pointer. The ADDRESS of
// the funcval is the identity of the func value, and is what the registry is
// keyed by.
type funcval struct {
	fn uintptr
}

// funcvalRef bundles the registry key of a func value with the typed funcval
// pointer needed to arm its eviction finalizer.
type funcvalRef struct {
	key uintptr
	ptr *funcval
}

// funcvalRefOf derives the funcval identity of the wrapper v: the address of
// the funcval, read as the single word a func variable stores. This is the
// same word reflect.Value comparison used by the previous canonical func
// keys, so key semantics are preserved; the value itself is not retained.
func funcvalRefOf(v reflect.Value) (funcvalRef, bool) {
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() || !v.CanInterface() {
		return funcvalRef{}, false
	}
	cell := reflect.New(v.Type())
	cell.Elem().Set(v)
	ptr := (*funcval)(*(*unsafe.Pointer)(cell.UnsafePointer()))
	return funcvalRef{key: uintptr(unsafe.Pointer(ptr)), ptr: ptr}, true
}

// funcvalKeyOf derives only the registry key of the wrapper v.
func funcvalKeyOf(v reflect.Value) (uintptr, bool) {
	ref, ok := funcvalRefOf(v)
	return ref.key, ok
}

// funcvalKey derives the registry key from an already canonical func value
// (one produced by canonicalFuncValue, which guarantees a non-nil,
// interfaceable func): its funcval address.
func funcvalKey(canonical reflect.Value) uintptr {
	cell := reflect.New(canonical.Type())
	cell.Elem().Set(canonical)
	return uintptr(*(*unsafe.Pointer)(cell.UnsafePointer()))
}

// sameFuncvalKey reports whether two func values share one funcval.
func sameFuncvalKey(left, right reflect.Value) bool {
	leftKey, leftOK := funcvalKeyOf(left)
	rightKey, rightOK := funcvalKeyOf(right)
	return leftOK && rightOK && leftKey == rightKey
}

// insertFuncMetaEntryLocked installs meta for ref.key in the registry,
// guarding it with a generation counter so a finalizer armed for a dropped
// wrapper can never evict a later registration that reused the address, and
// arming that finalizer: when the wrapper becomes unreachable, the entry is
// reclaimed without waiting for a manual purge. The caller holds funcMu.
func (interp *Interpreter) insertFuncMetaEntryLocked(ref funcvalRef, meta interpretedFuncMeta, owner *frame) {
	generation := uint64(0)
	if old, ok := interp.funcMeta[ref.key]; ok {
		generation = old.generation + 1
	}
	meta.generation = generation
	interp.funcMeta[ref.key] = meta
	if owner != nil {
		owner.funcMeta = append(owner.funcMeta, ref.key)
	}
	// Sweeps and purges may have deleted the previous entry while its wrapper
	// is still alive (e.g. a bound alias restored after a sweep); clearing the
	// leftover finalizer is required before arming a new one. A finalizer can
	// only be queued while no mutator holds the object, and the clear below
	// requires holding ref.ptr, so at clear time any previous finalizer is at
	// worst armed-not-queued and the clear fully cancels it. The generation
	// counter stays as belt-and-braces against future lifetime changes.
	runtime.SetFinalizer(ref.ptr, nil)
	key := ref.key
	runtime.SetFinalizer(ref.ptr, func(fv *funcval) {
		// The closure captures only the interpreter, the key and the
		// generation — never the wrapper or the typed funcval pointer — so
		// arming the finalizer does not keep the wrapper alive.
		//
		// Invariant: ref.ptr must be the base of a heap funcval. Every
		// registration path passes a reflect.MakeFunc product (heap
		// makeFuncImpl), which SetFinalizer requires. A static funcval (a
		// top-level func or a no-capture literal) lives in rodata, where
		// SetFinalizer fatal-errors instead of no-op'ing — never route one
		// through here.
		interp.evictFuncMetaAtFinalizer(key, generation)
	})
}

// evictFuncMetaAtFinalizer deletes the entry whose wrapper was dropped. It
// runs on the finalizer goroutine, so it only takes funcMu: the wrapper is
// provably unreachable at this point (no execution can look it up, no send,
// capture cell, global or host reference retains it), so no funcSweep fence
// participation is needed — the eviction cannot invalidate anything a running
// execution or sweep is about to use.
func (interp *Interpreter) evictFuncMetaAtFinalizer(key uintptr, generation uint64) {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	interp.evictFuncMetaKeyLocked(key, generation)
}

// evictFuncMetaKeyLocked deletes the registry entry for key if its generation
// still matches, cleaning the owning frame's key list, the directFuncs cache
// lines anchored at either endpoint, and the group's memoized host-bound
// wrappers for the key. The caller holds funcMu.
func (interp *Interpreter) evictFuncMetaKeyLocked(key uintptr, generation uint64) {
	meta, ok := interp.funcMeta[key]
	if !ok || meta.generation != generation {
		return
	}
	delete(interp.funcMeta, key)
	if meta.frame != nil {
		removeFrameFuncMetaKeyLocked(meta.frame, key)
	}
	for activationKey, activation := range interp.directFuncs {
		if activationKey.source == key || activation.key == key {
			delete(interp.directFuncs, activationKey)
		}
	}
	if meta.group != nil && len(meta.group.bound) > 0 {
		for cacheKey := range meta.group.bound {
			if cacheKey.target == key {
				delete(meta.group.bound, cacheKey)
			}
		}
	}
}

// removeFrameFuncMetaKeyLocked drops key from a frame's registration list.
// The caller holds funcMu.
func removeFrameFuncMetaKeyLocked(f *frame, key uintptr) {
	for i, existing := range f.funcMeta {
		if existing == key {
			f.funcMeta = append(f.funcMeta[:i], f.funcMeta[i+1:]...)
			return
		}
	}
}

type funcFrameState uint8

type funcMetaRetention uint8

type funcMetaGroup struct {
	_           byte
	pending     int
	received    bool
	root        *frame
	version     uint64
	captures    []funcMetaCapture
	panicTokens map[*ownedPanicToken]struct{}
	// bound memoizes host-bound MakeFunc wrappers per (target, activation,
	// signature, boundary mode) so repeated native calls carrying the same
	// interpreted argument reuse one wrapper instead of allocating and
	// registering a fresh alias per call. Guarded by funcMu.
	bound map[boundWrapperKey]reflect.Value
}

type funcMetaCapture struct {
	frame *frame
	index int
}

type funcMetaCaptureRef struct {
	level int
	index int
}

// interpretedFuncCaptureRefs records the exact lexical slots a function
// literal (including nested literals which close over the same outer scope)
// reads from its creation environment. Package globals are resolved through
// the active root and therefore are not lexical captures.
func interpretedFuncCaptureRefs(n *node) []funcMetaCaptureRef {
	if n == nil || len(n.child) < 4 {
		return nil
	}
	seen := map[funcMetaCaptureRef]struct{}{}
	refs := []funcMetaCaptureRef{}
	var walk func(*node, int)
	walk = func(current *node, depth int) {
		if current == nil {
			return
		}
		if current != n && current.kind == funcLit {
			depth++
		}
		if current.kind == identExpr && current.findex >= 0 && current.level != globalFrame && current.level > depth {
			ref := funcMetaCaptureRef{level: current.level - depth - 1, index: current.findex}
			if _, ok := seen[ref]; !ok {
				seen[ref] = struct{}{}
				refs = append(refs, ref)
			}
		}
		for _, child := range current.child {
			walk(child, depth)
		}
	}
	walk(n.child[3], 0)
	return refs
}

func resolveInterpretedFuncCaptures(captured *frame, refs []funcMetaCaptureRef) []funcMetaCapture {
	if captured == nil || len(refs) == 0 {
		return nil
	}
	result := make([]funcMetaCapture, 0, len(refs))
	for _, ref := range refs {
		owner := getFrame(captured, ref.level)
		if owner == nil || ref.index < 0 || ref.index >= len(owner.data) {
			continue
		}
		result = append(result, funcMetaCapture{frame: owner, index: ref.index})
	}
	return result
}

const (
	funcFrameInactive funcFrameState = iota
	funcFrameActive
	funcFrameReleasing
	funcFrameFinished
)

const (
	funcMetaVisible funcMetaRetention = iota
	funcMetaChannel
	funcMetaPanic
	funcMetaOpaque
)

// withFuncSweepWriteFromExec upgrades the execution fence while escape
// metadata changes, then restores the read fence used by the CFG runner.
func (interp *Interpreter) withFuncSweepWriteFromExec(run func()) {
	if interp.funcSweepExclusive.Load() > 0 {
		// A canceled worker's deferred phase holds the fence exclusively;
		// the write upgrade is already satisfied on this goroutine.
		run()
		return
	}
	interp.funcSweepMu.RUnlock()
	interp.funcSweepMu.Lock()
	defer func() {
		interp.funcSweepMu.Unlock()
		interp.funcSweepMu.RLock()
	}()
	run()
}

func (interp *Interpreter) beginInterpretedFuncPanic(value reflect.Value) *interpretedPanic {
	frozen := value
	if !unwrapOwnedValue(value).IsValid() {
		frozen = reflect.ValueOf(&runtime.PanicNilError{})
	} else {
		frozen = reflect.New(value.Type()).Elem()
		frozen.Set(value)
	}
	state := &interpretedPanic{value: frozen}
	interp.withFuncSweepWriteFromExec(func() {
		state.token = interp.beginOwnedPanicLocked(frozen)
	})
	return state
}

func (interp *Interpreter) markInterpretedFuncChannelSend(f *frame, channel, value reflect.Value) *ownedChannelSend {
	var send *ownedChannelSend
	interp.withFuncSweepWriteFromExec(func() {
		interp.funcMu.RLock()
		owned := interp.ownedChannelLocked(channel)
		hostVisible := owned == nil || owned.hostVisible
		interp.funcMu.RUnlock()
		if hostVisible {
			interp.markOwnedValuesHostSharedLocked(value)
			interp.preserveReturnedInterpretedFuncsLocked(value)
			return
		}
		interp.funcMu.Lock()
		send = interp.recordOwnedChannelSendLocked(channel, value, f)
		interp.funcMu.Unlock()
	})
	return send
}

func (interp *Interpreter) commitInterpretedFuncChannelSend(send *ownedChannelSend) {
	if send == nil {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		interp.funcMu.Lock()
		if send.state == ownedChannelSendPrepared {
			send.state = ownedChannelSendDelivered
		}
		interp.funcMu.Unlock()
	})
}

func (interp *Interpreter) rollbackInterpretedFuncChannelSend(send *ownedChannelSend) {
	if send == nil {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		interp.funcMu.Lock()
		interp.retireOwnedChannelSendLocked(send)
		interp.releaseOwnedChannelSendFuncsLocked(send, send.funcs, nil, false)
		interp.funcMu.Unlock()
	})
}

func (interp *Interpreter) rollbackInterpretedFuncPanicEscape(recovered interface{}) {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	value, token := splitInterpretedPanic(recovered)
	interp.finishOwnedPanicToken(token)
	if token != nil {
		return
	}
	reflected := reflect.ValueOf(value)
	collector := newFuncValueCollector()
	if funcBearingValue(reflected) {
		collector.collect(reflected)
	}
	if len(collector.exact) == 0 {
		return
	}
	interp.funcMu.RLock()
	entries := make(map[uintptr]interpretedFuncMeta, len(interp.funcMeta))
	for key, meta := range interp.funcMeta {
		entries[key] = meta
	}
	interp.funcMu.RUnlock()
	groups := map[*funcMetaGroup]struct{}{}
	for key, meta := range entries {
		if collector.exactContains(key) {
			groups[meta.group] = struct{}{}
		}
	}
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	for key, meta := range interp.funcMeta {
		if _, ok := groups[meta.group]; !ok {
			continue
		}
		if meta.retention == funcMetaPanic {
			meta.retention = funcMetaVisible
			interp.funcMeta[key] = meta
		}
		if meta.frame != nil && meta.frame != meta.frame.root && meta.frame.funcEscape == funcMetaPanic {
			meta.frame.funcEscape = funcMetaVisible
		}
	}
}

func (interp *Interpreter) markInterpretedFuncMetadataEscapedLocked(retention funcMetaRetention, values ...reflect.Value) {
	collector := newFuncValueCollector()
	for _, value := range values {
		if !funcBearingValue(value) {
			continue
		}
		collector.collect(value)
	}
	if len(collector.exact) == 0 {
		return
	}

	interp.funcMu.RLock()
	entries := make(map[uintptr]interpretedFuncMeta, len(interp.funcMeta))
	for key, meta := range interp.funcMeta {
		entries[key] = meta
	}
	interp.funcMu.RUnlock()

	escaped := map[uintptr]struct{}{}
	for key := range entries {
		if collector.exactContains(key) {
			escaped[key] = struct{}{}
		}
	}
	if len(escaped) == 0 {
		return
	}

	interp.funcMu.Lock()
	rootGroups := map[*funcMetaGroup]struct{}{}
	markedGroups := map[*funcMetaGroup]struct{}{}
	for key := range escaped {
		meta, ok := interp.funcMeta[key]
		if !ok || meta.frame == nil {
			continue
		}
		if meta.group != nil {
			markedGroups[meta.group] = struct{}{}
		}
		if meta.frame == meta.frame.root {
			rootGroups[meta.group] = struct{}{}
		} else if retention > meta.frame.funcEscape {
			meta.frame.funcEscape = retention
		}
	}
	for key, meta := range interp.funcMeta {
		if _, ok := rootGroups[meta.group]; ok && retention > meta.retention {
			meta.retention = retention
			interp.funcMeta[key] = meta
		}
	}
	if retention == funcMetaChannel {
		for group := range markedGroups {
			group.pending++
		}
	}
	interp.funcMu.Unlock()
}

// releaseInterpretedFuncs removes metadata for wrappers whose creating frame
// has finished and whose values cannot escape that frame. Root registrations
// are permanent. Any ambiguous escape retains the complete frame group, since
// one reachable closure can itself capture another wrapper opaquely.
func (interp *Interpreter) releaseInterpretedFuncs(f *frame, funcNode *node, recovered interface{}) {
	if f == nil || f == f.root {
		return
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	if recovered != nil {
		var token *ownedPanicToken
		recovered, token = splitInterpretedPanic(recovered)
		if token == nil {
			interp.markInterpretedFuncMetadataEscapedLocked(funcMetaPanic, reflect.ValueOf(recovered))
		}
	}

	interp.funcMu.Lock()
	f.funcState = funcFrameReleasing
	keys := append([]uintptr(nil), f.funcMeta...)
	escape := f.funcEscape
	interp.funcMu.Unlock()
	if len(keys) == 0 {
		interp.finishInterpretedFuncs(f, nil, false, funcMetaVisible)
		return
	}

	targets := make(map[uintptr]struct{}, len(keys))
	for _, key := range keys {
		targets[key] = struct{}{}
	}
	if escape != funcMetaVisible {
		interp.finishInterpretedFuncs(f, f.root, true, escape)
		return
	}
	if frameReturnsFunction(f, funcNode, targets) {
		interp.finishInterpretedFuncs(f, f.anc, true, funcMetaVisible)
		return
	}
	if owner := funcsReachableFromAncestors(f, targets); owner != nil {
		interp.finishInterpretedFuncs(f, owner, true, funcMetaVisible)
		return
	}
	interp.finishInterpretedFuncs(f, nil, false, funcMetaVisible)
}

// finishInterpretedFuncs atomically closes a frame to child transfers and
// either deletes its group or moves it to a live ancestor. A child which
// finishes concurrently after this frame entered the releasing state is
// redirected to the root, so no registration can be appended after the sweep
// snapshot and then lose its next sweep owner.
func (interp *Interpreter) finishInterpretedFuncs(from, to *frame, retain bool, retention funcMetaRetention) {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	if from.funcEscape != funcMetaVisible {
		retain = true
		to = from.root
		retention = from.funcEscape
	}
	if retain {
		to = funcRetentionOwnerLocked(from.root, to)
	}
	group := &funcMetaGroup{root: from.root, version: 1}
	seenGroups := map[*funcMetaGroup]struct{}{}
	for _, key := range from.funcMeta {
		if meta, ok := interp.funcMeta[key]; ok && meta.frame == from && meta.group != nil {
			seenGroups[meta.group] = struct{}{}
		}
	}
	seenCaptures := map[funcMetaCapture]struct{}{}
	for oldGroup := range seenGroups {
		for _, capture := range oldGroup.captures {
			if _, ok := seenCaptures[capture]; ok {
				continue
			}
			seenCaptures[capture] = struct{}{}
			group.captures = append(group.captures, capture)
		}
		if retain && to == from.root {
			for token := range oldGroup.panicTokens {
				if group.panicTokens == nil {
					group.panicTokens = map[*ownedPanicToken]struct{}{}
				}
				group.panicTokens[token] = struct{}{}
				if _, raw := token.groups[oldGroup]; raw {
					token.groups[group] = struct{}{}
				}
				if _, pending := token.pendingGroups[oldGroup]; pending {
					token.pendingGroups[group] = struct{}{}
				}
			}
		}
	}
	for _, key := range from.funcMeta {
		meta, ok := interp.funcMeta[key]
		if !ok || meta.frame != from {
			continue
		}
		if retain {
			meta.frame = to
			meta.retention = retention
			if to == from.root {
				meta.group = group
			}
			interp.funcMeta[key] = meta
			to.funcMeta = append(to.funcMeta, key)
		} else {
			delete(interp.funcMeta, key)
		}
	}
	from.funcMeta = nil
	from.funcState = funcFrameFinished
	if retention == funcMetaChannel {
		for _, channel := range interp.ownedChannels {
			for _, send := range channel.sends {
				if send.state != ownedChannelSendTerminal {
					interp.refreshOwnedChannelSendLocked(send)
				}
			}
		}
	}
}

func funcRetentionOwnerLocked(root, owner *frame) *frame {
	if owner == nil {
		return root
	}
	if owner.cloneOf != nil {
		owner = owner.cloneOf
	}
	if owner != root && owner.funcState != funcFrameActive {
		return root
	}
	return owner
}

// adoptInterpretedFuncValues moves a delivered escape group into the receiving
// activation. Pending channel values stay protected while queued, but after a
// receive they are ordinary interpreter-visible values and can be reclaimed
// when the receiver and its globals no longer reference them.
func (interp *Interpreter) adoptInterpretedFuncValues(f *frame, source funcMetaRetention, channel reflect.Value, values ...reflect.Value) []reflect.Value {
	adopted := values
	interp.withFuncSweepWriteFromExec(func() {
		if source == funcMetaChannel {
			interp.funcMu.Lock()
			send := interp.consumeOwnedChannelSendLocked(channel, values...)
			interp.funcMu.Unlock()
			if send != nil {
				if send.pendingRoot == f.root && send.pending.IsValid() && len(values) == 1 {
					adopted = []reflect.Value{send.pending}
				} else {
					adopted = values
				}
				adopted = interp.adoptOwnedValuesLocked(f, funcMetaVisible, adopted...)
				interp.funcMu.Lock()
				interp.retireOwnedChannelSendLocked(send)
				interp.releaseOwnedChannelSendFuncsLocked(send, send.funcs, f.root, send.pending.IsValid())
				interp.releaseOwnedChannelSendFuncsLocked(send, send.pendingFuncs, f.root, false)
				interp.funcMu.Unlock()
				return
			}
		}
		adopted = interp.adoptInterpretedFuncValuesLocked(f, source, values...)
	})
	return adopted
}

func (interp *Interpreter) releaseOwnedChannelSendFuncsLocked(send *ownedChannelSend, funcs map[reflect.Value]struct{}, receivingRoot *frame, replaced bool) {
	for value := range funcs {
		key, ok := funcvalKeyOf(value)
		if !ok {
			continue
		}
		usedElsewhere := false
		for _, channel := range interp.ownedChannels {
			for _, other := range channel.sends {
				if other == send || other.state == ownedChannelSendTerminal {
					continue
				}
				if _, ok := other.funcs[value]; ok {
					usedElsewhere = true
				}
				if _, ok := other.pendingFuncs[value]; ok {
					usedElsewhere = true
				}
			}
		}
		if usedElsewhere {
			continue
		}
		meta, ok := interp.funcMeta[key]
		if !ok || meta.retention == funcMetaOpaque {
			continue
		}
		if replaced {
			delete(interp.funcMeta, key)
			continue
		}
		root := receivingRoot
		if root == nil && meta.frame != nil {
			root = meta.frame.root
		}
		if root == nil {
			continue
		}
		meta.frame = root
		meta.retention = funcMetaVisible
		interp.funcMeta[key] = meta
		root.funcMeta = append(root.funcMeta, key)
	}
}

func (interp *Interpreter) adoptInterpretedFuncValue(f *frame, channel, value reflect.Value) reflect.Value {
	values := interp.adoptInterpretedFuncValues(f, funcMetaChannel, channel, value)
	if len(values) == 0 {
		return value
	}
	return values[0]
}

func (interp *Interpreter) adoptInterpretedFuncPanicValue(f *frame, token *ownedPanicToken, value reflect.Value) reflect.Value {
	values := []reflect.Value{value}
	interp.withFuncSweepWriteFromExec(func() {
		if token == nil {
			values = interp.adoptInterpretedFuncValuesLocked(f, funcMetaPanic, values...)
		} else {
			values = interp.adoptOwnedPanicValuesLocked(f, token, values...)
		}
	})
	return values[0]
}

func (interp *Interpreter) adoptInterpretedFuncValuesLocked(f *frame, source funcMetaRetention, values ...reflect.Value) []reflect.Value {
	values = interp.adoptOwnedValuesLocked(f, source, values...)
	return interp.adoptInterpretedFuncMetadataValuesLocked(f, source, values...)
}

func (interp *Interpreter) adoptInterpretedFuncMetadataValuesLocked(f *frame, source funcMetaRetention, values ...reflect.Value) []reflect.Value {
	interp.funcMu.RLock()
	entries := make(map[uintptr]interpretedFuncMeta, len(interp.funcMeta))
	for key, meta := range interp.funcMeta {
		eligible := meta.retention == source
		if meta.frame != nil && meta.frame != meta.frame.root {
			eligible = meta.frame.funcEscape == source
		}
		if eligible {
			entries[key] = meta
		}
	}
	interp.funcMu.RUnlock()

	groups := map[*funcMetaGroup]struct{}{}
	for key, meta := range entries {
		if meta.group == nil {
			continue
		}
		visitor := funcValueVisitor{targets: map[uintptr]struct{}{key: {}}}
		for _, value := range values {
			if visitor.contains(value) {
				groups[meta.group] = struct{}{}
				break
			}
		}
	}
	if len(groups) == 0 {
		return values
	}

	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	if source == funcMetaChannel {
		for group := range groups {
			if group == nil || group.pending == 0 {
				delete(groups, group)
				continue
			}
			group.pending--
			group.received = true
			group.root = f.root
			if group.pending != 0 {
				delete(groups, group)
				continue
			}
			interp.makeChannelGroupVisibleLocked(group, f.root)
			delete(groups, group)
		}
	}
	seenOwner := map[*frame]struct{}{}
	for key, meta := range interp.funcMeta {
		if _, ok := groups[meta.group]; !ok {
			continue
		}
		old := meta.frame
		if old == nil || (meta.retention != source && (old == old.root || old.funcEscape != source)) {
			continue
		}
		meta.frame = f
		meta.retention = funcMetaVisible
		interp.funcMeta[key] = meta
		f.funcMeta = append(f.funcMeta, key)
		if old != nil && old != old.root {
			seenOwner[old] = struct{}{}
		}
	}
	for owner := range seenOwner {
		owner.funcEscape = funcMetaVisible
	}
	return values
}

func (interp *Interpreter) makeChannelGroupVisibleLocked(group *funcMetaGroup, receivingRoot *frame) {
	for key, meta := range interp.funcMeta {
		if meta.group != group {
			continue
		}
		old := meta.frame
		if old == nil {
			continue
		}
		root := receivingRoot
		if root == nil {
			root = old.root
		}
		meta.frame = root
		meta.retention = funcMetaVisible
		interp.funcMeta[key] = meta
		root.funcMeta = append(root.funcMeta, key)
		if old != root && old.funcEscape == funcMetaChannel {
			old.funcEscape = funcMetaVisible
		}
	}
}

// preserveReturnedInterpretedFuncs protects groups which cross the Eval API.
// The host may retain and later return these values after a canceled root was
// detached, so they cannot participate in interpreter-visible root sweeping.
func (interp *Interpreter) preserveReturnedInterpretedFuncs(value reflect.Value) {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.preserveReturnedInterpretedFuncsLocked(value)
}

func (interp *Interpreter) preserveReturnedInterpretedFuncsLocked(value reflect.Value) {
	if !funcBearingValue(value) {
		return
	}
	interp.funcMu.RLock()
	entries := make(map[uintptr]interpretedFuncMeta, len(interp.funcMeta))
	for key, meta := range interp.funcMeta {
		entries[key] = meta
	}
	interp.funcMu.RUnlock()

	collector := newFuncValueCollector()
	collector.collect(value)
	groups := map[*funcMetaGroup]struct{}{}
	for key, meta := range entries {
		if collector.exactContains(key) {
			groups[meta.group] = struct{}{}
		}
	}
	if len(groups) == 0 {
		return
	}
	interp.funcMu.Lock()
	for key, meta := range interp.funcMeta {
		if _, ok := groups[meta.group]; ok {
			meta.retention = funcMetaOpaque
			interp.funcMeta[key] = meta
		}
	}
	interp.funcMu.Unlock()
}

// refreshGlobalVarIndexesLocked publishes stable symbol and variable-slot
// views. The caller holds compileMu, which serializes every mutation of the
// compiler-owned source-package symbol maps.
func (interp *Interpreter) refreshGlobalVarIndexesLocked() {
	indexes := map[int]struct{}{}
	published := imports{}
	interp.mutex.RLock()
	for path, pkg := range interp.srcPkg {
		symbols := map[string]*symbol{}
		for name, sym := range pkg {
			if sym != nil {
				copy := *sym
				symbols[name] = &copy
			}
			if sym != nil && sym.global && sym.kind == varSym && sym.index >= 0 {
				indexes[sym.index] = struct{}{}
			}
		}
		published[path] = symbols
	}
	interp.mutex.RUnlock()
	interp.mutex.Lock()
	interp.globalVarIndexes = indexes
	interp.publishedSrcPkg = published
	interp.mutex.Unlock()
}

func (interp *Interpreter) snapshotGlobalVarIndexes() map[int]struct{} {
	interp.mutex.RLock()
	indexes := make(map[int]struct{}, len(interp.globalVarIndexes))
	for index := range interp.globalVarIndexes {
		indexes[index] = struct{}{}
	}
	interp.mutex.RUnlock()
	return indexes
}

// preservePossiblyReturnedInterpretedFuncs is the conservative fallback when
// another interpreted activation prevents a quiescent graph scan. Scalar
// results cannot contain callbacks. For a function-bearing result, retain the
// visible groups because the host may keep a wrapper after this Eval returns.
func (interp *Interpreter) preservePossiblyReturnedInterpretedFuncs(result reflect.Value) {
	if !funcBearingValue(result) {
		return
	}
	interp.funcMu.Lock()
	for key, meta := range interp.funcMeta {
		if meta.retention == funcMetaVisible {
			meta.retention = funcMetaOpaque
			interp.funcMeta[key] = meta
		}
	}
	interp.funcMu.Unlock()
}

// PurgeRetainedFuncs removes escape metadata retained for function values
// which crossed the Eval API (as results, channel sends, or panics) and are no
// longer reachable from package-level variables. It returns the number of
// metadata entries removed and is idempotent.
//
// Experimental. Values reachable from package-level variables are never
// purged and keep re-binding. A purged value remains callable (a self-contained
// MakeFunc wrapper) but permanently executes against its original root: after
// a later cancel/detach its writes land in the abandoned root and lose
// channel-ownership tracking.
//
// Entries whose wrapper the host dropped entirely are reclaimed automatically
// by the registry's weak finalizers; the purge handles wrappers the host
// retains or that remain pinned by interpreter-side cells (e.g. the last Eval
// result stored in a root frame cell).
//
// WARNING: do not call from interpreted code, from a host callback of a
// running evaluation, or while paused under the debugger. Called from an
// unrelated goroutine it blocks until evaluations quiesce.
func (interp *Interpreter) PurgeRetainedFuncs() int {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()

	interp.mutex.RLock()
	root := interp.frame
	interp.mutex.RUnlock()

	// Group-scoped snapshot: aliases share meta.group, and a purge scoped to
	// single keys could be resurrected through an alias's convertible-type
	// lookup, so candidates, members, and eligibility are tracked per group.
	// Groups pinned by undelivered channel sends or live panic tokens are
	// skipped.
	interp.funcMu.RLock()
	candidates := map[uintptr]interpretedFuncMeta{}
	groupMembers := map[*funcMetaGroup][]uintptr{}
	groupCaptures := map[*funcMetaGroup][]funcMetaCapture{}
	groupVersions := map[*funcMetaGroup]uint64{}
	eligible := map[*funcMetaGroup]bool{}
	for key, meta := range interp.funcMeta {
		if meta.retention != funcMetaOpaque || meta.frame == nil {
			continue
		}
		candidates[key] = meta
		groupMembers[meta.group] = append(groupMembers[meta.group], key)
		if _, seen := groupVersions[meta.group]; seen {
			continue
		}
		groupVersions[meta.group] = 0
		if meta.group != nil {
			groupVersions[meta.group] = meta.group.version
			groupCaptures[meta.group] = append([]funcMetaCapture(nil), meta.group.captures...)
			eligible[meta.group] = meta.group.pending == 0 && len(meta.group.panicTokens) == 0
		} else {
			eligible[meta.group] = true
		}
	}
	// A group referenced by a send which has not reached a terminal state must
	// stay pinned until the value is delivered or the send is retired.
	for _, channel := range interp.ownedChannels {
		for _, send := range channel.sends {
			if send.state == ownedChannelSendTerminal {
				continue
			}
			for group := range send.groups {
				eligible[group] = false
			}
			for group := range send.pendingGroups {
				eligible[group] = false
			}
		}
	}
	interp.funcMu.RUnlock()
	if len(candidates) == 0 {
		return 0
	}

	// Anchors: the durable global cells of the current root. directFuncs
	// activations are deliberately not anchors — they are a private clone
	// cache, not a package-level variable, and would otherwise keep a dropped
	// wrapper's clone group alive forever.
	indexes := interp.snapshotGlobalVarIndexes()
	root.mutex.RLock()
	values := make([]reflect.Value, 0, len(indexes))
	for index := range indexes {
		if index < len(root.data) {
			values = append(values, root.data[index])
		}
	}
	root.mutex.RUnlock()

	collector := newFuncValueCollector()
	for _, value := range values {
		collector.collect(value)
		if collector.anyAmbiguous {
			break
		}
	}
	liveGroups := map[*funcMetaGroup]struct{}{}
	if collector.anyAmbiguous {
		for group := range groupVersions {
			liveGroups[group] = struct{}{}
		}
	} else {
		for key, meta := range candidates {
			if collector.exactContains(key) {
				liveGroups[meta.group] = struct{}{}
			}
		}
		// One reachable closure can itself capture another wrapper opaquely;
		// close over the captured cells until the live set stops growing.
		for changed := true; changed && !collector.anyAmbiguous; {
			changed = false
			for group := range liveGroups {
				for _, capture := range groupCaptures[group] {
					value, ok := snapshotFuncMetaCapture(capture)
					if !ok {
						continue
					}
					collector.collect(value)
					if collector.anyAmbiguous {
						for candidateGroup := range groupVersions {
							liveGroups[candidateGroup] = struct{}{}
						}
						changed = false
						break
					}
					for key, meta := range candidates {
						if _, live := liveGroups[meta.group]; live {
							continue
						}
						if collector.exactContains(key) {
							liveGroups[meta.group] = struct{}{}
							changed = true
						}
					}
				}
				if collector.anyAmbiguous {
					break
				}
			}
		}
	}

	removed := 0
	deletedKeys := map[uintptr]struct{}{}
	affectedFrames := map[*frame]struct{}{}
	interp.funcMu.Lock()
	for group, isEligible := range eligible {
		if !isEligible {
			continue
		}
		if _, live := liveGroups[group]; live {
			continue
		}
		if group != nil && group.version != groupVersions[group] {
			// The group captured or registered members after the snapshot;
			// leave it intact rather than splitting an alias group.
			continue
		}
		if group != nil && (group.pending != 0 || len(group.panicTokens) != 0) {
			// Re-check eligibility under the delete lock: panic-token
			// memberships attach under funcMu without bumping the version,
			// so a token attached after the snapshot would otherwise be
			// missed and a group pinned by a live panic could be purged.
			continue
		}
		for _, key := range groupMembers[group] {
			meta, ok := interp.funcMeta[key]
			if !ok || meta.group != group || meta.retention != funcMetaOpaque || meta.frame == nil {
				continue
			}
			delete(interp.funcMeta, key)
			deletedKeys[key] = struct{}{}
			removed++
			affectedFrames[meta.frame] = struct{}{}
		}
	}
	// Drop directFuncs activations whose source or cloned value lost its
	// metadata: such an entry can never again resolve to discoverable
	// metadata, and keeping either endpoint would anchor the other forever.
	for key, activation := range interp.directFuncs {
		if activation.key == 0 {
			continue
		}
		if _, deleted := deletedKeys[key.source]; !deleted {
			if _, deleted := deletedKeys[activation.key]; !deleted {
				continue
			}
		}
		delete(interp.directFuncs, key)
	}
	for affected := range affectedFrames {
		affected.funcMeta = affected.funcMeta[:0]
		for key, meta := range interp.funcMeta {
			if meta.frame == affected {
				affected.funcMeta = append(affected.funcMeta, key)
			}
		}
	}
	interp.funcMu.Unlock()

	if removed == 0 {
		return 0
	}
	// Captures may have been pinned only by frames whose metadata this purge
	// deleted; re-run the owned-object sweep per affected root generation to
	// release them. unregisterOwnedObjectLocked keeps hostSharedEstimate exact.
	affectedRoots := map[*frame]struct{}{}
	for affected := range affectedFrames {
		if affected.root != nil {
			affectedRoots[affected.root] = struct{}{}
		}
	}
	for affectedRoot := range affectedRoots {
		interp.sweepRootOwnedObjectsLocked(affectedRoot)
	}
	return removed
}

// sweepRootInterpretedFuncs removes only interpreter-visible root metadata.
// Opaque API returns and channel groups which have not yet been received are
// intentionally excluded. Package-global symbol slots are the durable roots;
// temporary CFG slots must not keep consumed callbacks alive indefinitely.
func (interp *Interpreter) sweepRootInterpretedFuncs(root *frame, result reflect.Value) {
	if root == nil {
		return
	}
	indexes := interp.snapshotGlobalVarIndexes()

	locked := false
	for attempt := 0; attempt < 64; attempt++ {
		if interp.funcSweepMu.TryLock() {
			locked = true
			break
		}
		runtime.Gosched()
	}
	if !locked {
		// The exclusive fence stayed contended (a worker is executing or
		// unwinding in native code). Preserve only what the skipped quiescent
		// scan could have lost this round: a function-bearing result the host
		// may still hold must not stay visible, or the next uncontended sweep
		// would delete its metadata. Scalar results keep the cheap skip.
		interp.preservePossiblyReturnedInterpretedFuncs(result)
		return
	}
	defer interp.funcSweepMu.Unlock()
	interp.preserveReturnedInterpretedFuncsLocked(result)

	interp.funcMu.RLock()
	candidates := map[uintptr]interpretedFuncMeta{}
	groupCaptures := map[*funcMetaGroup][]funcMetaCapture{}
	groupVersions := map[*funcMetaGroup]uint64{}
	directValues := []reflect.Value{}
	for key, meta := range interp.funcMeta {
		if meta.frame == root && meta.retention == funcMetaVisible {
			candidates[key] = meta
			if meta.group != nil {
				if _, ok := groupVersions[meta.group]; !ok {
					groupCaptures[meta.group] = append([]funcMetaCapture(nil), meta.group.captures...)
					groupVersions[meta.group] = meta.group.version
				}
			}
		}
	}
	for key, activation := range interp.directFuncs {
		if key.root == root {
			directValues = append(directValues, activation.value)
		}
	}
	interp.funcMu.RUnlock()
	if len(candidates) == 0 {
		return
	}

	root.mutex.RLock()
	values := make([]reflect.Value, 0, len(indexes))
	for index := range indexes {
		if index < len(root.data) {
			values = append(values, root.data[index])
		}
	}
	root.mutex.RUnlock()
	values = append(values, directValues...)

	// One walk of the seed values replaces one containment walk per candidate.
	// An ambiguous func-capable container keeps every candidate alive, matching
	// funcValueVisitor's ptr/slice/map ambiguity rules.
	collector := newFuncValueCollector()
	for _, value := range values {
		collector.collect(value)
		if collector.anyAmbiguous {
			break
		}
	}
	liveGroups := map[*funcMetaGroup]struct{}{}
	if collector.anyAmbiguous {
		for _, meta := range candidates {
			liveGroups[meta.group] = struct{}{}
		}
	} else {
		for key, meta := range candidates {
			if collector.exactContains(key) {
				liveGroups[meta.group] = struct{}{}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for group := range liveGroups {
			for _, capture := range groupCaptures[group] {
				value, ok := snapshotFuncMetaCapture(capture)
				if !ok {
					continue
				}
				for key, meta := range candidates {
					if _, live := liveGroups[meta.group]; live {
						continue
					}
					visitor := funcValueVisitor{targets: map[uintptr]struct{}{key: {}}}
					if visitor.possiblyContains(value) {
						liveGroups[meta.group] = struct{}{}
						changed = true
					}
				}
			}
		}
	}

	interp.deleteUnreachableRootFuncMeta(root, candidates, liveGroups, groupVersions)
}

func snapshotFuncMetaCapture(capture funcMetaCapture) (reflect.Value, bool) {
	if capture.frame == nil {
		return reflect.Value{}, false
	}
	if capture.frame.interp != nil {
		capture.frame.interp.funcMu.RLock()
		defer capture.frame.interp.funcMu.RUnlock()
	}
	capture.frame.mutex.RLock()
	defer capture.frame.mutex.RUnlock()
	if capture.index < 0 || capture.index >= len(capture.frame.data) {
		return reflect.Value{}, false
	}
	value := capture.frame.data[capture.index]
	if !value.IsValid() {
		return reflect.Value{}, false
	}
	if capture.frame.interp != nil && capture.frame.interp.ownedCellHostSharedLocked(value) {
		return reflect.Value{}, false
	}
	snapshot := reflect.New(value.Type()).Elem()
	snapshot.Set(value)
	return snapshot, true
}

func (interp *Interpreter) deleteUnreachableRootFuncMeta(root *frame, candidates map[uintptr]interpretedFuncMeta, liveGroups map[*funcMetaGroup]struct{}, groupVersions map[*funcMetaGroup]uint64) {
	interp.funcMu.Lock()
	for key, candidate := range candidates {
		meta, ok := interp.funcMeta[key]
		if !ok || meta.frame != root || meta.retention != funcMetaVisible || meta.group != candidate.group {
			continue
		}
		if meta.group != nil && meta.group.version != groupVersions[meta.group] {
			continue
		}
		if _, live := liveGroups[meta.group]; !live {
			delete(interp.funcMeta, key)
		}
	}
	root.funcMeta = root.funcMeta[:0]
	for key, meta := range interp.funcMeta {
		if meta.frame == root {
			root.funcMeta = append(root.funcMeta, key)
		}
	}
	interp.funcMu.Unlock()
}

func frameReturnsFunction(f *frame, funcNode *node, targets map[uintptr]struct{}) bool {
	if funcNode == nil || funcNode.typ == nil {
		return false
	}
	numRet := len(funcNode.typ.ret)
	if numRet > len(f.data) {
		numRet = len(f.data)
	}
	for _, value := range f.data[:numRet] {
		visitor := funcValueVisitor{targets: targets}
		if visitor.contains(value) {
			return true
		}
	}
	return false
}

func funcsReachableFromAncestors(f *frame, targets map[uintptr]struct{}) *frame {
	seenFrames := map[*frame]struct{}{}
	for ancestor := f.anc; ancestor != nil; ancestor = ancestor.anc {
		if funcsReachableFromFrame(ancestor, targets, seenFrames) {
			return ancestor
		}
	}
	if funcsReachableFromFrame(f.root, targets, seenFrames) {
		return f.root
	}
	return nil
}

func funcsReachableFromFrame(f *frame, targets map[uintptr]struct{}, seenFrames map[*frame]struct{}) bool {
	if f == nil {
		return false
	}
	if _, ok := seenFrames[f]; ok {
		return false
	}
	seenFrames[f] = struct{}{}
	if f.cloneOf != nil {
		f.interp.funcMu.RLock()
		_, carrierRegistered := f.interp.funcMeta[f.funcCarrier]
		cloneOwnerFinished := f.cloneOf.funcState == funcFrameFinished
		f.interp.funcMu.RUnlock()
		if f.funcCarrier != 0 && cloneOwnerFinished && !carrierRegistered {
			// The only remaining owner of this clone is its currently executing
			// wrapper. Once that call unwinds, values reachable only through the
			// clone cannot escape and must not be promoted to the root.
			return false
		}
	}

	f.mutex.RLock()
	data := append([]reflect.Value(nil), f.data...)
	f.mutex.RUnlock()
	visitor := funcValueVisitor{targets: targets}
	for _, value := range data {
		if visitor.contains(value) {
			return true
		}
	}
	return false
}

type funcValueVisitor struct {
	targets map[uintptr]struct{}
}

type funcValueMatch uint8

const (
	funcValueNoMatch funcValueMatch = iota
	funcValueExactMatch
	funcValueAmbiguousMatch
)

func (v *funcValueVisitor) contains(value reflect.Value) bool {
	return v.match(value) == funcValueExactMatch
}

func (v *funcValueVisitor) possiblyContains(value reflect.Value) bool {
	return v.match(value) != funcValueNoMatch
}

func (v *funcValueVisitor) match(value reflect.Value) funcValueMatch {
	if !value.IsValid() {
		return funcValueNoMatch
	}
	if value.CanInterface() {
		switch wrapped := value.Interface().(type) {
		case valueInterface:
			return v.match(wrapped.value)
		case reflect.Value:
			return v.match(wrapped)
		}
	}

	switch value.Kind() {
	case reflect.Func:
		if value.IsNil() {
			return funcValueNoMatch
		}
		key, ok := funcvalKeyOf(value)
		if !ok {
			return funcValueAmbiguousMatch
		}
		_, ok = v.targets[key]
		if ok {
			return funcValueExactMatch
		}
		return funcValueNoMatch
	case reflect.Interface:
		if value.IsNil() {
			return funcValueNoMatch
		}
		return v.match(value.Elem())
	case reflect.Ptr:
		if value.IsNil() || !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return funcValueNoMatch
		}
		// Native code can retain and mutate reference-backed values while an
		// interpreted call is canceled. Recursively reading them would race that
		// mutation even under the interpreter execution fence. Treat any non-nil,
		// function-capable reference as an ambiguous match for every candidate.
		return funcValueAmbiguousMatch
	case reflect.Struct:
		if !typeMayContainFunc(value.Type(), map[reflect.Type]bool{}) {
			return funcValueNoMatch
		}
		match := funcValueNoMatch
		for i := 0; i < value.NumField(); i++ {
			fieldMatch := v.match(value.Field(i))
			if fieldMatch == funcValueExactMatch {
				return fieldMatch
			}
			if fieldMatch == funcValueAmbiguousMatch {
				match = fieldMatch
			}
		}
		return match
	case reflect.Array:
		if !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return funcValueNoMatch
		}
		match := funcValueNoMatch
		for i := 0; i < value.Len(); i++ {
			elementMatch := v.match(value.Index(i))
			if elementMatch == funcValueExactMatch {
				return elementMatch
			}
			if elementMatch == funcValueAmbiguousMatch {
				match = elementMatch
			}
		}
		return match
	case reflect.Slice:
		if value.IsNil() || !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return funcValueNoMatch
		}
		return funcValueAmbiguousMatch
	case reflect.Map:
		if value.IsNil() || (!typeMayContainFunc(value.Type().Key(), map[reflect.Type]bool{}) &&
			!typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{})) {
			return funcValueNoMatch
		}
		return funcValueAmbiguousMatch
	}
	return funcValueNoMatch
}

func typeMayContainFunc(typ reflect.Type, seen map[reflect.Type]bool) bool {
	if typ == nil {
		return false
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch typ.Kind() {
	case reflect.Func, reflect.Interface:
		return true
	case reflect.Ptr, reflect.Array, reflect.Slice:
		return typeMayContainFunc(typ.Elem(), seen)
	case reflect.Map:
		return typeMayContainFunc(typ.Key(), seen) || typeMayContainFunc(typ.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if typeMayContainFunc(typ.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

// funcBearingValue reports whether a value's static type may hide a function.
// The boxed value wrappers must always be walked: their content is untyped at
// the call boundary even though their struct fields contain no function type.
func funcBearingValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	typ := value.Type()
	if typ == valueInterfaceType || typ == reflectedValueType {
		return true
	}
	return typeMayContainFunc(typ, map[reflect.Type]bool{})
}

// funcValueCollector gathers the exact function values reachable through a
// value graph, keyed by funcval identity, and records whether an ambiguous
// func-capable container was crossed. Traversal mirrors
// funcValueVisitor.match: boxed and interface values are unwrapped, structs
// and arrays are descended when their type may contain a function, and
// non-nil pointers, slices, and maps are never read recursively (native code
// may retain and mutate them while an interpreted call is canceled), so they
// only set anyAmbiguous.
type funcValueCollector struct {
	exact        map[uintptr]struct{}
	anyAmbiguous bool
}

func newFuncValueCollector() *funcValueCollector {
	return &funcValueCollector{exact: map[uintptr]struct{}{}}
}

// exactContains reports whether the funcval key was found in the collected
// graph. A candidate is live iff exactContains or anyAmbiguous, which
// reproduces funcValueVisitor.possiblyContains.
func (c *funcValueCollector) exactContains(key uintptr) bool {
	_, ok := c.exact[key]
	return ok
}

func (c *funcValueCollector) collect(value reflect.Value) {
	if !value.IsValid() {
		return
	}
	if value.CanInterface() {
		switch wrapped := value.Interface().(type) {
		case valueInterface:
			c.collect(wrapped.value)
			return
		case reflect.Value:
			c.collect(wrapped)
			return
		}
	}

	switch value.Kind() {
	case reflect.Func:
		if value.IsNil() {
			return
		}
		key, ok := funcvalKeyOf(value)
		if !ok {
			c.anyAmbiguous = true
			return
		}
		c.exact[key] = struct{}{}
	case reflect.Interface:
		if value.IsNil() {
			return
		}
		c.collect(value.Elem())
	case reflect.Ptr:
		if value.IsNil() || !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return
		}
		c.anyAmbiguous = true
	case reflect.Struct:
		if !typeMayContainFunc(value.Type(), map[reflect.Type]bool{}) {
			return
		}
		for i := 0; i < value.NumField(); i++ {
			c.collect(value.Field(i))
		}
	case reflect.Array:
		if !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return
		}
		for i := 0; i < value.Len(); i++ {
			c.collect(value.Index(i))
		}
	case reflect.Slice:
		if value.IsNil() || !typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{}) {
			return
		}
		c.anyAmbiguous = true
	case reflect.Map:
		if value.IsNil() || (!typeMayContainFunc(value.Type().Key(), map[reflect.Type]bool{}) &&
			!typeMayContainFunc(value.Type().Elem(), map[reflect.Type]bool{})) {
			return
		}
		c.anyAmbiguous = true
	}
}
