package interp

import (
	"math"
	"reflect"
	"sort"
	"unsafe"
)

var reflectedValueType = reflect.TypeOf(reflect.Value{})

// objectKey is only a lookup label. ownedObject.hold keeps the allocation
// alive, so an address cannot be reused by a later native allocation while the
// tag exists.
type objectKey struct {
	kind reflect.Kind
	typ  reflect.Type
	ptr  uintptr
}

type ownedObject struct {
	key          objectKey
	hold         reflect.Value
	owner        *frame
	channelRefs  int
	panicTokens  map[*ownedPanicToken]struct{}
	channelSends map[*ownedChannelSend]struct{}
	hostShared   bool
	pendingRoot  *frame
	pending      reflect.Value

	sliceElem reflect.Type
	sliceCap  int
	sliceBase uintptr
	sliceEnd  uintptr
}

// ownedPanicToken records the exact object graph retained by one interpreted
// panic. The panic value may be a mutable aggregate, so recovery and
// replacement must not rediscover that graph after user defers have changed
// it. finished makes every terminal path idempotent.
type ownedPanicToken struct {
	value          reflect.Value
	objects        map[*ownedObject]struct{}
	funcs          map[reflect.Value]struct{}
	groups         map[*funcMetaGroup]struct{}
	frames         map[*frame]struct{}
	pendingRoot    *frame
	pending        reflect.Value
	pendingObjects map[*ownedObject]struct{}
	pendingFuncs   map[reflect.Value]struct{}
	pendingGroups  map[*funcMetaGroup]struct{}
	pendingFrames  map[*frame]struct{}
	finished       bool
}

type interpretedPanic struct {
	value interface{}
	token *ownedPanicToken
}

func splitInterpretedPanic(recovered interface{}) (interface{}, *ownedPanicToken) {
	if state, ok := recovered.(*interpretedPanic); ok && state != nil {
		return state.value, state.token
	}
	return recovered, nil
}

func ownedObjectHasPanicToken(obj *ownedObject) bool {
	return obj != nil && len(obj.panicTokens) != 0
}

type ownedChannelSendState uint8

const (
	ownedChannelSendPrepared ownedChannelSendState = iota
	ownedChannelSendDelivered
	ownedChannelSendTerminal
)

type ownedChannelSend struct {
	channel        *ownedChannel
	value          reflect.Value
	objects        map[*ownedObject]struct{}
	funcs          map[reflect.Value]struct{}
	groups         map[*funcMetaGroup]struct{}
	matchObjects   map[*ownedObject]struct{}
	matchFuncs     map[reflect.Value]struct{}
	matchSignature []ownedChannelMatchAtom
	sender         *frame
	state          ownedChannelSendState
	pendingRoot    *frame
	pending        reflect.Value
	pendingObjects map[*ownedObject]struct{}
	pendingFuncs   map[reflect.Value]struct{}
	pendingGroups  map[*funcMetaGroup]struct{}
}

type ownedChannelMatchAtom struct {
	kind reflect.Kind
	typ  reflect.Type
	ptr  uintptr
	lo   uint64
	hi   uint64
	text string
}

type ownedChannel struct {
	hold        reflect.Value
	owner       *frame
	hostVisible bool
	sends       []*ownedChannelSend
}

func ownedChannelPointer(v reflect.Value) uintptr {
	v = unwrapOwnedValue(v)
	if !v.IsValid() || v.Kind() != reflect.Chan || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

func unwrapOwnedValue(v reflect.Value) reflect.Value {
	for v.IsValid() {
		if v.Type() == reflectedValueType && v.CanInterface() {
			v = v.Interface().(reflect.Value)
			continue
		}
		if v.Type() == valueInterfaceType && v.CanInterface() {
			v = v.Interface().(valueInterface).value
			continue
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
			continue
		}
		break
	}
	return v
}

func describeOwnedObject(v reflect.Value, owner *frame) (*ownedObject, bool) {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return nil, false
	}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return nil, false
		}
		return &ownedObject{
			key:   objectKey{kind: v.Kind(), ptr: v.Pointer()},
			hold:  v,
			owner: owner,
		}, true
	case reflect.Ptr:
		if v.IsNil() {
			return nil, false
		}
		return &ownedObject{
			key:   objectKey{kind: v.Kind(), typ: v.Type(), ptr: v.Pointer()},
			hold:  v,
			owner: owner,
		}, true
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 || v.Type().Elem().Size() == 0 {
			// Reflection cannot distinguish zero-size backing allocations. They
			// carry no mutable element storage, so leave them opaque.
			return nil, false
		}
		full := v.Slice(0, v.Cap())
		base := full.Pointer()
		size := v.Type().Elem().Size()
		return &ownedObject{
			key:       objectKey{kind: reflect.Slice, typ: v.Type(), ptr: base},
			hold:      full,
			owner:     owner,
			sliceElem: v.Type().Elem(),
			sliceCap:  v.Cap(),
			sliceBase: base,
			sliceEnd:  base + uintptr(v.Cap())*size,
		}, true
	}
	return nil, false
}

func (interp *Interpreter) registerOwnedValue(v reflect.Value, owner *frame) {
	obj, ok := describeOwnedObject(v, owner)
	if !ok {
		return
	}
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	if v.Kind() == reflect.Ptr && len(interp.ownedObjectsForValueLocked(v)) != 0 {
		return
	}
	if v.Kind() != reflect.Ptr && interp.ownedObjectLocked(v) != nil {
		return
	}
	interp.ownedObjects[obj.key] = obj
	interp.armOwnedGCLocked()
	if owner != nil {
		if owner.ownedObjects == nil {
			owner.ownedObjects = map[*ownedObject]struct{}{}
		}
		owner.ownedObjects[obj] = struct{}{}
	}
}

func (interp *Interpreter) registerOwnedChannel(v reflect.Value, owner *frame) {
	ptr := ownedChannelPointer(v)
	if ptr == 0 {
		return
	}
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	if _, exists := interp.ownedChannels[ptr]; exists {
		return
	}
	channel := &ownedChannel{hold: v, owner: owner}
	interp.ownedChannels[ptr] = channel
	interp.armOwnedGCLocked()
	if owner != nil {
		if owner.ownedChannels == nil {
			owner.ownedChannels = map[*ownedChannel]struct{}{}
		}
		owner.ownedChannels[channel] = struct{}{}
	}
}

func (interp *Interpreter) ownedChannelLocked(v reflect.Value) *ownedChannel {
	return interp.ownedChannels[ownedChannelPointer(v)]
}

func (interp *Interpreter) registerOwnedAppend(result, source reflect.Value, owner *frame) {
	result = unwrapOwnedValue(result)
	source = unwrapOwnedValue(source)
	if !result.IsValid() || result.Kind() != reflect.Slice || result.IsNil() || result.Cap() == 0 {
		return
	}
	interp.funcMu.RLock()
	sourceOwned := interp.ownedObjectLocked(source) != nil
	resultOwned := interp.ownedObjectLocked(result) != nil
	interp.funcMu.RUnlock()
	if resultOwned {
		return
	}
	reallocated := !source.IsValid() || source.Kind() != reflect.Slice || source.IsNil() || source.Cap() == 0 || source.Pointer() != result.Pointer()
	if sourceOwned || reallocated {
		interp.registerOwnedValue(result, owner)
	}
}

func valueContainsAddress(value reflect.Value, ptr uintptr) bool {
	if !value.IsValid() || !value.CanAddr() {
		return false
	}
	base := value.Addr().Pointer()
	size := value.Type().Size()
	if size == 0 {
		return ptr == base
	}
	return ptr >= base && ptr < base+size
}

// registerOwnedAddress tags an address only when its storage provenance is an
// interpreter frame cell or an already-owned allocation. Taking a field
// address through a native-returned pointer must remain native and shallow.
func (interp *Interpreter) registerOwnedAddress(address reflect.Value, f *frame) {
	if !address.IsValid() || address.Kind() != reflect.Ptr || address.IsNil() {
		return
	}
	ptr := address.Pointer()
	owned := false
	seenFrames := map[*frame]struct{}{}
	for current := f; current != nil; current = current.anc {
		if _, seen := seenFrames[current]; seen {
			break
		}
		seenFrames[current] = struct{}{}
		for _, value := range current.data {
			if valueContainsAddress(value, ptr) {
				owned = true
				break
			}
		}
		if owned {
			break
		}
	}
	interp.funcMu.RLock()
	if !owned {
		for _, obj := range interp.ownedObjects {
			switch obj.key.kind {
			case reflect.Ptr:
				base := obj.key.ptr
				size := obj.hold.Type().Elem().Size()
				owned = ptr >= base && (size == 0 && ptr == base || size > 0 && ptr < base+size)
			case reflect.Slice:
				owned = ptr >= obj.sliceBase && ptr < obj.sliceEnd
			}
			if owned {
				break
			}
		}
	}
	interp.funcMu.RUnlock()
	if owned {
		interp.registerOwnedValue(address, f)
	}
}

func (interp *Interpreter) ownedObjectLocked(v reflect.Value) *ownedObject {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		return interp.ownedObjects[objectKey{kind: v.Kind(), ptr: v.Pointer()}]
	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		if obj := interp.ownedObjects[objectKey{kind: v.Kind(), typ: v.Type(), ptr: v.Pointer()}]; obj != nil {
			return obj
		}
		for _, obj := range interp.ownedObjects {
			if obj.key.kind == reflect.Ptr && obj.key.ptr == v.Pointer() && obj.hold.Type().ConvertibleTo(v.Type()) {
				return obj
			}
		}
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 || v.Type().Elem().Size() == 0 {
			return nil
		}
		ptr := v.Pointer()
		end := ptr + uintptr(v.Cap())*v.Type().Elem().Size()
		for _, obj := range interp.ownedObjects {
			if obj.key.kind == reflect.Slice && obj.sliceElem == v.Type().Elem() &&
				ptr >= obj.sliceBase && end <= obj.sliceEnd {
				return obj
			}
		}
	}
	return nil
}

func ownedObjectInterval(obj *ownedObject) (uintptr, uintptr, bool) {
	if obj == nil {
		return 0, 0, false
	}
	switch obj.key.kind {
	case reflect.Ptr:
		size := obj.hold.Type().Elem().Size()
		if size == 0 {
			return obj.key.ptr, obj.key.ptr + 1, true
		}
		return obj.key.ptr, obj.key.ptr + size, true
	case reflect.Slice:
		return obj.sliceBase, obj.sliceEnd, true
	}
	return 0, 0, false
}

func ownedObjectsShareStorage(a, b *ownedObject) bool {
	if a == nil || b == nil {
		return false
	}
	if a.key.kind == reflect.Map || b.key.kind == reflect.Map {
		return a.key.kind == reflect.Map && b.key.kind == reflect.Map && a.key.ptr == b.key.ptr
	}
	ab, ae, aok := ownedObjectInterval(a)
	bb, be, bok := ownedObjectInterval(b)
	return aok && bok && ab < be && bb < ae
}

func (interp *Interpreter) markOwnedObjectHostSharedLocked(seed *ownedObject) {
	if seed == nil {
		return
	}
	// Propagate sharing with a worklist instead of an all-pairs fixpoint:
	// each object transitions at most once (hostShared is sticky), so the
	// total cost is one registry pass per newly shared object rather than
	// repeated full scans until quiescence. Seeding from every already
	// shared object re-closes the registry: an allocation registered after a
	// previous marking can still share storage with it.
	queue := make([]*ownedObject, 0, 8)
	if !seed.hostShared {
		seed.hostShared = true
		interp.hostSharedEstimate++
		queue = append(queue, seed)
	}
	for _, current := range interp.ownedObjects {
		if current.hostShared {
			queue = append(queue, current)
		}
	}
	for len(queue) > 0 {
		obj := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, current := range interp.ownedObjects {
			if current.hostShared {
				continue
			}
			if ownedObjectsShareStorage(current, obj) {
				current.hostShared = true
				interp.hostSharedEstimate++
				queue = append(queue, current)
			}
		}
	}
}

// unregisterOwnedObjectLocked removes an object from the interpreter registry
// while keeping the hostShared estimate exact. The caller holds funcMu.
func (interp *Interpreter) unregisterOwnedObjectLocked(obj *ownedObject) {
	if obj == nil {
		return
	}
	if obj.hostShared {
		interp.hostSharedEstimate--
	}
	delete(interp.ownedObjects, obj.key)
}

func (interp *Interpreter) publishOwnedChannelLocked(v reflect.Value) {
	ptr := ownedChannelPointer(v)
	if ptr == 0 {
		return
	}
	channel := interp.ownedChannels[ptr]
	if channel == nil {
		channel = &ownedChannel{hold: v}
		interp.ownedChannels[ptr] = channel
		interp.armOwnedGCLocked()
	}
	channel.hostVisible = true
	for _, send := range channel.sends {
		if send.state == ownedChannelSendTerminal {
			continue
		}
		interp.refreshOwnedChannelSendLocked(send)
		for value := range send.funcs {
			key, ok := funcvalKeyOf(value)
			if !ok {
				continue
			}
			meta, ok := interp.funcMeta[key]
			if !ok {
				continue
			}
			meta.retention = funcMetaOpaque
			if meta.frame != nil && meta.frame != meta.frame.root {
				meta.frame.funcEscape = funcMetaOpaque
			}
			interp.funcMeta[key] = meta
		}
		for obj := range send.objects {
			interp.markOwnedObjectHostSharedLocked(obj)
		}
		interp.retireOwnedChannelSendLocked(send)
		interp.releaseOwnedChannelSendFuncsLocked(send, send.funcs, nil, false)
		interp.releaseOwnedChannelSendFuncsLocked(send, send.pendingFuncs, nil, false)
	}
	channel.sends = nil
}

func (interp *Interpreter) recordOwnedChannelSendLocked(channelValue, value reflect.Value, sender *frame) *ownedChannelSend {
	channel := interp.ownedChannelLocked(channelValue)
	if channel == nil || channel.hostVisible {
		return nil
	}
	frozen := reflect.New(value.Type()).Elem()
	frozen.Set(value)
	send := &ownedChannelSend{
		channel: channel, value: frozen, sender: sender, state: ownedChannelSendPrepared,
		objects: map[*ownedObject]struct{}{}, funcs: map[reflect.Value]struct{}{}, groups: map[*funcMetaGroup]struct{}{},
		matchObjects: map[*ownedObject]struct{}{}, matchFuncs: map[reflect.Value]struct{}{},
	}
	send.matchSignature = ownedChannelValueSignature(frozen, nil)
	interp.collectOwnedChannelGraphLocked(frozen, send.objects, send.funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
	interp.collectOwnedChannelGraphLocked(frozen, send.matchObjects, send.matchFuncs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, true)
	interp.attachOwnedChannelSendMembershipsLocked(send)
	channel.sends = append(channel.sends, send)
	return send
}

func (interp *Interpreter) retireOwnedChannelSendLocked(send *ownedChannelSend) {
	if send == nil || send.state == ownedChannelSendTerminal || send.channel == nil {
		return
	}
	send.state = ownedChannelSendTerminal
	for obj := range send.objects {
		delete(obj.channelSends, send)
		if obj.channelRefs > 0 {
			obj.channelRefs--
		}
	}
	for obj := range send.pendingObjects {
		if _, already := send.objects[obj]; already {
			continue
		}
		delete(obj.channelSends, send)
		if obj.channelRefs > 0 {
			obj.channelRefs--
		}
	}
	for group := range send.groups {
		if group != nil && group.pending > 0 {
			group.pending--
		}
	}
	for group := range send.pendingGroups {
		if _, already := send.groups[group]; already {
			continue
		}
		if group != nil && group.pending > 0 {
			group.pending--
		}
	}
	for index, current := range send.channel.sends {
		if current == send {
			send.channel.sends = append(send.channel.sends[:index], send.channel.sends[index+1:]...)
			break
		}
	}
}

func (interp *Interpreter) consumeOwnedChannelSendLocked(channelValue reflect.Value, values ...reflect.Value) *ownedChannelSend {
	channel := interp.ownedChannelLocked(channelValue)
	if channel == nil {
		return nil
	}
	for _, send := range channel.sends {
		if send.state != ownedChannelSendTerminal && interp.ownedChannelSendMatchesLocked(send, values...) {
			return send
		}
	}
	return nil
}

func (interp *Interpreter) ownedChannelSendMatchesLocked(send *ownedChannelSend, values ...reflect.Value) bool {
	signature := []ownedChannelMatchAtom{}
	for _, value := range values {
		signature = ownedChannelValueSignature(value, signature)
	}
	if len(signature) != len(send.matchSignature) {
		return false
	}
	for index := range signature {
		if signature[index] != send.matchSignature[index] {
			return false
		}
	}
	return true
}

func ownedChannelValueSignature(v reflect.Value, signature []ownedChannelMatchAtom) []ownedChannelMatchAtom {
	for v.IsValid() && v.Type() == reflectedValueType && v.CanInterface() {
		v = v.Interface().(reflect.Value)
	}
	if !v.IsValid() {
		return append(signature, ownedChannelMatchAtom{kind: reflect.Invalid})
	}
	if v.Type() == valueInterfaceType && v.CanInterface() {
		signature = append(signature, ownedChannelMatchAtom{kind: reflect.Interface, typ: v.Type()})
		return ownedChannelValueSignature(v.Interface().(valueInterface).value, signature)
	}
	atom := ownedChannelMatchAtom{kind: v.Kind(), typ: v.Type()}
	switch v.Kind() {
	case reflect.Interface:
		signature = append(signature, atom)
		if v.IsNil() {
			return signature
		}
		return ownedChannelValueSignature(v.Elem(), signature)
	case reflect.Bool:
		if v.Bool() {
			atom.lo = 1
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		atom.lo = uint64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		atom.lo = v.Uint()
	case reflect.Float32:
		atom.lo = uint64(math.Float32bits(float32(v.Float())))
	case reflect.Float64:
		atom.lo = math.Float64bits(v.Float())
	case reflect.Complex64:
		value := complex64(v.Complex())
		atom.lo = uint64(math.Float32bits(real(value)))
		atom.hi = uint64(math.Float32bits(imag(value)))
	case reflect.Complex128:
		value := v.Complex()
		atom.lo = math.Float64bits(real(value))
		atom.hi = math.Float64bits(imag(value))
	case reflect.String:
		atom.text = v.String()
	case reflect.Func, reflect.Map, reflect.Ptr, reflect.Chan, reflect.UnsafePointer:
		if !v.IsNil() {
			atom.ptr = v.Pointer()
		}
	case reflect.Slice:
		if !v.IsNil() {
			atom.ptr = v.Pointer()
			atom.lo = uint64(v.Len())
			atom.hi = uint64(v.Cap())
		}
	case reflect.Struct:
		signature = append(signature, atom)
		for index := 0; index < v.NumField(); index++ {
			signature = ownedChannelValueSignature(v.Field(index), signature)
		}
		return signature
	case reflect.Array:
		signature = append(signature, atom)
		for index := 0; index < v.Len(); index++ {
			signature = ownedChannelValueSignature(v.Index(index), signature)
		}
		return signature
	}
	return append(signature, atom)
}

func (interp *Interpreter) collectOwnedChannelGraphLocked(v reflect.Value, objects map[*ownedObject]struct{}, funcs map[reflect.Value]struct{}, seenObjects map[*ownedObject]struct{}, seenFuncs map[reflect.Value]struct{}, topOnly bool) {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Func {
		key, ok := canonicalFuncValue(v)
		if !ok {
			return
		}
		meta, exists := interp.funcMeta[funcvalKey(key)]
		if !exists {
			return
		}
		funcs[key] = struct{}{}
		if topOnly {
			return
		}
		if _, seen := seenFuncs[key]; seen {
			return
		}
		seenFuncs[key] = struct{}{}
		for _, capture := range meta.captures {
			if capture.frame != nil && capture.index >= 0 && capture.index < len(capture.frame.data) {
				captured := capture.frame.data[capture.index]
				if !interp.ownedCellHostSharedLocked(captured) {
					interp.collectOwnedChannelGraphLocked(captured, objects, funcs, seenObjects, seenFuncs, false)
				}
			}
		}
		return
	}
	if (v.Kind() == reflect.Struct || v.Kind() == reflect.Array) && interp.ownedCellHostSharedLocked(v) {
		return
	}
	switch v.Kind() {
	case reflect.Map, reflect.Ptr, reflect.Slice:
		owned := interp.ownedObjectsForValueLocked(v)
		if len(owned) == 0 {
			return
		}
		for _, obj := range owned {
			objects[obj] = struct{}{}
		}
		if topOnly {
			return
		}
		for _, obj := range owned {
			if obj.hostShared {
				return
			}
			if _, seen := seenObjects[obj]; seen {
				return
			}
			seenObjects[obj] = struct{}{}
		}
		switch v.Kind() {
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				interp.collectOwnedChannelGraphLocked(iter.Key(), objects, funcs, seenObjects, seenFuncs, false)
				interp.collectOwnedChannelGraphLocked(iter.Value(), objects, funcs, seenObjects, seenFuncs, false)
			}
		case reflect.Ptr:
			interp.collectOwnedChannelGraphLocked(v.Elem(), objects, funcs, seenObjects, seenFuncs, false)
		case reflect.Slice:
			full := v.Slice(0, v.Cap())
			for index := 0; index < full.Len(); index++ {
				interp.collectOwnedChannelGraphLocked(full.Index(index), objects, funcs, seenObjects, seenFuncs, false)
			}
		}
	case reflect.UnsafePointer:
		for _, obj := range interp.ownedObjectsForValueLocked(v) {
			objects[obj] = struct{}{}
		}
	case reflect.Struct:
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() {
				interp.collectOwnedChannelGraphLocked(v.Field(index), objects, funcs, seenObjects, seenFuncs, topOnly)
			}
		}
	case reflect.Array:
		for index := 0; index < v.Len(); index++ {
			interp.collectOwnedChannelGraphLocked(v.Index(index), objects, funcs, seenObjects, seenFuncs, topOnly)
		}
	}
}

func (interp *Interpreter) attachOwnedChannelSendMembershipsLocked(send *ownedChannelSend) {
	for obj := range send.objects {
		if obj.channelSends == nil {
			obj.channelSends = map[*ownedChannelSend]struct{}{}
		}
		if _, exists := obj.channelSends[send]; !exists {
			obj.channelSends[send] = struct{}{}
			obj.channelRefs++
		}
	}
	for value := range send.funcs {
		key, ok := funcvalKeyOf(value)
		if !ok {
			continue
		}
		meta, exists := interp.funcMeta[key]
		if !exists || meta.group == nil {
			continue
		}
		if _, exists := send.groups[meta.group]; !exists {
			send.groups[meta.group] = struct{}{}
			meta.group.pending++
		}
		if meta.frame != nil && meta.frame != meta.frame.root {
			if meta.frame.funcEscape < funcMetaChannel {
				meta.frame.funcEscape = funcMetaChannel
			}
		} else if meta.retention < funcMetaChannel {
			meta.retention = funcMetaChannel
			interp.funcMeta[key] = meta
		}
	}
}

func (interp *Interpreter) refreshOwnedChannelSendLocked(send *ownedChannelSend) {
	interp.refreshOwnedChannelSendGraphLocked(send, false)
}

func (interp *Interpreter) refreshOwnedChannelSendGraphLocked(send *ownedChannelSend, pending bool) {
	if send == nil || send.state == ownedChannelSendTerminal {
		return
	}
	source := send.value
	if pending {
		if !send.pending.IsValid() {
			return
		}
		source = send.pending
	}
	objects := map[*ownedObject]struct{}{}
	funcs := map[reflect.Value]struct{}{}
	interp.collectOwnedChannelGraphLocked(source, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
	interp.replaceOwnedChannelSendGraphLocked(send, pending, objects, funcs, false)
}

func (interp *Interpreter) replaceOwnedChannelSendGraphLocked(send *ownedChannelSend, pending bool, objects map[*ownedObject]struct{}, funcs map[reflect.Value]struct{}, releaseReplaced bool) {
	if send == nil || send.state == ownedChannelSendTerminal {
		return
	}
	oldObjects, oldFuncs, oldGroups := send.objects, send.funcs, send.groups
	otherObjects, otherFuncs, otherGroups := send.pendingObjects, send.pendingFuncs, send.pendingGroups
	if pending {
		oldObjects, oldFuncs, oldGroups = send.pendingObjects, send.pendingFuncs, send.pendingGroups
		otherObjects, otherFuncs, otherGroups = send.objects, send.funcs, send.groups
	}
	groups := map[*funcMetaGroup]struct{}{}
	for value := range funcs {
		key, ok := funcvalKeyOf(value)
		if !ok {
			continue
		}
		meta, exists := interp.funcMeta[key]
		if !exists || meta.group == nil {
			continue
		}
		groups[meta.group] = struct{}{}
		if _, already := oldGroups[meta.group]; !already {
			if _, shared := otherGroups[meta.group]; !shared {
				meta.group.pending++
			}
		}
		if meta.frame != nil && meta.frame != meta.frame.root {
			if meta.frame.funcEscape < funcMetaChannel {
				meta.frame.funcEscape = funcMetaChannel
			}
		} else if meta.retention < funcMetaChannel {
			meta.retention = funcMetaChannel
			interp.funcMeta[key] = meta
		}
	}
	for obj := range objects {
		if obj.channelSends == nil {
			obj.channelSends = map[*ownedChannelSend]struct{}{}
		}
		if _, exists := obj.channelSends[send]; !exists {
			obj.channelSends[send] = struct{}{}
			obj.channelRefs++
		}
	}
	for obj := range oldObjects {
		if _, keep := objects[obj]; keep {
			continue
		}
		if _, shared := otherObjects[obj]; shared {
			continue
		}
		delete(obj.channelSends, send)
		if obj.channelRefs > 0 {
			obj.channelRefs--
		}
	}
	for group := range oldGroups {
		if _, keep := groups[group]; keep {
			continue
		}
		if _, shared := otherGroups[group]; shared {
			continue
		}
		if group != nil && group.pending > 0 {
			group.pending--
		}
	}
	if pending {
		send.pendingObjects, send.pendingFuncs, send.pendingGroups = objects, funcs, groups
	} else {
		send.objects, send.funcs, send.groups = objects, funcs, groups
	}
	staleFuncs := map[reflect.Value]struct{}{}
	for key := range oldFuncs {
		if _, keep := funcs[key]; keep {
			continue
		}
		if _, shared := otherFuncs[key]; shared {
			continue
		}
		staleFuncs[key] = struct{}{}
	}
	interp.releaseOwnedChannelSendFuncsLocked(send, staleFuncs, nil, releaseReplaced)
}

func (interp *Interpreter) ownedObjectsForValueLocked(v reflect.Value) []*ownedObject {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return nil
	}
	result := []*ownedObject{}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		for _, obj := range interp.ownedObjects {
			if obj.key.kind == reflect.Map && obj.key.ptr == v.Pointer() {
				result = append(result, obj)
			}
		}
	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		ptr := v.Pointer()
		for _, obj := range interp.ownedObjects {
			base, end, ok := ownedObjectInterval(obj)
			if ok && ptr >= base && ptr < end {
				result = append(result, obj)
			}
		}
	case reflect.UnsafePointer:
		if v.IsNil() {
			return nil
		}
		ptr := v.Pointer()
		for _, obj := range interp.ownedObjects {
			base, end, ok := ownedObjectInterval(obj)
			if ok && ptr >= base && ptr < end {
				result = append(result, obj)
			}
		}
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 || v.Type().Elem().Size() == 0 {
			return nil
		}
		base := v.Pointer()
		end := base + uintptr(v.Cap())*v.Type().Elem().Size()
		for _, obj := range interp.ownedObjects {
			ob, oe, ok := ownedObjectInterval(obj)
			if ok && base < oe && ob < end {
				result = append(result, obj)
			}
		}
	}
	return result
}

func (interp *Interpreter) ownedCellHostSharedLocked(v reflect.Value) bool {
	// Exact count maintained by markOwnedObjectHostSharedLocked and the
	// release paths; when it is zero no object can match below. This keeps
	// the per-write ownership scan O(1) for pure interpreted workloads.
	if interp.hostSharedEstimate == 0 {
		return false
	}
	v = unwrapOwnedValue(v)
	if !v.IsValid() || !v.CanAddr() {
		return false
	}
	base := v.Addr().Pointer()
	end := base + v.Type().Size()
	if end == base {
		end++
	}
	for _, obj := range interp.ownedObjects {
		ob, oe, ok := ownedObjectInterval(obj)
		if obj.hostShared && ok && base < oe && ob < end {
			return true
		}
	}
	return false
}

// markOwnedValuesHostShared records the explicit native boundary. The native
// call may mutate a direct reference concurrently after cancellation, so a
// detached root must keep that object shallow and must never traverse it.
// Immutable struct/array shells are safe to inspect before the call starts;
// mutable containers are deliberately not traversed.
func (interp *Interpreter) markOwnedValuesHostShared(values ...reflect.Value) {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.markOwnedValuesHostSharedLocked(values...)
}

func (interp *Interpreter) publishHostValues(values ...reflect.Value) {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	for _, value := range values {
		interp.publishHostValueLocked(value, false)
	}
}

func (interp *Interpreter) publishHostPanic(recovered any) interface{} {
	panicValue, token := splitInterpretedPanic(recovered)
	value := reflect.ValueOf(panicValue)
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.publishOwnedPanicToken(token)
	interp.finishOwnedPanicToken(token)
	interp.publishHostValueLocked(value, false)
	return panicValue
}

func (interp *Interpreter) markOwnedValuesHostSharedFromExec(values ...reflect.Value) {
	interp.withFuncSweepWriteFromExec(func() {
		interp.markOwnedValuesHostSharedLocked(values...)
	})
}

func (interp *Interpreter) registerNativeResultValuesFromExec(owner *frame, values ...reflect.Value) {
	interp.withFuncSweepWriteFromExec(func() {
		interp.funcMu.Lock()
		defer interp.funcMu.Unlock()
		var register func(reflect.Value)
		register = func(value reflect.Value) {
			value = unwrapOwnedValue(value)
			if !value.IsValid() {
				return
			}
			switch value.Kind() {
			case reflect.Map, reflect.Ptr, reflect.Slice:
				if value.IsNil() {
					return
				}
				objects := interp.ownedObjectsForValueLocked(value)
				if len(objects) == 0 {
					if obj, ok := describeOwnedObject(value, owner); ok {
						obj.hostShared = true
						interp.hostSharedEstimate++
						interp.ownedObjects[obj.key] = obj
						interp.armOwnedGCLocked()
						if owner != nil {
							if owner.ownedObjects == nil {
								owner.ownedObjects = map[*ownedObject]struct{}{}
							}
							owner.ownedObjects[obj] = struct{}{}
						}
					}
					return
				}
				for _, obj := range objects {
					interp.markOwnedObjectHostSharedLocked(obj)
				}
			case reflect.UnsafePointer:
				if value.IsNil() {
					return
				}
				// A native converter may return a raw view into a different
				// interpreter allocation. Preserve existing provenance, but do
				// not invent an allocation extent for an arbitrary native address.
				for _, obj := range interp.ownedObjectsForValueLocked(value) {
					interp.markOwnedObjectHostSharedLocked(obj)
				}
			case reflect.Chan:
				interp.publishOwnedChannelLocked(value)
			case reflect.Interface:
				if !value.IsNil() {
					register(value.Elem())
				}
			case reflect.Struct:
				for index := 0; index < value.NumField(); index++ {
					if value.Field(index).CanInterface() {
						register(value.Field(index))
					}
				}
			case reflect.Array:
				for index := 0; index < value.Len(); index++ {
					register(value.Index(index))
				}
			}
		}
		for _, value := range values {
			register(value)
		}
	})
}

func (interp *Interpreter) markOwnedWriteFromExec(destination reflect.Value, values ...reflect.Value) {
	hostShared := interp.ownedWriteNeedsHostShare(destination)
	channelPending := interp.ownedWriteTouchesChannelSend(destination)
	if !hostShared && !channelPending {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		if hostShared {
			interp.markOwnedValuesHostSharedLocked(values...)
		}
		if channelPending {
			interp.extendOwnedChannelSendsForWriteLocked(destination, values...)
		}
	})
}

func (interp *Interpreter) markOwnedCellWriteFromExec(destination reflect.Value, values ...reflect.Value) {
	interp.funcMu.RLock()
	shared := interp.ownedCellHostSharedLocked(destination)
	pending := interp.ownedCellTouchesChannelSendLocked(destination)
	interp.funcMu.RUnlock()
	if !shared && !pending {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		if shared {
			interp.markOwnedValuesHostSharedLocked(values...)
		}
		if pending {
			interp.extendOwnedChannelSendsForWriteLocked(destination, values...)
		}
	})
}

func (interp *Interpreter) ownedCellTouchesChannelSendLocked(destination reflect.Value) bool {
	// Channel-send references can only exist while a channel record or a live
	// panic token exists (both exact, funcMu-guarded registries), so skip the
	// registry walk entirely in the common send-free case.
	if len(interp.ownedChannels) == 0 && len(interp.panicTokens) == 0 {
		return false
	}
	destination = unwrapOwnedValue(destination)
	if !destination.IsValid() || !destination.CanAddr() {
		return false
	}
	base := destination.Addr().Pointer()
	end := base + destination.Type().Size()
	if end == base {
		end++
	}
	for _, obj := range interp.ownedObjects {
		ob, oe, ok := ownedObjectInterval(obj)
		if ok && base < oe && ob < end && (len(obj.channelSends) > 0 || len(obj.panicTokens) > 0) {
			return true
		}
	}
	return false
}

func (interp *Interpreter) ownedWriteTouchesChannelSend(destination reflect.Value) bool {
	interp.funcMu.RLock()
	defer interp.funcMu.RUnlock()
	// Sends and panic tokens only exist while their registries are non-empty
	// (both exact, funcMu-guarded), so skip the destination walk entirely in
	// the common send-free case. Registries are read under funcMu.
	if len(interp.ownedChannels) == 0 && len(interp.panicTokens) == 0 {
		return false
	}
	if interp.ownedCellTouchesChannelSendLocked(destination) {
		return true
	}
	for _, obj := range interp.ownedObjectsForValueLocked(destination) {
		if len(obj.channelSends) > 0 || len(obj.panicTokens) > 0 {
			return true
		}
	}
	return false
}

func (interp *Interpreter) extendOwnedChannelSendsForWriteLocked(destination reflect.Value, values ...reflect.Value) {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	seeds := map[*ownedObject]struct{}{}
	for _, obj := range interp.ownedObjectsForValueLocked(destination) {
		seeds[obj] = struct{}{}
	}
	destination = unwrapOwnedValue(destination)
	if destination.IsValid() && destination.CanAddr() {
		base := destination.Addr().Pointer()
		end := base + destination.Type().Size()
		if end == base {
			end++
		}
		for _, obj := range interp.ownedObjects {
			ob, oe, ok := ownedObjectInterval(obj)
			if ok && base < oe && ob < end {
				seeds[obj] = struct{}{}
			}
		}
	}
	rawSends := map[*ownedChannelSend]struct{}{}
	pendingSends := map[*ownedChannelSend]struct{}{}
	rawTokens := map[*ownedPanicToken]struct{}{}
	pendingTokens := map[*ownedPanicToken]struct{}{}
	for obj := range seeds {
		for send := range obj.channelSends {
			if send.state == ownedChannelSendTerminal {
				continue
			}
			if _, raw := send.objects[obj]; raw {
				rawSends[send] = struct{}{}
			}
			if _, pending := send.pendingObjects[obj]; pending {
				pendingSends[send] = struct{}{}
			}
		}
		for token := range obj.panicTokens {
			if token.finished {
				continue
			}
			if _, raw := token.objects[obj]; raw {
				rawTokens[token] = struct{}{}
			}
			if _, pending := token.pendingObjects[obj]; pending {
				pendingTokens[token] = struct{}{}
			}
		}
	}
	for send := range rawSends {
		interp.refreshOwnedChannelSendGraphLocked(send, false)
		objects := make(map[*ownedObject]struct{}, len(send.objects))
		for obj := range send.objects {
			objects[obj] = struct{}{}
		}
		funcs := make(map[reflect.Value]struct{}, len(send.funcs))
		for key := range send.funcs {
			funcs[key] = struct{}{}
		}
		for _, value := range values {
			interp.collectOwnedChannelGraphLocked(value, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		}
		interp.replaceOwnedChannelSendGraphLocked(send, false, objects, funcs, false)
	}
	for send := range pendingSends {
		interp.refreshOwnedChannelSendGraphLocked(send, true)
		objects := make(map[*ownedObject]struct{}, len(send.pendingObjects))
		for obj := range send.pendingObjects {
			objects[obj] = struct{}{}
		}
		funcs := make(map[reflect.Value]struct{}, len(send.pendingFuncs))
		for key := range send.pendingFuncs {
			funcs[key] = struct{}{}
		}
		for _, value := range values {
			interp.collectOwnedChannelGraphLocked(value, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		}
		// A function removed from the active pending snapshot may still have
		// been copied into another active-root cell. Re-root it visibly and let
		// the ordinary root sweep decide reachability.
		interp.replaceOwnedChannelSendGraphLocked(send, true, objects, funcs, false)
	}
	for token := range rawTokens {
		interp.refreshOwnedPanicTokenGraphLocked(token, false)
		for _, value := range values {
			interp.collectOwnedChannelGraphLocked(value, token.objects, token.funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		}
		interp.attachOwnedPanicTokenMembershipsLocked(token, token.objects, token.funcs, token.groups, token.frames)
	}
	for token := range pendingTokens {
		interp.refreshOwnedPanicTokenGraphLocked(token, true)
		for _, value := range values {
			interp.collectOwnedChannelGraphLocked(value, token.pendingObjects, token.pendingFuncs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		}
		interp.attachOwnedPanicTokenMembershipsLocked(token, token.pendingObjects, token.pendingFuncs, token.pendingGroups, token.pendingFrames)
	}
}

func (interp *Interpreter) refreshOwnedPanicTokenGraphLocked(token *ownedPanicToken, pending bool) {
	if token == nil || token.finished {
		return
	}
	source := token.value
	oldObjects, oldGroups := token.objects, token.groups
	if pending {
		if !token.pending.IsValid() {
			return
		}
		source = token.pending
		oldObjects, oldGroups = token.pendingObjects, token.pendingGroups
	}
	objects := map[*ownedObject]struct{}{}
	funcs := map[reflect.Value]struct{}{}
	groups := map[*funcMetaGroup]struct{}{}
	frames := map[*frame]struct{}{}
	interp.collectOwnedChannelGraphLocked(source, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
	if pending {
		token.pendingObjects, token.pendingFuncs, token.pendingGroups, token.pendingFrames = objects, funcs, groups, frames
	} else {
		token.objects, token.funcs, token.groups, token.frames = objects, funcs, groups, frames
	}
	interp.attachOwnedPanicTokenMembershipsLocked(token, objects, funcs, groups, frames)
	otherObjects := token.pendingObjects
	otherGroups := token.pendingGroups
	if pending {
		otherObjects, otherGroups = token.objects, token.groups
	}
	for obj := range oldObjects {
		if _, current := objects[obj]; current {
			continue
		}
		if _, other := otherObjects[obj]; other {
			continue
		}
		delete(obj.panicTokens, token)
	}
	for group := range oldGroups {
		if _, current := groups[group]; current {
			continue
		}
		if _, other := otherGroups[group]; other {
			continue
		}
		delete(group.panicTokens, token)
		interp.releaseOwnedPanicGroupLocked(group)
	}
}

func (interp *Interpreter) ownedWriteNeedsHostShare(destination reflect.Value) bool {
	interp.funcMu.RLock()
	// Exact count: with no host-shared objects registered, no destination can
	// need host-share marking. Keeps the per-write check O(1). Read under
	// funcMu so concurrent markings on other activations are synchronized.
	if interp.hostSharedEstimate == 0 {
		interp.funcMu.RUnlock()
		return false
	}
	shared := interp.ownedCellHostSharedLocked(destination)
	if !shared {
		for _, obj := range interp.ownedObjectsForValueLocked(destination) {
			if obj.hostShared {
				shared = true
				break
			}
		}
	}
	interp.funcMu.RUnlock()
	return shared
}

func (interp *Interpreter) markOwnedCopyWriteFromExec(destination, source reflect.Value) {
	hostShared := interp.ownedWriteNeedsHostShare(destination)
	channelPending := interp.ownedWriteTouchesChannelSend(destination)
	if !hostShared && !channelPending {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		count := source.Len()
		if destination.Len() < count {
			count = destination.Len()
		}
		if hostShared {
			interp.markOwnedSequenceElementsHostSharedLocked(source, count)
		}
		if channelPending {
			values := make([]reflect.Value, count)
			for index := 0; index < count; index++ {
				values[index] = source.Index(index)
			}
			interp.extendOwnedChannelSendsForWriteLocked(destination, values...)
		}
	})
}

func (interp *Interpreter) markOwnedCopyWriteLocked(destination, source reflect.Value) {
	hostShared := interp.ownedWriteNeedsHostShare(destination)
	channelPending := interp.ownedWriteTouchesChannelSend(destination)
	if !hostShared && !channelPending {
		return
	}
	count := source.Len()
	if destination.Len() < count {
		count = destination.Len()
	}
	if hostShared {
		interp.markOwnedSequenceElementsHostSharedLocked(source, count)
	}
	if channelPending {
		values := make([]reflect.Value, count)
		for index := 0; index < count; index++ {
			values[index] = source.Index(index)
		}
		interp.extendOwnedChannelSendsForWriteLocked(destination, values...)
	}
}

func (interp *Interpreter) markOwnedAppendSliceWriteFromExec(destination, source reflect.Value) {
	hostShared := interp.ownedWriteNeedsHostShare(destination)
	channelPending := interp.ownedWriteTouchesChannelSend(destination)
	if !hostShared && !channelPending {
		return
	}
	interp.withFuncSweepWriteFromExec(func() {
		if hostShared {
			interp.markOwnedSequenceElementsHostSharedLocked(source, source.Len())
		}
		if channelPending {
			values := make([]reflect.Value, source.Len())
			for index := range values {
				values[index] = source.Index(index)
			}
			interp.extendOwnedChannelSendsForWriteLocked(destination, values...)
		}
	})
}

func (interp *Interpreter) markOwnedSequenceElementsHostSharedLocked(source reflect.Value, count int) {
	source = unwrapOwnedValue(source)
	if !source.IsValid() || source.Kind() == reflect.String {
		return
	}
	interp.funcMu.RLock()
	if interp.ownedCellHostSharedLocked(source) {
		interp.funcMu.RUnlock()
		return
	}
	objects := interp.ownedObjectsForValueLocked(source)
	for _, obj := range objects {
		if obj.hostShared {
			interp.funcMu.RUnlock()
			return
		}
	}
	owned := len(objects) > 0 || source.Kind() == reflect.Array
	interp.funcMu.RUnlock()
	if !owned {
		return
	}
	if source.Len() < count {
		count = source.Len()
	}
	values := make([]reflect.Value, count)
	for index := 0; index < count; index++ {
		values[index] = source.Index(index)
	}
	interp.markOwnedValuesHostSharedLocked(values...)
}

func (interp *Interpreter) markOwnedValuesHostSharedLocked(values ...reflect.Value) {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	seen := map[*ownedObject]struct{}{}
	var mark func(reflect.Value)
	mark = func(v reflect.Value) {
		v = unwrapOwnedValue(v)
		if !v.IsValid() {
			return
		}
		if (v.Kind() == reflect.Struct || v.Kind() == reflect.Array) && interp.ownedCellHostSharedLocked(v) {
			return
		}
		switch v.Kind() {
		case reflect.Chan:
			interp.publishOwnedChannelLocked(v)
		case reflect.Map, reflect.Ptr, reflect.Slice:
			objects := interp.ownedObjectsForValueLocked(v)
			if len(objects) == 0 {
				return
			}
			for _, obj := range objects {
				if obj.hostShared {
					interp.markOwnedObjectHostSharedLocked(obj)
					return
				}
				if _, ok := seen[obj]; ok {
					return
				}
				seen[obj] = struct{}{}
			}
			children := []reflect.Value{}
			switch v.Kind() {
			case reflect.Map:
				iter := v.MapRange()
				for iter.Next() {
					children = append(children, iter.Key(), iter.Value())
				}
			case reflect.Ptr:
				children = append(children, v.Elem())
			case reflect.Slice:
				full := v.Slice(0, v.Cap())
				for index := 0; index < full.Len(); index++ {
					children = append(children, full.Index(index))
				}
			}
			for _, child := range children {
				mark(child)
			}
			for _, obj := range objects {
				interp.markOwnedObjectHostSharedLocked(obj)
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Field(i).CanInterface() {
					mark(v.Field(i))
				}
			}
		case reflect.Array:
			for i := 0; i < v.Len(); i++ {
				mark(v.Index(i))
			}
		case reflect.UnsafePointer:
			for _, obj := range interp.ownedObjectsForValueLocked(v) {
				interp.markOwnedObjectHostSharedLocked(obj)
			}
		}
	}
	for _, value := range values {
		mark(value)
	}
}

// publishHostValueLocked records values whose identity becomes observable to
// the embedding Go program. The caller holds funcSweepMu exclusively so root
// cells and ownership records cannot change while the publication barrier is
// installed.
func (interp *Interpreter) publishHostValueLocked(value reflect.Value, exposeCell bool) {
	if !value.IsValid() {
		return
	}
	interp.markOwnedValuesHostSharedLocked(value)
	if exposeCell && value.CanAddr() {
		address := value.Addr()
		interp.registerOwnedAddress(address, interp.frame)
		interp.markOwnedValuesHostSharedLocked(address)
	}
	interp.preserveReturnedInterpretedFuncsLocked(value)
}

func (interp *Interpreter) ownedValueContainsObjectLocked(v reflect.Value, target *ownedObject, seen map[*ownedObject]struct{}) bool {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return false
	}
	if target != nil && target.key.kind == reflect.Ptr && valueContainsAddress(v, target.key.ptr) {
		return true
	}
	if (v.Kind() == reflect.Struct || v.Kind() == reflect.Array) && interp.ownedCellHostSharedLocked(v) {
		return false
	}
	switch v.Kind() {
	case reflect.Map, reflect.Ptr, reflect.Slice:
		objects := interp.ownedObjectsForValueLocked(v)
		for _, obj := range objects {
			if obj == target {
				return true
			}
		}
		if len(objects) == 0 {
			return false
		}
		for _, obj := range objects {
			if obj.hostShared {
				return false
			}
			if _, ok := seen[obj]; ok {
				return false
			}
			seen[obj] = struct{}{}
		}
		switch v.Kind() {
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				if interp.ownedValueContainsObjectLocked(iter.Key(), target, seen) || interp.ownedValueContainsObjectLocked(iter.Value(), target, seen) {
					return true
				}
			}
		case reflect.Ptr:
			return interp.ownedValueContainsObjectLocked(v.Elem(), target, seen)
		case reflect.Slice:
			full := v.Slice(0, v.Cap())
			for index := 0; index < full.Len(); index++ {
				if interp.ownedValueContainsObjectLocked(full.Index(index), target, seen) {
					return true
				}
			}
		}
	case reflect.UnsafePointer:
		for _, obj := range interp.ownedObjectsForValueLocked(v) {
			if obj == target {
				return true
			}
		}
	case reflect.Struct:
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() && interp.ownedValueContainsObjectLocked(v.Field(index), target, seen) {
				return true
			}
		}
	case reflect.Array:
		for index := 0; index < v.Len(); index++ {
			if interp.ownedValueContainsObjectLocked(v.Index(index), target, seen) {
				return true
			}
		}
	}
	return false
}

func (interp *Interpreter) ownedObjectsInValueLocked(v reflect.Value, result map[*ownedObject]struct{}, seen map[*ownedObject]struct{}) {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Map, reflect.Ptr, reflect.Slice:
		objects := interp.ownedObjectsForValueLocked(v)
		if len(objects) == 0 {
			return
		}
		for _, obj := range objects {
			result[obj] = struct{}{}
			if obj.hostShared {
				return
			}
			if _, ok := seen[obj]; ok {
				return
			}
			seen[obj] = struct{}{}
		}
		switch v.Kind() {
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				interp.ownedObjectsInValueLocked(iter.Key(), result, seen)
				interp.ownedObjectsInValueLocked(iter.Value(), result, seen)
			}
		case reflect.Ptr:
			interp.ownedObjectsInValueLocked(v.Elem(), result, seen)
		case reflect.Slice:
			full := v.Slice(0, v.Cap())
			for index := 0; index < full.Len(); index++ {
				interp.ownedObjectsInValueLocked(full.Index(index), result, seen)
			}
		}
	case reflect.UnsafePointer:
		for _, obj := range interp.ownedObjectsForValueLocked(v) {
			result[obj] = struct{}{}
		}
	case reflect.Struct:
		if interp.ownedCellHostSharedLocked(v) {
			return
		}
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() {
				interp.ownedObjectsInValueLocked(v.Field(index), result, seen)
			}
		}
	case reflect.Array:
		if interp.ownedCellHostSharedLocked(v) {
			return
		}
		for index := 0; index < v.Len(); index++ {
			interp.ownedObjectsInValueLocked(v.Index(index), result, seen)
		}
	}
}

func (interp *Interpreter) beginOwnedPanicLocked(values ...reflect.Value) *ownedPanicToken {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	objects := map[*ownedObject]struct{}{}
	funcs := map[reflect.Value]struct{}{}
	for _, value := range values {
		interp.collectOwnedChannelGraphLocked(value, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
	}
	token := &ownedPanicToken{
		objects:        objects,
		funcs:          funcs,
		groups:         map[*funcMetaGroup]struct{}{},
		frames:         map[*frame]struct{}{},
		pendingObjects: map[*ownedObject]struct{}{},
		pendingFuncs:   map[reflect.Value]struct{}{},
		pendingGroups:  map[*funcMetaGroup]struct{}{},
		pendingFrames:  map[*frame]struct{}{},
	}
	if len(values) != 0 {
		token.value = values[0]
	}
	interp.panicTokens[token] = struct{}{}
	interp.attachOwnedPanicTokenMembershipsLocked(token, objects, funcs, token.groups, token.frames)
	return token
}

func (interp *Interpreter) attachOwnedPanicTokenMembershipsLocked(token *ownedPanicToken, objects map[*ownedObject]struct{}, funcs map[reflect.Value]struct{}, groups map[*funcMetaGroup]struct{}, frames map[*frame]struct{}) {
	for obj := range objects {
		if obj.panicTokens == nil {
			obj.panicTokens = map[*ownedPanicToken]struct{}{}
		}
		obj.panicTokens[token] = struct{}{}
	}
	for value := range funcs {
		key, ok := funcvalKeyOf(value)
		if !ok {
			continue
		}
		meta, ok := interp.funcMeta[key]
		if !ok || meta.group == nil {
			continue
		}
		group := meta.group
		groups[group] = struct{}{}
		if group.panicTokens == nil {
			group.panicTokens = map[*ownedPanicToken]struct{}{}
		}
		group.panicTokens[token] = struct{}{}
		if meta.frame != nil && meta.frame != meta.frame.root {
			frames[meta.frame] = struct{}{}
			if meta.frame.funcEscape < funcMetaPanic {
				meta.frame.funcEscape = funcMetaPanic
			}
		} else if meta.retention < funcMetaPanic {
			meta.retention = funcMetaPanic
			interp.funcMeta[key] = meta
		}
	}
}

func (interp *Interpreter) releaseOwnedPanicGroupLocked(group *funcMetaGroup) {
	if group == nil || len(group.panicTokens) != 0 {
		return
	}
	retention := funcMetaVisible
	if group.pending > 0 {
		retention = funcMetaChannel
	}
	for key, meta := range interp.funcMeta {
		if meta.group != group {
			continue
		}
		if meta.retention == funcMetaPanic {
			meta.retention = retention
			interp.funcMeta[key] = meta
		}
		if meta.frame != nil && meta.frame != meta.frame.root && meta.frame.funcEscape == funcMetaPanic {
			meta.frame.funcEscape = retention
		}
	}
}

// finishOwnedPanicTokenLocked requires funcMu and consumes a token at most
// once, regardless of how many unwind/publication paths observe the panic.
func (interp *Interpreter) finishOwnedPanicTokenLocked(token *ownedPanicToken) {
	if token == nil || token.finished {
		return
	}
	for obj := range token.objects {
		delete(obj.panicTokens, token)
	}
	for obj := range token.pendingObjects {
		delete(obj.panicTokens, token)
	}
	groups := map[*funcMetaGroup]struct{}{}
	for group := range token.groups {
		groups[group] = struct{}{}
	}
	for group := range token.pendingGroups {
		groups[group] = struct{}{}
	}
	delete(interp.panicTokens, token)
	for group := range groups {
		delete(group.panicTokens, token)
		interp.releaseOwnedPanicGroupLocked(group)
	}
	frames := map[*frame]struct{}{}
	for frame := range token.frames {
		frames[frame] = struct{}{}
	}
	for frame := range token.pendingFrames {
		frames[frame] = struct{}{}
	}
	for frame := range frames {
		if frame == nil || frame.funcEscape != funcMetaPanic {
			continue
		}
		active := false
		for other := range interp.panicTokens {
			if _, ok := other.frames[frame]; ok {
				active = true
				break
			}
			if _, ok := other.pendingFrames[frame]; ok {
				active = true
				break
			}
		}
		if !active {
			frame.funcEscape = funcMetaVisible
		}
	}
	token.finished = true
}

func (interp *Interpreter) finishOwnedPanicToken(token *ownedPanicToken) {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	interp.finishOwnedPanicTokenLocked(token)
}

func (interp *Interpreter) publishOwnedPanicToken(token *ownedPanicToken) {
	if token == nil {
		return
	}
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	for group := range token.groups {
		for key, meta := range interp.funcMeta {
			if meta.group != group {
				continue
			}
			meta.retention = funcMetaOpaque
			if meta.frame != nil && meta.frame != meta.frame.root {
				meta.frame.funcEscape = funcMetaOpaque
			}
			interp.funcMeta[key] = meta
		}
	}
}

func (interp *Interpreter) transferOwnedObjectLocked(obj *ownedObject, owner *frame) {
	if obj == nil || owner == nil || obj.owner == owner {
		return
	}
	if obj.owner != nil {
		delete(obj.owner.ownedObjects, obj)
	}
	obj.owner = owner
	if owner.ownedObjects == nil {
		owner.ownedObjects = map[*ownedObject]struct{}{}
	}
	owner.ownedObjects[obj] = struct{}{}
}

func (interp *Interpreter) adoptOwnedValuesLocked(owner *frame, retention funcMetaRetention, values ...reflect.Value) []reflect.Value {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	objects := map[*ownedObject]struct{}{}
	for _, value := range values {
		interp.ownedObjectsInValueLocked(value, objects, map[*ownedObject]struct{}{})
	}
	adopted := make([]reflect.Value, len(values))
	for index, value := range values {
		adopted[index] = interp.pendingOwnedValueLocked(value, owner.root)
	}
	for obj := range objects {
		if retention == funcMetaChannel {
			if obj.channelRefs > 0 {
				obj.channelRefs--
			}
		}
		interp.adoptOwnedObjectLocked(owner, obj)
	}
	return adopted
}

func (interp *Interpreter) adoptOwnedPanicValuesLocked(owner *frame, token *ownedPanicToken, values ...reflect.Value) []reflect.Value {
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	adopted := append([]reflect.Value(nil), values...)
	if token == nil || token.finished {
		return adopted
	}
	objects := token.objects
	groups := token.groups
	if token.pendingRoot == owner.root && token.pending.IsValid() && len(adopted) == 1 {
		adopted[0] = token.pending
		objects = token.pendingObjects
		groups = token.pendingGroups
	}
	interp.finishOwnedPanicTokenLocked(token)
	for obj := range objects {
		interp.adoptOwnedObjectLocked(owner, obj)
	}
	for group := range groups {
		interp.adoptOwnedPanicGroupLocked(owner, group)
	}
	return adopted
}

func (interp *Interpreter) adoptOwnedPanicGroupLocked(owner *frame, group *funcMetaGroup) {
	if owner == nil || group == nil {
		return
	}
	group.root = owner.root
	active := len(group.panicTokens) != 0
	for key, meta := range interp.funcMeta {
		if meta.group != group {
			continue
		}
		old := meta.frame
		meta.frame = owner
		if active {
			meta.retention = funcMetaPanic
		} else {
			meta.retention = funcMetaVisible
		}
		interp.funcMeta[key] = meta
		owner.funcMeta = append(owner.funcMeta, key)
		if old != nil && old != old.root && old != owner && old.funcEscape == funcMetaPanic && len(group.panicTokens) == 0 {
			old.funcEscape = funcMetaVisible
		}
	}
}

func (interp *Interpreter) adoptOwnedObjectLocked(owner *frame, obj *ownedObject) {
	if obj == nil {
		return
	}
	if obj.pendingRoot == owner.root && obj.pending.IsValid() {
		pending, ok := describeOwnedObject(obj.pending, owner)
		if ok {
			if existing := interp.ownedObjectLocked(obj.pending); existing != nil {
				interp.transferOwnedObjectLocked(existing, owner)
			} else {
				interp.ownedObjects[pending.key] = pending
				interp.armOwnedGCLocked()
				if owner.ownedObjects == nil {
					owner.ownedObjects = map[*ownedObject]struct{}{}
				}
				owner.ownedObjects[pending] = struct{}{}
			}
		}
	} else {
		interp.transferOwnedObjectLocked(obj, owner)
	}
	if obj.channelRefs == 0 && !ownedObjectHasPanicToken(obj) && obj.pending.IsValid() && obj.owner != nil && obj.owner.root != owner.root {
		interp.unregisterOwnedObjectLocked(obj)
		delete(obj.owner.ownedObjects, obj)
	}
}

func (interp *Interpreter) pendingOwnedValueLocked(v reflect.Value, root *frame) reflect.Value {
	if !v.IsValid() {
		return v
	}
	if v.Type() == valueInterfaceType && v.CanInterface() {
		wrapped := v.Interface().(valueInterface)
		wrapped.value = interp.pendingOwnedValueLocked(wrapped.value, root)
		return reflect.ValueOf(wrapped)
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(interp.pendingOwnedValueLocked(v.Elem(), root))
		return out
	case reflect.Map, reflect.Ptr, reflect.Slice:
		obj := interp.ownedObjectLocked(v)
		if obj == nil || obj.pendingRoot != root || !obj.pending.IsValid() {
			return v
		}
		pending := obj.pending
		if v.Kind() == reflect.Slice {
			offset := int((v.Pointer() - obj.sliceBase) / v.Type().Elem().Size())
			pending = pending.Slice3(offset, offset+v.Len(), offset+v.Cap())
		}
		if pending.Type() == v.Type() {
			return pending
		}
		if pending.Type().ConvertibleTo(v.Type()) {
			return pending.Convert(v.Type())
		}
		return v
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() {
				out.Field(index).Set(interp.pendingOwnedValueLocked(v.Field(index), root))
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for index := 0; index < v.Len(); index++ {
			out.Index(index).Set(interp.pendingOwnedValueLocked(v.Index(index), root))
		}
		return out
	}
	return v
}

func (interp *Interpreter) rollbackOwnedPublishedPanic(recovered interface{}) {
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	_, token := splitInterpretedPanic(recovered)
	interp.finishOwnedPanicToken(token)
}

func (interp *Interpreter) ownedValueContainsChannelLocked(v reflect.Value, target uintptr, seen map[*ownedObject]struct{}) bool {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return false
	}
	if v.Kind() == reflect.Chan {
		return !v.IsNil() && v.Pointer() == target
	}
	if (v.Kind() == reflect.Struct || v.Kind() == reflect.Array) && interp.ownedCellHostSharedLocked(v) {
		return false
	}
	switch v.Kind() {
	case reflect.Map, reflect.Ptr, reflect.Slice:
		objects := interp.ownedObjectsForValueLocked(v)
		if len(objects) == 0 {
			return false
		}
		for _, obj := range objects {
			if obj.hostShared {
				return false
			}
			if _, ok := seen[obj]; ok {
				return false
			}
			seen[obj] = struct{}{}
		}
		switch v.Kind() {
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				if interp.ownedValueContainsChannelLocked(iter.Key(), target, seen) || interp.ownedValueContainsChannelLocked(iter.Value(), target, seen) {
					return true
				}
			}
		case reflect.Ptr:
			return interp.ownedValueContainsChannelLocked(v.Elem(), target, seen)
		case reflect.Slice:
			full := v.Slice(0, v.Cap())
			for index := 0; index < full.Len(); index++ {
				if interp.ownedValueContainsChannelLocked(full.Index(index), target, seen) {
					return true
				}
			}
		}
	case reflect.Struct:
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() && interp.ownedValueContainsChannelLocked(v.Field(index), target, seen) {
				return true
			}
		}
	case reflect.Array:
		for index := 0; index < v.Len(); index++ {
			if interp.ownedValueContainsChannelLocked(v.Index(index), target, seen) {
				return true
			}
		}
	}
	return false
}

func (interp *Interpreter) releaseUnreachableChannelSends(f *frame, funcNode *node) {
	if f == nil || f == f.root {
		return
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.funcMu.RLock()
	channels := make([]*ownedChannel, 0, len(f.ownedChannels))
	for channel := range f.ownedChannels {
		if channel.owner == f {
			channels = append(channels, channel)
		}
	}
	interp.funcMu.RUnlock()
	if len(channels) == 0 {
		return
	}

	frames := []*frame{}
	for ancestor := f.anc; ancestor != nil; ancestor = ancestor.anc {
		if ancestor == f.root || ancestor.funcState == funcFrameActive || ancestor.funcState == funcFrameReleasing {
			frames = append(frames, ancestor)
		}
	}
	if len(frames) == 0 || frames[len(frames)-1] != f.root {
		frames = append(frames, f.root)
	}
	values := map[*frame][]reflect.Value{}
	for _, frame := range frames {
		values[frame] = interp.snapshotOwnedReachabilityValues(frame)
	}
	returnValues := snapshotFrameValues(f)
	if funcNode != nil && funcNode.typ != nil && len(returnValues) > len(funcNode.typ.ret) {
		returnValues = returnValues[:len(funcNode.typ.ret)]
	}

	for _, channel := range channels {
		var owner *frame
		interp.funcMu.RLock()
		if channel.hostVisible {
			owner = f.root
		} else {
			ptr := channel.hold.Pointer()
			if len(returnValues) > 0 && interp.ownedValuesContainChannelThroughFuncsLocked(returnValues, ptr) {
				owner = funcRetentionOwnerLocked(f.root, f.anc)
			}
			for _, frame := range frames {
				if owner != nil {
					break
				}
				for _, value := range values[frame] {
					if interp.ownedValuesContainChannelThroughFuncsLocked([]reflect.Value{value}, ptr) {
						owner = frame
						break
					}
				}
				if owner != nil {
					break
				}
			}
		}
		sends := append([]*ownedChannelSend(nil), channel.sends...)
		interp.funcMu.RUnlock()
		if owner == nil {
			interp.funcMu.Lock()
			for _, send := range sends {
				if send.state != ownedChannelSendTerminal {
					interp.retireOwnedChannelSendLocked(send)
					interp.releaseOwnedChannelSendFuncsLocked(send, send.funcs, nil, false)
					interp.releaseOwnedChannelSendFuncsLocked(send, send.pendingFuncs, nil, false)
				}
			}
			delete(interp.ownedChannels, channel.hold.Pointer())
			delete(f.ownedChannels, channel)
			channel.sends = nil
			interp.funcMu.Unlock()
			continue
		}
		interp.funcMu.Lock()
		delete(f.ownedChannels, channel)
		channel.owner = owner
		if owner.ownedChannels == nil {
			owner.ownedChannels = map[*ownedChannel]struct{}{}
		}
		owner.ownedChannels[channel] = struct{}{}
		interp.funcMu.Unlock()
	}
}

func (interp *Interpreter) ownedValuesContainChannelThroughFuncsLocked(values []reflect.Value, target uintptr) bool {
	for _, value := range values {
		if interp.ownedValueContainsChannelLocked(value, target, map[*ownedObject]struct{}{}) {
			return true
		}
	}
	seenGroups := map[*funcMetaGroup]struct{}{}
	for {
		changed := false
		for key, meta := range interp.funcMeta {
			if meta.group == nil {
				continue
			}
			if _, seen := seenGroups[meta.group]; seen {
				continue
			}
			visitor := funcValueVisitor{targets: map[uintptr]struct{}{key: {}}}
			matched := false
			for _, value := range values {
				if visitor.contains(value) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			seenGroups[meta.group] = struct{}{}
			changed = true
			for _, capture := range meta.group.captures {
				if capture.frame == nil || capture.index < 0 || capture.index >= len(capture.frame.data) {
					continue
				}
				captured := capture.frame.data[capture.index]
				if interp.ownedValueContainsChannelLocked(captured, target, map[*ownedObject]struct{}{}) {
					return true
				}
				values = append(values, captured)
			}
		}
		if !changed {
			return false
		}
	}
}

func (interp *Interpreter) sweepRootOwnedChannels(root *frame) {
	if root == nil {
		return
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	values := interp.snapshotOwnedReachabilityValues(root)
	interp.funcMu.RLock()
	unreachable := map[*ownedChannel][]*ownedChannelSend{}
	for _, channel := range interp.ownedChannels {
		if channel.owner != root || channel.hostVisible || interp.ownedValuesContainChannelThroughFuncsLocked(values, channel.hold.Pointer()) {
			continue
		}
		unreachable[channel] = append([]*ownedChannelSend(nil), channel.sends...)
	}
	interp.funcMu.RUnlock()
	for channel, sends := range unreachable {
		interp.funcMu.Lock()
		for _, send := range sends {
			if send.state != ownedChannelSendTerminal {
				interp.retireOwnedChannelSendLocked(send)
				interp.releaseOwnedChannelSendFuncsLocked(send, send.funcs, nil, false)
				interp.releaseOwnedChannelSendFuncsLocked(send, send.pendingFuncs, nil, false)
			}
		}
		delete(interp.ownedChannels, channel.hold.Pointer())
		delete(root.ownedChannels, channel)
		channel.sends = nil
		interp.funcMu.Unlock()
	}
}

func snapshotFrameValues(f *frame) []reflect.Value {
	if f == nil {
		return nil
	}
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	return append([]reflect.Value(nil), f.data...)
}

func sameFrameCell(a, b reflect.Value) bool {
	return a.IsValid() && b.IsValid() && a.CanAddr() && b.CanAddr() && a.Addr().Type() == b.Addr().Type() && a.Addr().Pointer() == b.Addr().Pointer()
}

func (interp *Interpreter) snapshotOwnedReachabilityValues(f *frame) []reflect.Value {
	if f == nil || f != f.root {
		return snapshotFrameValues(f)
	}
	indexes := interp.snapshotGlobalVarIndexes()
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	values := make([]reflect.Value, 0, len(indexes))
	for index := range indexes {
		if index < len(f.data) {
			values = append(values, f.data[index])
		}
	}
	return values
}

func (interp *Interpreter) releaseOwnedObjects(f *frame, funcNode *node) {
	if f == nil || f == f.root {
		return
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.funcMu.Lock()
	targets := make([]*ownedObject, 0, len(f.ownedObjects))
	for obj := range f.ownedObjects {
		if obj.owner == f {
			targets = append(targets, obj)
		}
	}
	// The registry key is in the object, so removal is O(1): scanning the
	// whole registry per target made frame exit quadratic in the number of
	// allocations a long-lived frame had produced.
	deleteRecord := func(target *ownedObject) {
		interp.unregisterOwnedObjectLocked(target)
		delete(f.ownedObjects, target)
	}
	if len(targets) == 0 {
		interp.funcMu.Unlock()
		return
	}

	// Reachability is computed with one collector traversal per ownership
	// source (ancestor frames, return cells, metadata captures) instead of
	// one containment walk per owned object: the collector cost is
	// independent of len(targets), which keeps frame exit O(frame+registry)
	// rather than quadratic in the number of owned allocations.
	type ownershipSource struct {
		owner     *frame
		reachable map[*ownedObject]struct{}
		intervals *visitedIntervalCollector
	}
	sources := make([]ownershipSource, 0, 8)
	collect := func(owner *frame, values []reflect.Value) {
		if len(values) == 0 {
			return
		}
		src := ownershipSource{owner: owner, reachable: map[*ownedObject]struct{}{}, intervals: &visitedIntervalCollector{}}
		seen := map[*ownedObject]struct{}{}
		for _, value := range values {
			interp.collectReachableObjectsLocked(value, src.reachable, seen, src.intervals, nil)
		}
		src.intervals.sort()
		sources = append(sources, src)
	}

	frames := []*frame{}
	seenFrames := map[*frame]struct{}{}
	for ancestor := f.anc; ancestor != nil; ancestor = ancestor.anc {
		if _, seen := seenFrames[ancestor]; seen {
			break
		}
		seenFrames[ancestor] = struct{}{}
		if ancestor == f.root || ancestor.funcState == funcFrameActive || ancestor.funcState == funcFrameReleasing {
			frames = append(frames, ancestor)
		}
	}
	if _, seen := seenFrames[f.root]; !seen && f.root != nil {
		frames = append(frames, f.root)
	}
	frameCells := make(map[*frame][]reflect.Value, len(frames))
	for _, frame := range frames {
		collect(frame, interp.snapshotOwnedReachabilityValues(frame))
		frameCells[frame] = snapshotFrameValues(frame)
	}
	returnValues := snapshotFrameValues(f)
	if funcNode != nil && funcNode.typ != nil && len(returnValues) > len(funcNode.typ.ret) {
		returnValues = returnValues[:len(funcNode.typ.ret)]
	}
	for _, returned := range returnValues {
		var returnOwner *frame
		for _, frame := range frames {
			for _, cell := range frameCells[frame] {
				if sameFrameCell(returned, cell) {
					returnOwner = frame
					break
				}
			}
			if returnOwner != nil {
				break
			}
		}
		if returnOwner != nil {
			collect(returnOwner, []reflect.Value{returned})
		}
	}
	for _, meta := range interp.funcMeta {
		if meta.group == nil || meta.frame == nil {
			continue
		}
		captureValues := []reflect.Value{}
		for _, capture := range meta.group.captures {
			if capture.frame == nil || capture.frame != f && capture.frame.cloneOf != f {
				continue
			}
			if capture.index >= 0 && capture.index < len(capture.frame.data) {
				captureValues = append(captureValues, capture.frame.data[capture.index])
			}
		}
		collect(meta.frame, captureValues)
	}

	for _, target := range targets {
		if target.channelRefs > 0 || ownedObjectHasPanicToken(target) {
			continue
		}
		var owner *frame
		for _, src := range sources {
			if _, ok := src.reachable[target]; ok {
				owner = src.owner
				break
			}
			if target.key.kind == reflect.Ptr && src.intervals.contains(target.key.ptr) {
				owner = src.owner
				break
			}
		}
		if owner == nil {
			deleteRecord(target)
			continue
		}
		if owner != owner.root && owner.funcState != funcFrameActive && owner.funcState != funcFrameReleasing {
			deleteRecord(target)
			continue
		}
		delete(f.ownedObjects, target)
		target.owner = owner
		if owner.ownedObjects == nil {
			owner.ownedObjects = map[*ownedObject]struct{}{}
		}
		owner.ownedObjects[target] = struct{}{}
	}
	interp.funcMu.Unlock()
}

// sweepRootOwnedObjects drops aggregate identity metadata once no durable
// interpreter root needs it. Host-held values remain alive through the host's
// own references and are intentionally treated as native/shallow if they later
// re-enter the interpreter.
func (interp *Interpreter) sweepRootOwnedObjects(root *frame) {
	if root == nil {
		return
	}
	interp.funcSweepMu.Lock()
	defer interp.funcSweepMu.Unlock()
	interp.sweepRootOwnedObjectsLocked(root)
}

// sweepRootOwnedObjectsLocked is the body of sweepRootOwnedObjects for callers
// which already hold the exclusive funcSweepMu fence, such as
// PurgeRetainedFuncs. The caller must not hold funcMu: the body acquires it.
func (interp *Interpreter) sweepRootOwnedObjectsLocked(root *frame) {
	if root == nil {
		return
	}
	values := interp.snapshotOwnedReachabilityValues(root)
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	interp.sweepOwnedObjectsValuesLocked(root, values, nil)
}

// sweepOwnedObjectsValuesLocked evicts ownership metadata for objects
// unreachable from the given root values plus the capture cells of the
// relevant funcMeta groups. When root is non-nil only objects owned below root
// are candidates and only root's visible/opaque metadata captures seed the
// collector (the Eval-end sweep); when root is nil every owned object is a
// candidate and every group's capture cells seed it (the incremental ownedGC
// sweep). The eviction predicate itself is identical in both modes:
// channelRefs == 0, no panic token, not in the reachable set, and interior
// pointers of visited storage are kept. When channels is non-nil, registered
// owned channels found in the traversed graph are recorded there, so the
// incremental sweep reuses this traversal for its channel eviction. The
// caller holds funcMu; the exclusive funcSweepMu fence must be held so frame
// cells and registries cannot change concurrently.
func (interp *Interpreter) sweepOwnedObjectsValuesLocked(root *frame, values []reflect.Value, channels map[uintptr]struct{}) {
	metas := make([]interpretedFuncMeta, 0, len(interp.funcMeta))
	for _, meta := range interp.funcMeta {
		if root != nil && (meta.frame == nil || meta.frame.root != root || meta.retention != funcMetaVisible && meta.retention != funcMetaOpaque) {
			continue
		}
		metas = append(metas, meta)
	}
	// Compute reachability in one traversal instead of one containment walk
	// per owned object: collect the objects resolvable from the root values,
	// plus the address intervals of every traversed node so Ptr-kind
	// targets reachable only as interior pointers of raw storage are kept,
	// matching ownedValueContainsObjectLocked semantics.
	reachableSet := map[*ownedObject]struct{}{}
	visited := &visitedIntervalCollector{}
	seen := map[*ownedObject]struct{}{}
	for _, value := range values {
		interp.collectReachableObjectsLocked(value, reachableSet, seen, visited, channels)
	}
	for _, meta := range metas {
		if meta.group == nil {
			continue
		}
		for _, capture := range meta.group.captures {
			if capture.frame == nil || capture.index < 0 || capture.index >= len(capture.frame.data) {
				continue
			}
			interp.collectReachableObjectsLocked(capture.frame.data[capture.index], reachableSet, seen, visited, channels)
		}
	}
	visited.sort()
	for _, target := range interp.ownedObjects {
		if target.owner == nil || target.channelRefs > 0 || ownedObjectHasPanicToken(target) {
			continue
		}
		if root != nil && target.owner.root != root {
			continue
		}
		if _, ok := reachableSet[target]; ok {
			continue
		}
		if target.key.kind == reflect.Ptr && visited.contains(target.key.ptr) {
			continue
		}
		interp.unregisterOwnedObjectLocked(target)
		delete(target.owner.ownedObjects, target)
	}
}

// armOwnedGCLocked bounds the ownership registries and the directFuncs
// activation cache: once their combined size crosses ownedGCRegistryCap, one
// incremental sweep is requested per ownedGCAmortizeRegistrations inserts.
// The caller holds funcMu (every registry insert site calls this from its
// existing critical section). Arming must never take or upgrade the
// funcSweepMu fence: insert sites run under funcMu inside fence-holding
// callers (e.g. publishHostValueLocked), the fence is not reentrant, and
// upgrading while holding funcMu could fatal against write-held sections. The
// request is consumed later, where the goroutine holds no locks.
func (interp *Interpreter) armOwnedGCLocked() {
	interp.ownedRegistrations++
	if len(interp.ownedObjects)+len(interp.ownedChannels)+len(interp.directFuncs) < ownedGCRegistryCap {
		return
	}
	if interp.ownedRegistrations < ownedGCAmortizeRegistrations {
		return
	}
	interp.ownedRegistrations = 0
	interp.ownedGCPending.CompareAndSwap(false, true)
}

// maybeRunOwnedGCSweep consumes a pending incremental sweep request. The
// caller must hold no locks: the sweep acquires the funcSweepMu fence
// exclusively via TryLock and then funcMu for the whole body, which is the
// universal fence-before-funcMu order (the reverse is forbidden: registry
// insert sites hold funcMu inside fence-holding callers). A TryLock loss
// leaves the request pending; the next execution step retries. inFlight and
// the fence are always released, including on panic.
func (interp *Interpreter) maybeRunOwnedGCSweep() {
	if !interp.ownedGCPending.Load() {
		return
	}
	if !interp.ownedGCInFlight.CompareAndSwap(false, true) {
		return
	}
	defer interp.ownedGCInFlight.Store(false)
	if !interp.funcSweepMu.TryLock() {
		return
	}
	func() {
		defer interp.funcSweepMu.Unlock()
		// The fence is held exclusively here, so the check is exact: a drain
		// cannot start underneath the sweep (frameDrains is incremented under
		// the fence in runCfg). A draining frame's remaining deferred call
		// values are invisible to the root set, so the sweep must wait; the
		// request stays pending and a later step consumes it.
		if interp.frameDrains.Load() != 0 {
			return
		}
		interp.ownedGCPending.Store(false)
		interp.ownedGCSweepLocked()
	}()
}

// ownedGCSweepLocked runs one incremental ownership sweep against the complete
// active root set. It evicts strictly less than the Eval-end sweep would: the
// root set below is a superset of snapshotOwnedReachabilityValues(root) plus
// every capture cell, so cancel/detach isolation is preserved without any
// relaxation of the eviction predicates. Locking: the caller holds the
// funcSweepMu fence exclusively (via maybeRunOwnedGCSweep's TryLock, or
// directly from prepareExecutionFrame) and must not hold funcMu; the body
// takes funcMu for its whole extent and reads frame cells via
// snapshotFrameValues under f.mutex.RLock taken after funcMu, matching
// releaseOwnedObjects' order. The exclusive fence guarantees no interpreted
// step mutates frame cells or registries concurrently.
func (interp *Interpreter) ownedGCSweepLocked() {
	// (1) Durable globals of the current root. Taken before funcMu: the
	// snapshot acquires interp.mutex and the root's frame mutex only.
	values := interp.snapshotOwnedReachabilityValues(interp.frame)
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	// (2) Every frame with a live runCfg activation, plus its full ancestor
	// chain up to and including its root: ALL cells, regardless of funcState.
	// This covers parked-in-native and zombie-draining frames; the frame
	// registry itself is maintained by runCfg.
	seenFrames := map[*frame]struct{}{}
	for f := range interp.activeFrames {
		for current := f; current != nil; current = current.anc {
			if _, seen := seenFrames[current]; seen {
				break
			}
			seenFrames[current] = struct{}{}
			values = append(values, snapshotFrameValues(current)...)
		}
	}
	// (3) Every funcMeta group's capture cells, read directly under funcMu.
	// snapshotFuncMetaCapture cannot be used here: it acquires funcMu.RLock
	// (self-deadlock under funcMu.Lock) and returns a copy, whose fresh
	// storage would defeat the interior-pointer keep-alive of the interval
	// collector. The exclusive fence makes the direct reads race-free.
	for _, meta := range interp.funcMeta {
		if meta.group == nil {
			continue
		}
		for _, capture := range meta.group.captures {
			if capture.frame == nil || capture.index < 0 || capture.index >= len(capture.frame.data) {
				continue
			}
			values = append(values, capture.frame.data[capture.index])
		}
	}
	// (4) directFuncs activations.
	for _, activation := range interp.directFuncs {
		values = append(values, activation.value)
	}
	// One collector traversal over the union drives both evictions: object
	// eviction uses exactly the sweepRootOwnedObjects predicate (see
	// sweepOwnedObjectsValuesLocked), generalized to candidates across every
	// root — an object unreachable from the union is dead in the same sense
	// the Eval-end sweep already treats unreachable current-root objects —
	// while the channel extension records owned-channel hits for the channel
	// eviction below.
	reachableChannels := map[uintptr]struct{}{}
	interp.sweepOwnedObjectsValuesLocked(nil, values, reachableChannels)
	// Channel eviction (F1c): mirror sweepRootOwnedChannels semantics against
	// the larger root set. Candidates must be non-host-visible, carry no
	// non-terminal sends (terminal ones are already removed from .sends by
	// retireOwnedChannelSendLocked), and be unreachable from the union. An
	// owner frame with a live non-root activation is kept: a draining or
	// mid-flight frame may hold the only reference in its deferred call
	// values. A channel value buffered inside another channel is not
	// reflect-traversable, so channels referenced by pending send values are
	// pinned explicitly.
	pendingSendValues := []reflect.Value{}
	for _, channel := range interp.ownedChannels {
		for _, send := range channel.sends {
			pendingSendValues = append(pendingSendValues, send.value)
			if send.pending.IsValid() {
				pendingSendValues = append(pendingSendValues, send.pending)
			}
		}
	}
	unreachable := []*ownedChannel{}
	for ptr, channel := range interp.ownedChannels {
		if channel.hostVisible || len(channel.sends) != 0 {
			continue
		}
		if _, ok := reachableChannels[ptr]; ok {
			continue
		}
		if owner := channel.owner; owner != nil && owner != owner.root && interp.activeFrames[owner] > 0 {
			continue
		}
		pinned := false
		for _, value := range pendingSendValues {
			if interp.ownedValueContainsChannelLocked(value, ptr, map[*ownedObject]struct{}{}) {
				pinned = true
				break
			}
		}
		if pinned {
			continue
		}
		unreachable = append(unreachable, channel)
	}
	for _, channel := range unreachable {
		delete(interp.ownedChannels, channel.hold.Pointer())
		if channel.owner != nil {
			delete(channel.owner.ownedChannels, channel)
		}
		channel.sends = nil
	}

	// (5) directFuncs activation cache eviction. Each entry pins its root
	// frame (transitively the root's globals) and its cloned wrapper, so an
	// unbounded interpreter accumulates one pinned root per detached
	// generation and one dead line per swept-away source. An entry is
	// evicted when its root is no longer live — not the durable root, not a
	// root reachable from any active frame chain, and not referenced by any
	// retained funcMeta metadata — or when neither endpoint can any longer
	// resolve to metadata, mirroring the endpoint rule of the metadata
	// purge. Eviction is transparent: the cache is consulted only after a
	// successful metadata lookup keyed by the live activation root, so a
	// dropped line costs at most one extra clone.
	if len(interp.directFuncs) > 0 {
		liveRoots := map[*frame]struct{}{interp.frame: {}}
		for f := range seenFrames {
			if f.root != nil {
				liveRoots[f.root] = struct{}{}
			}
		}
		for _, meta := range interp.funcMeta {
			if meta.frame != nil && meta.frame.root != nil {
				liveRoots[meta.frame.root] = struct{}{}
			}
			if meta.group != nil && meta.group.root != nil {
				liveRoots[meta.group.root] = struct{}{}
			}
		}
		exactKeys := make(map[uintptr]struct{}, len(interp.funcMeta))
		for key := range interp.funcMeta {
			exactKeys[key] = struct{}{}
		}
		for key, activation := range interp.directFuncs {
			if _, live := liveRoots[key.root]; !live {
				delete(interp.directFuncs, key)
				continue
			}
			if _, live := exactKeys[key.source]; live {
				continue
			}
			if _, live := exactKeys[activation.key]; !live {
				delete(interp.directFuncs, key)
			}
		}
	}
}

// ownedInterval is an address range of a value node visited during a
// reachability traversal.
type ownedInterval struct {
	base, end uintptr
}

// visitedIntervalCollector records the address ranges of value nodes visited
// by collectReachableObjectsLocked. Ptr-kind owned objects whose address falls
// inside a visited range are reachable even when no registry lookup resolves
// them, mirroring valueContainsAddress checks in the per-target walk.
type visitedIntervalCollector struct {
	intervals []ownedInterval
	maxEnd    []uintptr
}

func (v *visitedIntervalCollector) record(value reflect.Value) {
	if !value.IsValid() || !value.CanAddr() {
		return
	}
	base := value.Addr().Pointer()
	size := value.Type().Size()
	end := base + size
	if size == 0 {
		end = base + 1
	}
	v.intervals = append(v.intervals, ownedInterval{base: base, end: end})
	if n := len(v.maxEnd); n > 0 && v.maxEnd[n-1] > end {
		v.maxEnd = append(v.maxEnd, v.maxEnd[n-1])
	} else {
		v.maxEnd = append(v.maxEnd, end)
	}
}

// sort orders the intervals by base and rebuilds the prefix max-end table.
func (v *visitedIntervalCollector) sort() {
	sort.Slice(v.intervals, func(i, j int) bool { return v.intervals[i].base < v.intervals[j].base })
	v.maxEnd = v.maxEnd[:0]
	for _, interval := range v.intervals {
		if n := len(v.maxEnd); n > 0 && v.maxEnd[n-1] > interval.end {
			v.maxEnd = append(v.maxEnd, v.maxEnd[n-1])
		} else {
			v.maxEnd = append(v.maxEnd, interval.end)
		}
	}
}

// contains reports whether ptr falls inside any recorded interval. The list
// is sorted by base; maxEnd[i] holds the largest end among intervals[0..i],
// so among the intervals whose base is <= ptr the one ending latest is still
// accounted for (nested and sibling intervals both covered).
func (v *visitedIntervalCollector) contains(ptr uintptr) bool {
	i := sort.Search(len(v.intervals), func(i int) bool { return v.intervals[i].base > ptr })
	if i == 0 {
		return false
	}
	return ptr < v.maxEnd[i-1]
}

// collectReachableObjectsLocked gathers every owned object resolvable from v
// into reachable, and records the address interval of every visited node.
// When channels is non-nil, every registered owned channel whose value is
// found in the traversed graph is recorded by pointer, so a channel sweep can
// reuse the same traversal (existing callers pass nil). Traversal pruning
// mirrors ownedValueContainsObjectLocked: containers that resolve to no owned
// object are not descended, hostShared objects stop the descent, and
// already-seen objects prune revisits of identical storage.
func (interp *Interpreter) collectReachableObjectsLocked(v reflect.Value, reachable map[*ownedObject]struct{}, seen map[*ownedObject]struct{}, visited *visitedIntervalCollector, channels map[uintptr]struct{}) {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return
	}
	visited.record(v)
	switch v.Kind() {
	case reflect.Chan:
		if channels == nil || v.IsNil() {
			return
		}
		if ptr := v.Pointer(); ptr != 0 {
			if _, owned := interp.ownedChannels[ptr]; owned {
				channels[ptr] = struct{}{}
			}
		}
	case reflect.Map, reflect.Ptr, reflect.Slice:
		objects := interp.ownedObjectsForValueLocked(v)
		if len(objects) == 0 {
			return
		}
		for _, obj := range objects {
			reachable[obj] = struct{}{}
			if obj.hostShared {
				return
			}
			if _, ok := seen[obj]; ok {
				return
			}
			seen[obj] = struct{}{}
		}
		switch v.Kind() {
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				interp.collectReachableObjectsLocked(iter.Key(), reachable, seen, visited, channels)
				interp.collectReachableObjectsLocked(iter.Value(), reachable, seen, visited, channels)
			}
		case reflect.Ptr:
			interp.collectReachableObjectsLocked(v.Elem(), reachable, seen, visited, channels)
		case reflect.Slice:
			full := v.Slice(0, v.Cap())
			for index := 0; index < full.Len(); index++ {
				interp.collectReachableObjectsLocked(full.Index(index), reachable, seen, visited, channels)
			}
		}
	case reflect.UnsafePointer:
		for _, obj := range interp.ownedObjectsForValueLocked(v) {
			reachable[obj] = struct{}{}
		}
	case reflect.Struct:
		if interp.ownedCellHostSharedLocked(v) {
			return
		}
		for index := 0; index < v.NumField(); index++ {
			if v.Field(index).CanInterface() {
				interp.collectReachableObjectsLocked(v.Field(index), reachable, seen, visited, channels)
			}
		}
	case reflect.Array:
		if interp.ownedCellHostSharedLocked(v) {
			return
		}
		for index := 0; index < v.Len(); index++ {
			interp.collectReachableObjectsLocked(v.Index(index), reachable, seen, visited, channels)
		}
	}
}

type ownedCellKey struct {
	ptr uintptr
	typ reflect.Type
}

type detachedFuncClone struct {
	oldKey uintptr
	value  reflect.Value
	meta   interpretedFuncMeta
}

type detachedRootCloner struct {
	interp  *Interpreter
	oldRoot *frame
	newRoot *frame
	cancel  <-chan struct{}

	objects          map[objectKey]*ownedObject
	objectSet        map[*ownedObject]struct{}
	objectMemo       map[*ownedObject]reflect.Value
	cells            map[ownedCellKey]reflect.Value
	funcMeta         map[uintptr]interpretedFuncMeta
	funcMemo         map[uintptr]reflect.Value
	funcBuilding     map[uintptr]bool
	frameMemo        map[*frame]*frame
	groupMemo        map[*funcMetaGroup]*funcMetaGroup
	funcRepairs      map[uintptr][]reflect.Value
	funcClones       []detachedFuncClone
	newObjects       map[objectKey]*ownedObject
	pending          map[*ownedObject]reflect.Value
	rehomeAllFuncs   bool
	directLineage    map[directFuncActivationKey]directFuncActivation
	directPromotions map[directFuncActivationKey]directFuncActivation
}

func newDetachedRootCloner(interp *Interpreter, oldRoot, newRoot *frame, cancel <-chan struct{}) *detachedRootCloner {
	c := &detachedRootCloner{
		interp: interp, oldRoot: oldRoot, newRoot: newRoot, cancel: cancel,
		objects: map[objectKey]*ownedObject{}, objectSet: map[*ownedObject]struct{}{},
		objectMemo: map[*ownedObject]reflect.Value{}, cells: map[ownedCellKey]reflect.Value{},
		funcMeta: map[uintptr]interpretedFuncMeta{}, funcMemo: map[uintptr]reflect.Value{},
		funcBuilding: map[uintptr]bool{}, frameMemo: map[*frame]*frame{},
		groupMemo:        map[*funcMetaGroup]*funcMetaGroup{},
		funcRepairs:      map[uintptr][]reflect.Value{},
		newObjects:       map[objectKey]*ownedObject{},
		pending:          map[*ownedObject]reflect.Value{},
		directLineage:    map[directFuncActivationKey]directFuncActivation{},
		directPromotions: map[directFuncActivationKey]directFuncActivation{},
	}
	interp.funcMu.RLock()
	for key, obj := range interp.ownedObjects {
		c.objects[key] = obj
		c.objectSet[obj] = struct{}{}
	}
	for key, meta := range interp.funcMeta {
		c.funcMeta[key] = meta
	}
	for key, activation := range interp.directFuncs {
		if key.root == oldRoot {
			c.directLineage[key] = activation
		}
	}
	interp.funcMu.RUnlock()
	return c
}

func (c *detachedRootCloner) cloneDirectFuncLineage() {
	for key, active := range c.directLineage {
		clone := c.cloneFunc(active.value)
		if clone.IsValid() && !sameFuncvalKey(clone, active.value) {
			c.directPromotions[directFuncActivationKey{source: key.source, root: c.newRoot}] = newDirectFuncActivation(clone)
			if active.key != 0 {
				c.directPromotions[directFuncActivationKey{source: active.key, root: c.newRoot}] = newDirectFuncActivation(clone)
			}
			if cloneKey, ok := funcvalKeyOf(clone); ok {
				c.directPromotions[directFuncActivationKey{source: cloneKey, root: c.newRoot}] = newDirectFuncActivation(clone)
			}
		}
	}
}

func (c *detachedRootCloner) seedCell(oldValue, newValue reflect.Value) {
	if !oldValue.IsValid() || !newValue.IsValid() || oldValue.Type() != newValue.Type() {
		return
	}
	if oldValue.CanAddr() && newValue.CanAddr() {
		oldAddr := oldValue.Addr()
		key := ownedCellKey{ptr: oldAddr.Pointer(), typ: oldAddr.Type()}
		if _, exists := c.cells[key]; !exists {
			c.cells[key] = newValue.Addr()
		}
	}
	if oldValue.IsValid() && oldValue.Type() == valueInterfaceType && oldValue.CanInterface() {
		wrapped := oldValue.Interface().(valueInterface).value
		if wrapped.IsValid() && c.cellHostShared(wrapped) {
			c.seedCell(wrapped, wrapped)
		}
		return
	}
	switch oldValue.Kind() {
	case reflect.Struct:
		for i := 0; i < oldValue.NumField(); i++ {
			c.seedCell(oldValue.Field(i), newValue.Field(i))
		}
	case reflect.Array:
		for i := 0; i < oldValue.Len(); i++ {
			c.seedCell(oldValue.Index(i), newValue.Index(i))
		}
	}
}

// seedOwnedCell makes the cloned allocation authoritative for every interior
// view. Root-frame temporaries may alias the same address and are seeded before
// allocations are cloned; those provisional mappings must not split a field or
// slice element from its containing allocation.
func (c *detachedRootCloner) seedOwnedCell(oldValue, newValue reflect.Value) {
	if !oldValue.IsValid() || !newValue.IsValid() || oldValue.Type() != newValue.Type() {
		return
	}
	if oldValue.CanAddr() && newValue.CanAddr() {
		oldAddr := oldValue.Addr()
		c.cells[ownedCellKey{ptr: oldAddr.Pointer(), typ: oldAddr.Type()}] = newValue.Addr()
	}
	switch oldValue.Kind() {
	case reflect.Struct:
		for i := 0; i < oldValue.NumField(); i++ {
			c.seedOwnedCell(oldValue.Field(i), newValue.Field(i))
		}
	case reflect.Array:
		for i := 0; i < oldValue.Len(); i++ {
			c.seedOwnedCell(oldValue.Index(i), newValue.Index(i))
		}
	}
}

func (c *detachedRootCloner) mappedCell(oldValue reflect.Value) (reflect.Value, bool) {
	if !oldValue.IsValid() || !oldValue.CanAddr() {
		return reflect.Value{}, false
	}
	address := oldValue.Addr()
	mapped, ok := c.cells[ownedCellKey{ptr: address.Pointer(), typ: address.Type()}]
	if !ok || mapped.Kind() != reflect.Ptr {
		return reflect.Value{}, false
	}
	if mapped.Type() != address.Type() {
		if !mapped.Type().ConvertibleTo(address.Type()) {
			return reflect.Value{}, false
		}
		mapped = mapped.Convert(address.Type())
	}
	return mapped.Elem(), true
}

func (c *detachedRootCloner) objectFor(v reflect.Value) *ownedObject {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Map:
		if !v.IsNil() {
			obj := c.objects[objectKey{kind: v.Kind(), ptr: v.Pointer()}]
			if obj != nil && obj.hostShared {
				return nil
			}
			return obj
		}
	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		obj := c.objects[objectKey{kind: v.Kind(), typ: v.Type(), ptr: v.Pointer()}]
		if obj == nil {
			for candidate := range c.objectSet {
				if candidate.key.kind == reflect.Ptr && candidate.key.ptr == v.Pointer() && candidate.hold.Type().ConvertibleTo(v.Type()) {
					obj = candidate
					break
				}
			}
		}
		if obj != nil && obj.hostShared {
			return nil
		}
		return obj
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 || v.Type().Elem().Size() == 0 {
			return nil
		}
		ptr := v.Pointer()
		end := ptr + uintptr(v.Cap())*v.Type().Elem().Size()
		for obj := range c.objectSet {
			if obj.key.kind == reflect.Slice && obj.sliceElem == v.Type().Elem() &&
				ptr >= obj.sliceBase && end <= obj.sliceEnd {
				if obj.hostShared {
					return nil
				}
				return obj
			}
		}
	}
	return nil
}

func (c *detachedRootCloner) valueHostShared(v reflect.Value) bool {
	v = unwrapOwnedValue(v)
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return false
		}
		for obj := range c.objectSet {
			if obj.hostShared && obj.key.kind == reflect.Map && obj.key.ptr == v.Pointer() {
				return true
			}
		}
	case reflect.Ptr:
		if v.IsNil() {
			return false
		}
		ptr := v.Pointer()
		for obj := range c.objectSet {
			base, end, ok := ownedObjectInterval(obj)
			if obj.hostShared && ok && ptr >= base && ptr < end {
				return true
			}
		}
	case reflect.Slice:
		if v.IsNil() || v.Cap() == 0 || v.Type().Elem().Size() == 0 {
			return false
		}
		base := v.Pointer()
		end := base + uintptr(v.Cap())*v.Type().Elem().Size()
		for obj := range c.objectSet {
			ob, oe, ok := ownedObjectInterval(obj)
			if obj.hostShared && ok && base < oe && ob < end {
				return true
			}
		}
	}
	return false
}

func (c *detachedRootCloner) cellHostShared(v reflect.Value) bool {
	v = unwrapOwnedValue(v)
	if !v.IsValid() || !v.CanAddr() {
		return false
	}
	base := v.Addr().Pointer()
	size := v.Type().Size()
	end := base + size
	if size == 0 {
		end = base + 1
	}
	for obj := range c.objectSet {
		ob, oe, ok := ownedObjectInterval(obj)
		if obj.hostShared && ok && base < oe && ob < end {
			return true
		}
	}
	return false
}

func (c *detachedRootCloner) sliceForPointer(v reflect.Value) *ownedObject {
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	ptr, elem := v.Pointer(), v.Type().Elem()
	for obj := range c.objectSet {
		if !obj.hostShared && obj.key.kind == reflect.Slice && obj.sliceElem == elem && ptr >= obj.sliceBase && ptr < obj.sliceEnd &&
			(ptr-obj.sliceBase)%elem.Size() == 0 {
			return obj
		}
	}
	return nil
}

func (c *detachedRootCloner) objectContainingPointer(v reflect.Value) *ownedObject {
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	ptr := v.Pointer()
	var selected *ownedObject
	var selectedSize uintptr
	for obj := range c.objectSet {
		if obj.hostShared || obj.key.kind != reflect.Ptr {
			continue
		}
		base := obj.key.ptr
		size := obj.hold.Type().Elem().Size()
		if ptr >= base && (size == 0 && ptr == base || size > 0 && ptr < base+size) {
			compatible := obj.hold.Type() == v.Type() || obj.hold.Type().ConvertibleTo(v.Type())
			selectedCompatible := selected != nil && (selected.hold.Type() == v.Type() || selected.hold.Type().ConvertibleTo(v.Type()))
			container := obj.hold.Type().Elem().Kind() == reflect.Struct || obj.hold.Type().Elem().Kind() == reflect.Array
			selectedContainer := selected != nil && (selected.hold.Type().Elem().Kind() == reflect.Struct || selected.hold.Type().Elem().Kind() == reflect.Array)
			if selected == nil || size > selectedSize || size == selectedSize && container && !selectedContainer ||
				size == selectedSize && container == selectedContainer && compatible && !selectedCompatible {
				selected = obj
				selectedSize = size
			}
		}
	}
	return selected
}

func (c *detachedRootCloner) objectContainingAddress(ptr uintptr) *ownedObject {
	var selected *ownedObject
	var selectedSize uintptr
	for obj := range c.objectSet {
		base, end, ok := ownedObjectInterval(obj)
		if !ok || ptr < base || ptr >= end {
			continue
		}
		size := end - base
		if selected == nil || size > selectedSize {
			selected = obj
			selectedSize = size
		}
	}
	return selected
}

func (c *detachedRootCloner) addNewObject(v reflect.Value) {
	if obj, ok := describeOwnedObject(v, c.newRoot); ok {
		c.newObjects[obj.key] = obj
	}
}

// snapshotPendingEscapes freezes values which are already buffered in a
// channel before the old root is abandoned. The channel itself remains
// shallow, so receive-time adoption substitutes these private snapshots rather
// than publishing an object which a canceled activation can still mutate.
func (c *detachedRootCloner) snapshotPendingEscapes() {
	c.interp.funcMu.Lock()
	activeSends := []*ownedChannelSend{}
	for _, channel := range c.interp.ownedChannels {
		if channel.hostVisible {
			continue
		}
		for _, send := range channel.sends {
			if send.state == ownedChannelSendTerminal || send.sender == nil || send.sender.root != c.oldRoot && send.pendingRoot != c.oldRoot {
				continue
			}
			if send.pendingRoot != c.oldRoot {
				c.interp.refreshOwnedChannelSendLocked(send)
			}
			activeSends = append(activeSends, send)
		}
	}
	c.interp.funcMu.Unlock()
	for _, send := range activeSends {
		source := send.value
		if send.pendingRoot == c.oldRoot && send.pending.IsValid() {
			source = send.pending
		}
		c.rehomeAllFuncs = true
		send.pending = c.cloneValue(source, true)
		c.rehomeAllFuncs = false
		send.pendingRoot = c.newRoot
	}

	c.interp.funcMu.RLock()
	tokens := make([]*ownedPanicToken, 0, len(c.interp.panicTokens))
	for token := range c.interp.panicTokens {
		if token.finished {
			continue
		}
		relevant := token.pendingRoot == c.oldRoot
		if !relevant && token.pendingRoot == nil {
			for obj := range token.objects {
				if obj.owner != nil && obj.owner.root == c.oldRoot {
					relevant = true
					break
				}
			}
			if !relevant {
				for group := range token.groups {
					if group != nil && group.root == c.oldRoot {
						relevant = true
						break
					}
				}
			}
		}
		if relevant {
			tokens = append(tokens, token)
		}
	}
	c.interp.funcMu.RUnlock()
	for _, token := range tokens {
		source := token.value
		if token.pendingRoot == c.oldRoot && token.pending.IsValid() {
			source = token.pending
		}
		c.rehomeAllFuncs = true
		token.pending = c.cloneValue(source, true)
		c.rehomeAllFuncs = false
		token.pendingRoot = c.newRoot
	}
}

func (c *detachedRootCloner) cloneOwnedSlice(obj *ownedObject) reflect.Value {
	if cloned, ok := c.objectMemo[obj]; ok {
		return cloned
	}
	oldFull := obj.hold
	newFull := reflect.MakeSlice(oldFull.Type(), obj.sliceCap, obj.sliceCap)
	c.objectMemo[obj] = newFull
	c.addNewObject(newFull)
	for i := 0; i < obj.sliceCap; i++ {
		c.seedOwnedCell(oldFull.Index(i), newFull.Index(i))
	}
	for i := 0; i < obj.sliceCap; i++ {
		newFull.Index(i).Set(c.cloneValue(oldFull.Index(i), c.rehomeAllFuncs))
	}
	return newFull
}

func (c *detachedRootCloner) clonePointer(v reflect.Value, obj *ownedObject) reflect.Value {
	if c.valueHostShared(v) {
		return v
	}
	key := ownedCellKey{ptr: v.Pointer(), typ: v.Type()}
	if container := c.objectContainingPointer(v); container != nil && container != obj {
		c.clonePointer(container.hold, container)
		if mapped, ok := c.cells[key]; ok {
			if mapped.Type() == v.Type() {
				return mapped
			}
			if mapped.Type().ConvertibleTo(v.Type()) {
				return mapped.Convert(v.Type())
			}
		}
	}
	if mapped, ok := c.cells[key]; ok {
		if mapped.Type() == v.Type() {
			return mapped
		}
		if mapped.Type().ConvertibleTo(v.Type()) {
			return mapped.Convert(v.Type())
		}
	}
	if sliceObj := c.sliceForPointer(v); sliceObj != nil {
		c.cloneOwnedSlice(sliceObj)
		if mapped, ok := c.cells[key]; ok {
			return mapped
		}
	}
	if obj == nil {
		return v
	}
	if cloned, ok := c.objectMemo[obj]; ok {
		if cloned.Type() == v.Type() {
			return cloned
		}
		if cloned.Type().ConvertibleTo(v.Type()) {
			return cloned.Convert(v.Type())
		}
		return v
	}
	cloned := reflect.New(v.Type().Elem())
	c.objectMemo[obj] = cloned
	c.cells[key] = cloned
	c.seedOwnedCell(v.Elem(), cloned.Elem())
	c.addNewObject(cloned)
	cloned.Elem().Set(c.cloneValue(v.Elem(), c.rehomeAllFuncs))
	if cloned.Type() == v.Type() {
		return cloned
	}
	if cloned.Type().ConvertibleTo(v.Type()) {
		return cloned.Convert(v.Type())
	}
	return v
}

func (c *detachedRootCloner) cloneMap(v reflect.Value, obj *ownedObject) reflect.Value {
	if obj == nil {
		return v
	}
	if cloned, ok := c.objectMemo[obj]; ok {
		if cloned.Type() == v.Type() {
			return cloned
		}
		if cloned.Type().ConvertibleTo(v.Type()) {
			return cloned.Convert(v.Type())
		}
		return v
	}
	cloned := reflect.MakeMapWithSize(v.Type(), v.Len())
	c.objectMemo[obj] = cloned
	c.addNewObject(cloned)
	iter := v.MapRange()
	for iter.Next() {
		cloned.SetMapIndex(c.cloneValue(iter.Key(), c.rehomeAllFuncs), c.cloneValue(iter.Value(), c.rehomeAllFuncs))
	}
	return cloned
}

func (c *detachedRootCloner) cloneSlice(v reflect.Value, obj *ownedObject) reflect.Value {
	if obj == nil {
		return v
	}
	newFull := c.cloneOwnedSlice(obj)
	offset := int((v.Pointer() - obj.sliceBase) / v.Type().Elem().Size())
	cloned := newFull.Slice3(offset, offset+v.Len(), offset+v.Cap())
	if cloned.Type() != v.Type() && cloned.Type().ConvertibleTo(v.Type()) {
		cloned = cloned.Convert(v.Type())
	}
	return cloned
}

func (c *detachedRootCloner) cloneUnsafePointer(v reflect.Value) reflect.Value {
	ptr := v.Pointer()
	if ptr == 0 {
		return reflect.Zero(v.Type())
	}
	obj := c.objectContainingAddress(ptr)
	if obj != nil {
		if obj.hostShared {
			return v
		}
		base, _, _ := ownedObjectInterval(obj)
		var clonedBase unsafe.Pointer
		switch obj.key.kind {
		case reflect.Ptr:
			clonedBase = c.clonePointer(obj.hold, obj).UnsafePointer()
		case reflect.Slice:
			clonedBase = c.cloneOwnedSlice(obj).UnsafePointer()
		}
		if clonedBase != nil {
			return reflect.ValueOf(unsafe.Add(clonedBase, ptr-base)).Convert(v.Type())
		}
	}
	for key, mapped := range c.cells {
		if key.ptr == ptr && mapped.Kind() == reflect.Ptr {
			return reflect.ValueOf(unsafe.Pointer(mapped.Pointer())).Convert(v.Type())
		}
	}
	return v
}

func (c *detachedRootCloner) cloneFunc(v reflect.Value) reflect.Value {
	key, ok := funcvalKeyOf(v)
	if !ok {
		return v
	}
	if cloned, ok := c.funcMemo[key]; ok {
		return cloned
	}
	if c.funcBuilding[key] {
		// A closure's lexical carrier normally contains the wrapper itself.
		// The carrier is repaired by the rebuilt wrapper after this recursive
		// edge unwinds.
		return v
	}
	meta, ok := c.funcMeta[key]
	if !ok || meta.rebind == nil {
		return v
	}
	c.funcBuilding[key] = true
	build := meta.rebind(c)
	delete(c.funcBuilding, key)
	cloned := build.value
	if !cloned.IsValid() {
		return v
	}
	c.funcMemo[key] = cloned
	for _, dest := range c.funcRepairs[key] {
		setClonedDirectFunc(dest, cloned)
	}
	delete(c.funcRepairs, key)
	meta.invoke = build.invoke
	meta.rebind = build.rebind
	meta.captures = append([]funcMetaCapture(nil), build.captures...)
	if meta.group != nil {
		group := c.groupMemo[meta.group]
		if group == nil {
			group = &funcMetaGroup{root: c.newRoot, version: 1}
			c.groupMemo[meta.group] = group
		}
		for _, capture := range build.captures {
			found := false
			for _, existing := range group.captures {
				if existing == capture {
					found = true
					break
				}
			}
			if !found {
				group.captures = append(group.captures, capture)
			}
		}
		meta.group = group
	}
	c.funcClones = append(c.funcClones, detachedFuncClone{oldKey: key, value: cloned, meta: meta})
	return cloned
}

// newDirectFuncActivation records a cloned wrapper and its funcval key. The
// key is derived eagerly (the wrapper is alive at every call site) so eviction
// paths never allocate under funcMu.
func newDirectFuncActivation(value reflect.Value) directFuncActivation {
	activation := directFuncActivation{value: value}
	if key, ok := funcvalKeyOf(value); ok {
		activation.key = key
	}
	return activation
}

func directFuncKey(v reflect.Value) (uintptr, bool) {
	for v.IsValid() {
		if v.Type() == valueInterfaceType && v.CanInterface() {
			v = v.Interface().(valueInterface).value
			continue
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return 0, false
			}
			v = v.Elem()
			continue
		}
		break
	}
	return funcvalKeyOf(v)
}

func setClonedDirectFunc(dest, cloned reflect.Value) {
	if !dest.IsValid() || !dest.CanSet() || !cloned.IsValid() {
		return
	}
	if dest.Type() == valueInterfaceType {
		dest.Set(reflect.ValueOf(valueInterface{value: cloned}))
		return
	}
	if cloned.Type().AssignableTo(dest.Type()) || dest.Kind() == reflect.Interface && cloned.Type().Implements(dest.Type()) {
		dest.Set(cloned)
	}
}

func (c *detachedRootCloner) cloneFrame(old *frame) *frame {
	if old == nil {
		return nil
	}
	if old == c.oldRoot {
		return c.newRoot
	}
	if cloned, ok := c.frameMemo[old]; ok {
		return cloned
	}
	old.mutex.RLock()
	data := append([]reflect.Value(nil), old.data...)
	anc, cloneOf := old.anc, old.cloneOf
	id, debug := old.runid(), old.debug
	old.mutex.RUnlock()
	cloned := &frame{
		interp: c.interp, root: c.newRoot, id: id, debug: debug,
		cancel: c.cancel,
		done:   reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(c.cancel)},
		data:   make([]reflect.Value, len(data)),
	}
	c.frameMemo[old] = cloned
	cloned.anc = c.cloneFrame(anc)
	cloned.cloneOf = c.cloneFrame(cloneOf)
	sharedCells := make([]bool, len(data))
	for i, value := range data {
		if value.IsValid() {
			if mapped, ok := c.mappedCell(value); ok {
				cloned.data[i] = mapped
				sharedCells[i] = true
			} else if c.cellHostShared(value) {
				cloned.data[i] = value
				sharedCells[i] = true
			} else {
				cloned.data[i] = reflect.New(value.Type()).Elem()
			}
			c.seedCell(value, cloned.data[i])
		}
	}
	for i, value := range data {
		if value.IsValid() && !sharedCells[i] {
			cloned.data[i].Set(c.cloneValue(value, true))
			if key, ok := directFuncKey(value); ok && c.funcBuilding[key] {
				c.funcRepairs[key] = append(c.funcRepairs[key], cloned.data[i])
			}
		}
	}
	return cloned
}

func (c *detachedRootCloner) cloneValue(v reflect.Value, rehomeFunc bool) reflect.Value {
	if !v.IsValid() {
		return v
	}
	if v.Type() == valueInterfaceType && v.CanInterface() {
		wrapped := v.Interface().(valueInterface)
		if c.cellHostShared(wrapped.value) {
			c.seedCell(wrapped.value, wrapped.value)
			return v
		}
		wrapped.value = c.cloneValue(wrapped.value, rehomeFunc)
		return reflect.ValueOf(wrapped)
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(c.cloneValue(v.Elem(), rehomeFunc))
		return out
	case reflect.Ptr:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		return c.clonePointer(v, c.objectFor(v))
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		return c.cloneMap(v, c.objectFor(v))
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		return c.cloneSlice(v, c.objectFor(v))
	case reflect.UnsafePointer:
		return c.cloneUnsafePointer(v)
	case reflect.Func:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		if rehomeFunc {
			return c.cloneFunc(v)
		}
		return v
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanInterface() {
				out.Field(i).Set(c.cloneValue(v.Field(i), rehomeFunc || c.rehomeAllFuncs))
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(c.cloneValue(v.Index(i), rehomeFunc || c.rehomeAllFuncs))
		}
		return out
	}
	out := reflect.New(v.Type()).Elem()
	out.Set(v)
	return out
}

func (c *detachedRootCloner) commit() {
	c.interp.funcMu.Lock()
	defer c.interp.funcMu.Unlock()
	for key := range c.interp.directFuncs {
		if key.root == c.oldRoot {
			delete(c.interp.directFuncs, key)
		}
	}
	for _, channel := range c.interp.ownedChannels {
		if channel.owner != c.oldRoot || channel.hostVisible {
			continue
		}
		delete(c.oldRoot.ownedChannels, channel)
		channel.owner = c.newRoot
		if c.newRoot.ownedChannels == nil {
			c.newRoot.ownedChannels = map[*ownedChannel]struct{}{}
		}
		c.newRoot.ownedChannels[channel] = struct{}{}
	}
	for _, obj := range c.interp.ownedObjects {
		if obj.owner != nil && obj.owner.root == c.oldRoot && !obj.hostShared && obj.channelRefs == 0 && !ownedObjectHasPanicToken(obj) {
			capturedByOpaque := false
			for _, meta := range c.interp.funcMeta {
				if meta.retention != funcMetaOpaque || meta.frame == nil || meta.frame.root != c.oldRoot || meta.group == nil {
					continue
				}
				for _, capture := range meta.group.captures {
					if capture.frame != nil && capture.index >= 0 && capture.index < len(capture.frame.data) &&
						c.interp.ownedValueContainsObjectLocked(capture.frame.data[capture.index], obj, map[*ownedObject]struct{}{}) {
						capturedByOpaque = true
						break
					}
				}
				if capturedByOpaque {
					break
				}
			}
			if capturedByOpaque {
				continue
			}
			c.interp.unregisterOwnedObjectLocked(obj)
		}
	}
	for obj, pending := range c.pending {
		obj.pendingRoot = c.newRoot
		obj.pending = pending
	}
	for key, obj := range c.newObjects {
		if obj.hostShared {
			// Clones are struct copies; keep the hostShared estimate exact
			// for objects that inherit the flag from their source.
			c.interp.hostSharedEstimate++
		}
		c.interp.ownedObjects[key] = obj
		c.interp.armOwnedGCLocked()
		if c.newRoot.ownedObjects == nil {
			c.newRoot.ownedObjects = map[*ownedObject]struct{}{}
		}
		c.newRoot.ownedObjects[obj] = struct{}{}
	}
	for _, clone := range c.funcClones {
		newRef, ok := funcvalRefOf(clone.value)
		if !ok {
			continue
		}
		group := clone.meta.group
		if group == nil {
			group = &funcMetaGroup{root: c.newRoot, version: 1}
		}
		meta := clone.meta
		meta.frame = c.newRoot
		meta.group = group
		c.interp.insertFuncMetaEntryLocked(newRef, meta, c.newRoot)
		if clone.meta.retention == funcMetaVisible {
			delete(c.interp.funcMeta, clone.oldKey)
		}
	}
	for key, activation := range c.directPromotions {
		c.interp.directFuncs[key] = activation
	}
	c.interp.armOwnedGCLocked()
	for token := range c.interp.panicTokens {
		if token.finished || token.pendingRoot != c.newRoot || !token.pending.IsValid() {
			continue
		}
		oldObjects := token.pendingObjects
		oldFuncs := token.pendingFuncs
		oldGroups := token.pendingGroups
		objects := map[*ownedObject]struct{}{}
		funcs := map[reflect.Value]struct{}{}
		groups := map[*funcMetaGroup]struct{}{}
		c.interp.collectOwnedChannelGraphLocked(token.pending, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
		token.pendingObjects = objects
		token.pendingFuncs = funcs
		token.pendingGroups = groups
		c.interp.attachOwnedPanicTokenMembershipsLocked(token, objects, funcs, groups, token.pendingFrames)
		for obj := range oldObjects {
			if _, raw := token.objects[obj]; raw {
				continue
			}
			if _, current := objects[obj]; current {
				continue
			}
			delete(obj.panicTokens, token)
			if obj.owner != nil && obj.owner.root == c.oldRoot && !obj.hostShared && obj.channelRefs == 0 && !ownedObjectHasPanicToken(obj) {
				delete(c.interp.ownedObjects, obj.key)
				delete(obj.owner.ownedObjects, obj)
			}
		}
		for group := range oldGroups {
			if _, raw := token.groups[group]; raw {
				continue
			}
			if _, current := groups[group]; current {
				continue
			}
			delete(group.panicTokens, token)
			c.interp.releaseOwnedPanicGroupLocked(group)
		}
		for value := range oldFuncs {
			if _, raw := token.funcs[value]; raw {
				continue
			}
			if _, current := funcs[value]; current {
				continue
			}
			key, ok := funcvalKeyOf(value)
			if !ok {
				continue
			}
			meta, ok := c.interp.funcMeta[key]
			if ok && meta.frame != nil && meta.frame.root == c.oldRoot && meta.retention != funcMetaOpaque {
				delete(c.interp.funcMeta, key)
			}
		}
	}
	for _, channel := range c.interp.ownedChannels {
		if channel.hostVisible {
			continue
		}
		for _, send := range channel.sends {
			if send.state == ownedChannelSendTerminal || send.pendingRoot != c.newRoot || !send.pending.IsValid() {
				continue
			}
			objects := map[*ownedObject]struct{}{}
			funcs := map[reflect.Value]struct{}{}
			c.interp.collectOwnedChannelGraphLocked(send.pending, objects, funcs, map[*ownedObject]struct{}{}, map[reflect.Value]struct{}{}, false)
			// The cloned wrapper can be present in the pending aggregate before
			// its canonical metadata is discoverable through graph traversal.
			// Carry the exact old->new wrapper mapping produced by cloneFunc.
			for value := range send.pendingFuncs {
				oldKey, ok := funcvalKeyOf(value)
				if !ok {
					continue
				}
				clone, ok := c.funcMemo[oldKey]
				if !ok {
					continue
				}
				if _, valid := funcvalKeyOf(clone); valid {
					funcs[reflect.ValueOf(clone.Interface())] = struct{}{}
				}
			}
			for value := range funcs {
				key, ok := funcvalKeyOf(value)
				if !ok {
					continue
				}
				meta, exists := c.interp.funcMeta[key]
				if !exists {
					continue
				}
				meta.frame = c.newRoot
				c.interp.funcMeta[key] = meta
			}
			// The previous pending graph belongs to an abandoned generation. Its
			// cloned replacements are already installed above, so obsolete keys
			// can be deleted rather than re-rooted visibly.
			c.interp.replaceOwnedChannelSendGraphLocked(send, true, objects, funcs, true)
		}
	}
	c.oldRoot.funcMeta = c.oldRoot.funcMeta[:0]
	for key, meta := range c.interp.funcMeta {
		if meta.frame == c.oldRoot {
			c.oldRoot.funcMeta = append(c.oldRoot.funcMeta, key)
		}
	}
}

func (c *detachedRootCloner) commitTargeted(owner *frame) {
	if owner == nil {
		owner = c.newRoot
	}
	// Derive the clone activations before funcMu: each derivation allocates,
	// and the clones are alive here on the calling stack.
	cloneActivations := make([]directFuncActivation, 0, len(c.funcClones))
	for _, clone := range c.funcClones {
		cloneActivations = append(cloneActivations, newDirectFuncActivation(clone.value))
	}
	c.interp.funcMu.Lock()
	defer c.interp.funcMu.Unlock()
	for key, obj := range c.newObjects {
		if existing := c.interp.ownedObjects[key]; existing != nil {
			continue
		}
		if obj.hostShared {
			// Clones are struct copies; keep the hostShared estimate exact
			// for objects that inherit the flag from their source.
			c.interp.hostSharedEstimate++
		}
		obj.owner = owner
		c.interp.ownedObjects[key] = obj
		c.interp.armOwnedGCLocked()
		if owner.ownedObjects == nil {
			owner.ownedObjects = map[*ownedObject]struct{}{}
		}
		owner.ownedObjects[obj] = struct{}{}
	}
	for i, clone := range c.funcClones {
		newRef, ok := funcvalRefOf(clone.value)
		if !ok {
			continue
		}
		meta := clone.meta
		meta.frame = owner
		meta.retention = funcMetaVisible
		if meta.group == nil {
			meta.group = &funcMetaGroup{root: owner.root, version: 1}
		} else {
			meta.group.root = owner.root
		}
		c.interp.insertFuncMetaEntryLocked(newRef, meta, owner)
		cloneActivation := cloneActivations[i]
		c.interp.directFuncs[directFuncActivationKey{source: clone.oldKey, root: owner.root}] = cloneActivation
		c.interp.directFuncs[directFuncActivationKey{source: newRef.key, root: owner.root}] = cloneActivation
		for lineageKey, active := range c.directLineage {
			if active.key == clone.oldKey {
				c.interp.directFuncs[directFuncActivationKey{source: lineageKey.source, root: owner.root}] = cloneActivation
			}
		}
		c.interp.armOwnedGCLocked()
	}
}

func (interp *Interpreter) activateDirectFuncFromExec(owner *frame, value reflect.Value, cancel <-chan struct{}) reflect.Value {
	if owner == nil || owner.root == nil {
		return value
	}
	_, initialMeta, interpreted := interp.lookupInterpretedFunc(value)
	if !interpreted || initialMeta.rebind == nil {
		return value
	}
	activated := value
	interp.withFuncSweepWriteFromExec(func() {
		source, meta, ok := interp.lookupInterpretedFunc(value)
		if !ok || meta.rebind == nil {
			return
		}
		oldRoot := meta.frame
		if meta.group != nil && meta.group.root != nil {
			oldRoot = meta.group.root
		} else if oldRoot != nil {
			oldRoot = oldRoot.root
		}
		if oldRoot == nil || oldRoot == owner.root {
			return
		}
		cacheKey := directFuncActivationKey{source: source, root: owner.root}
		interp.funcMu.RLock()
		cached, cachedOK := interp.directFuncs[cacheKey]
		interp.funcMu.RUnlock()
		if cachedOK {
			activated = cached.value
			return
		}
		cloner := newDetachedRootCloner(interp, oldRoot, owner.root, cancel)
		clone := cloner.cloneFunc(value)
		if !clone.IsValid() || sameFuncvalKey(clone, value) {
			return
		}
		if clone.Type() != value.Type() && clone.Type().ConvertibleTo(value.Type()) {
			clone = clone.Convert(value.Type())
		}
		cloner.commitTargeted(owner.root)
		// Derived before funcMu: the clone is alive here and the derivation
		// allocates.
		activation := newDirectFuncActivation(clone)
		interp.funcMu.Lock()
		interp.directFuncs[cacheKey] = activation
		interp.armOwnedGCLocked()
		interp.funcMu.Unlock()
		activated = clone
	})
	return activated
}
