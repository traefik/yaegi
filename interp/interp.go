package interp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/build"
	"go/scanner"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Interpreter node structure for AST and CFG.
type node struct {
	debug      *nodeDebugData             // debug info
	child      []*node                    // child subtrees (AST)
	anc        *node                      // ancestor (AST)
	param      []*itype                   // generic parameter nodes (AST)
	start      *node                      // entry point in subtree (CFG)
	tnext      *node                      // true branch successor (CFG)
	fnext      *node                      // false branch successor (CFG)
	interp     *Interpreter               // interpreter context
	index      int64                      // node index (dot display)
	findex     int                        // index of value in frame or frame size (func def, type def)
	level      int                        // number of frame indirections to access value
	nleft      int                        // number of children in left part (assign) or indicates preceding type (compositeLit)
	nright     int                        // number of children in right part (assign)
	kind       nkind                      // kind of node
	pos        token.Pos                  // position in source code, relative to fset
	sym        *symbol                    // associated symbol
	typ        *itype                     // type of value in frame, or nil
	recv       *receiver                  // method receiver node for call, or nil
	types      []reflect.Type             // frame types, used by function literals only
	scope      *scope                     // frame scope
	action     action                     // action
	exec       bltn                       // generated function to execute
	gen        bltnGenerator              // generator function to produce above bltn
	val        interface{}                // static generic value (CFG execution)
	rval       reflect.Value              // reflection value to let runtime access interpreter (CFG)
	ident      string                     // set if node is a var or func
	redeclared bool                       // set if node is a redeclared variable (CFG)
	meta       interface{}                // meta stores meta information between gta runs, like errors
	assignTmp  []int                      // frame indexes that snapshot multi-assignment operands
	assignSrc  *node                      // source operand of a synthetic multi-assignment snapshot node
	callFunc   *node                      // synthetic node that snapshots the called function value
	callSnaps  []*node                    // synthetic nodes that snapshot call arguments left-to-right
	callValue  func(*frame) reflect.Value // runtime value copied by a synthetic call snapshot
}

func (n *node) shouldBreak() bool {
	if n == nil || n.debug == nil {
		return false
	}

	if n.debug.breakOnLine || n.debug.breakOnCall {
		return true
	}

	return false
}

func (n *node) setProgram(p *Program) {
	if n.debug == nil {
		n.debug = new(nodeDebugData)
	}
	n.debug.program = p
}

func (n *node) setBreakOnCall(v bool) {
	if n.debug == nil {
		if !v {
			return
		}
		n.debug = new(nodeDebugData)
	}
	n.debug.breakOnCall = v
}

func (n *node) setBreakOnLine(v bool) {
	if n.debug == nil {
		if !v {
			return
		}
		n.debug = new(nodeDebugData)
	}
	n.debug.breakOnLine = v
}

// receiver stores method receiver object access path.
type receiver struct {
	node  *node         // receiver value for alias and struct types
	val   reflect.Value // receiver value for interface type and value type
	index []int         // path in receiver value for interface or value type
}

// frame contains values for the current execution level (a function context).
type frame struct {
	// id is an atomic counter used for cancellation, only accessed
	// via newFrame/runid/setrunid/clone.
	// Located at start of struct to ensure proper alignment.
	id uint64

	debug  *frameDebugData
	interp *Interpreter

	root *frame          // global space
	anc  *frame          // ancestor frame (caller space)
	data []reflect.Value // values

	mutex          sync.RWMutex
	callArgs       map[*node]reflect.Value    // transient left-to-right call argument snapshots
	deferred       []deferredCall             // defer stack
	recovered      interface{}                // to handle panic recover
	done           reflect.SelectCase         // for cancellation of channel operations
	cancel         <-chan struct{}            // cancellation owner for this execution; the shared root's field is rewritten by prepareExecutionFrame under mutex, so shared-root reads must take the read lock (see canceled). Every other frame's cancel is copied once by newFrame/clone under the parent's lock and frozen, so unlocked reads of f.cancel on non-root frames are race-free.
	fenceExclusive atomic.Bool                // funcSweep fence mode captured at step acquisition
	funcMeta       []uintptr                  // interpreted wrappers registered by this activation, as funcval keys
	funcEscape     funcMetaRetention          // how wrappers crossed an opaque activation boundary
	funcState      funcFrameState             // lifecycle of metadata owned by this activation
	cloneOf        *frame                     // live activation whose lexical slots this clone shares
	funcCarrier    uintptr                    // funcval key of the wrapper whose closure runs this lexical clone; a key, not a value, so the clone does not pin the wrapper against the registry's weak eviction
	funcGroup      *funcMetaGroup             // wrappers created by this activation
	funcSlots      []funcSlotRestore          // literal slots to restore when the last activation exits
	ownedObjects   map[*ownedObject]struct{}  // reference-backed allocations created by this activation
	ownedChannels  map[*ownedChannel]struct{} // channels created by this activation
}

// funcSlotRestore reverts a function literal's value slot to the value it held
// before the literal last executed. Without it, the wrapper registered in the
// metadata registry would stay pinned by its own entry (the entry's invoker
// keeps the creating frame alive, and the frame's slot keeps the wrapper), so
// a dropped wrapper could never be observed unreachable by its finalizer.
type funcSlotRestore struct {
	owner *frame
	index int
	value reflect.Value
}

type deferredCall struct {
	values    []reflect.Value
	exclusive bool
}

// interpretedFuncInvoker executes a reflected interpreted function against an
// explicit global root and cancellation owner. Keeping this metadata separate
// from the reflect wrapper lets deferred cleanup and host-retained callbacks
// avoid consulting mutable "latest Eval" state.
type interpretedFuncInvoker func([]reflect.Value, *frame, <-chan struct{}) []reflect.Value

type interpretedFuncBuild struct {
	value    reflect.Value
	invoke   interpretedFuncInvoker
	rebind   interpretedFuncRebinder
	captures []funcMetaCapture
}

// interpretedFuncRebinder rebuilds a wrapper against a detached root and its
// cloned lexical carrier. The returned wrapper must not retain frames from the
// canceled root.
type interpretedFuncRebinder func(*detachedRootCloner) interpretedFuncBuild

// boundWrapperKey identifies a memoized host-bound wrapper: the funcval key
// of the interpreted value being bound, the activation it is bound to, and
// the signature the native callee expects. Guarded by funcMu alongside the
// group's bound map.
type boundWrapperKey struct {
	target       uintptr
	root         *frame
	cancel       <-chan struct{}
	typ          reflect.Type
	hostBoundary bool
}

// interpretedFuncMeta is the metadata registered for one interpreted wrapper.
// The registry is keyed by the wrapper's funcval address (see funcvalKeyOf),
// so the entry must not retain the wrapper itself: the registered signature
// is kept in typ for the convertible-type fallback, and the generation guard
// makes a stale finalizer harmless if a new wrapper later reuses the address.
type interpretedFuncMeta struct {
	invoke     interpretedFuncInvoker
	rebind     interpretedFuncRebinder
	frame      *frame
	retention  funcMetaRetention
	group      *funcMetaGroup
	captures   []funcMetaCapture
	typ        reflect.Type
	generation uint64
}

type directFuncActivationKey struct {
	source uintptr
	root   *frame
}

// directFuncActivation is a cached direct activation: the cloned wrapper plus
// its funcval key, stored so eviction paths can match the value endpoint
// without deriving the key (an allocation) under funcMu.
type directFuncActivation struct {
	value reflect.Value
	key   uintptr
}

func (f *frame) setCallArg(n *node, value reflect.Value) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.callArgs == nil {
		f.callArgs = map[*node]reflect.Value{}
	}
	f.callArgs[n] = value
}

func (f *frame) callArg(n *node) reflect.Value {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	return f.callArgs[n]
}

func (f *frame) clearCallArgs(call *node) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if call.callFunc != nil {
		delete(f.callArgs, call.callFunc)
	}
	for _, snapshot := range call.callSnaps {
		delete(f.callArgs, snapshot)
	}
	if len(f.callArgs) == 0 {
		f.callArgs = nil
	}
}

func canonicalFuncValue(v reflect.Value) (reflect.Value, bool) {
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() || !v.CanInterface() {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(v.Interface()), true
}

func (interp *Interpreter) registerInterpretedFuncWithRebinder(v reflect.Value, invoke interpretedFuncInvoker, rebind interpretedFuncRebinder, f *frame, captures []funcMetaCapture) {
	ref, ok := funcvalRefOf(v)
	if !ok {
		return
	}
	interp.funcMu.Lock()
	if _, exists := interp.funcMeta[ref.key]; !exists {
		owner := f
		if owner != nil && owner != owner.root && owner.funcState != funcFrameActive {
			owner = owner.root
		}
		var group *funcMetaGroup
		if owner != nil {
			if owner.funcGroup == nil {
				owner.funcGroup = &funcMetaGroup{root: owner.root}
			}
			group = owner.funcGroup
			for _, capture := range captures {
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
			group.version++
		}
		interp.insertFuncMetaEntryLocked(ref, interpretedFuncMeta{
			invoke: invoke, rebind: rebind, frame: owner, group: group,
			captures: append([]funcMetaCapture(nil), captures...), typ: v.Type(),
		}, owner)
	}
	interp.funcMu.Unlock()
}

func (interp *Interpreter) registerInterpretedFuncAlias(v reflect.Value, meta interpretedFuncMeta, owner *frame) {
	ref, ok := funcvalRefOf(v)
	if !ok || owner == nil {
		return
	}
	interp.funcMu.Lock()
	defer interp.funcMu.Unlock()
	if _, exists := interp.funcMeta[ref.key]; exists {
		return
	}
	// The bound wrapper itself keeps the exact invocation owner alive. Metadata
	// only makes an alias discoverable if it re-enters interpreted state, so let
	// ordinary root reachability discard aliases retained solely by native code.
	meta.frame = owner.root
	meta.retention = funcMetaVisible
	if meta.group != nil {
		meta.group.root = owner.root
	}
	interp.insertFuncMetaEntryLocked(ref, meta, owner.root)
}

func (interp *Interpreter) interpretedFunc(v reflect.Value) (interpretedFuncInvoker, bool) {
	_, meta, ok := interp.lookupInterpretedFunc(v)
	return meta.invoke, ok
}

// lookupInterpretedFunc resolves the metadata registered for the wrapper v.
// The registry is keyed by v's funcval address, so a direct hit covers both
// the exact registered type and any convertible named type over the same
// funcval: a funcval is always created under one concrete signature, and func
// conversions between identical underlying types preserve the funcval. The
// registered type is still checked (equal or convertible) as a safety net,
// mirroring the value-keyed registry's canonical-equality plus
// convertible-fallback semantics at the cost of one type comparison.
func (interp *Interpreter) lookupInterpretedFunc(v reflect.Value) (uintptr, interpretedFuncMeta, bool) {
	key, ok := funcvalKeyOf(v)
	if !ok {
		return 0, interpretedFuncMeta{}, false
	}
	interp.funcMu.RLock()
	meta, ok := interp.funcMeta[key]
	if ok && meta.typ != nil && v.Type() != meta.typ && !v.Type().ConvertibleTo(meta.typ) {
		ok = false
	}
	interp.funcMu.RUnlock()
	return key, meta, ok
}

func newFrame(anc *frame, length int, id uint64) *frame {
	f := &frame{
		anc:  anc,
		data: make([]reflect.Value, length),
		id:   id,
	}
	if anc == nil {
		f.root = f
	} else {
		f.interp = anc.interp
		// The owner of a shared root can be rewritten by a later evaluation
		// (prepareExecutionFrame); copy it under the root's lock so a frame
		// created by an orphaned goroutine (a deferred call unwinding after
		// cancellation) cannot race that rewrite.
		anc.mutex.RLock()
		f.done = anc.done
		f.cancel = anc.cancel
		anc.mutex.RUnlock()
		f.root = anc.root
	}
	return f
}

func (f *frame) runid() uint64      { return atomic.LoadUint64(&f.id) }
func (f *frame) setrunid(id uint64) { atomic.StoreUint64(&f.id, id) }
func (f *frame) canceled() bool {
	// The cancellation owner of a shared root is rewritten by
	// prepareExecutionFrame under f.mutex, while interpreted goroutines that
	// outlive their evaluation (a `go` statement, a canceled worker draining
	// its deferred calls) keep reading it without any other synchronization.
	// Take the read lock so those reads cannot race the rewrite.
	f.mutex.RLock()
	cancel := f.cancel
	f.mutex.RUnlock()
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

func (f *frame) clone() *frame {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	nf := &frame{
		anc:       f.anc,
		root:      f.root,
		interp:    f.interp,
		deferred:  f.deferred,
		recovered: f.recovered,
		id:        f.runid(),
		done:      f.done,
		cancel:    f.cancel,
		debug:     f.debug,
		cloneOf:   f,
	}
	nf.data = make([]reflect.Value, len(f.data))
	copy(nf.data, f.data)
	return nf
}

// cloneDetached creates durable storage for a later execution after the
// current frame owner was canceled. Unlike clone, which preserves shared slots
// for lexical closures, this clone must not allow the abandoned activation to
// write through into the later root.
func (f *frame) cloneDetached(cancel <-chan struct{}) *frame {
	// Locking note: this function holds f.mutex.RLock while the cloner takes
	// funcMu.RLock, the reverse of snapshotFuncMetaCapture's funcMu.RLock ->
	// capture.frame.mutex.RLock order. Both nestings are read-held today and
	// no funcMu writer acquires a frame mutex (or vice versa), so no cycle is
	// reachable; preserve that property when touching either lock section.
	f.mutex.RLock()
	nf := &frame{
		anc:    f.anc,
		interp: f.interp,
		id:     f.runid(),
		debug:  f.debug,
		cancel: cancel,
		done:   reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(cancel)},
	}
	var cloner *detachedRootCloner
	if f.root == f && f.interp != nil {
		cloner = newDetachedRootCloner(f.interp, f, nf, cancel)
	}
	nf.data = make([]reflect.Value, len(f.data))
	sharedCells := make([]bool, len(f.data))
	for i, v := range f.data {
		if !v.IsValid() {
			continue
		}
		if cloner != nil && cloner.cellHostShared(v) {
			nf.data[i] = v
			sharedCells[i] = true
			continue
		}
		nf.data[i] = reflect.New(v.Type()).Elem()
	}
	if f.root == f {
		nf.root = nf
	} else {
		nf.root = f.root
	}
	data := append([]reflect.Value(nil), f.data...)
	f.mutex.RUnlock()

	if f.root != f || f.interp == nil {
		for i, value := range data {
			if value.IsValid() {
				nf.data[i].Set(value)
			}
		}
		return nf
	}

	for i, value := range data {
		if value.IsValid() {
			cloner.seedCell(value, nf.data[i])
		}
	}
	// Pending channel and panic values are the authoritative copies of their
	// aggregate graphs. Clone them with function rehoming enabled before root
	// globals can memoize the same aggregate with shallow function values.
	cloner.snapshotPendingEscapes()
	for i, value := range data {
		if value.IsValid() && !sharedCells[i] {
			nf.data[i].Set(cloner.cloneValue(value, true))
		}
	}
	cloner.cloneDirectFuncLineage()
	cloner.commit()
	return nf
}

// Exports stores the map of binary packages per package path.
// The package path is the path joined from the import path and the package name
// as specified in source files by the "package" statement.
type Exports map[string]map[string]reflect.Value

// imports stores the map of source packages per package path.
type imports map[string]map[string]*symbol

// opt stores interpreter options.
type opt struct {
	// dotCmd is the command to process the dot graph produced when astDot and/or
	// cfgDot is enabled. It defaults to 'dot -Tdot -o <filename>.dot'.
	dotCmd       string
	context      build.Context     // build context: GOPATH, build constraints
	stdin        io.Reader         // standard input
	stdout       io.Writer         // standard output
	stderr       io.Writer         // standard error
	args         []string          // cmdline args
	env          map[string]string // environment of interpreter, entries in form of "key=value"
	filesystem   fs.FS             // filesystem containing sources
	astDot       bool              // display AST graph (debug)
	cfgDot       bool              // display CFG graph (debug)
	noRun        bool              // compile, but do not run
	fastChan     bool              // disable cancellable chan operations
	specialStdio bool              // allows os.Stdin, os.Stdout, os.Stderr to not be file descriptors
	unrestricted bool              // allow use of non-sandboxed symbols
}

// Interpreter contains global resources and state.
type Interpreter struct {
	// id is an atomic counter used for run cancellation,
	// only accessed via runid/stop
	// Located at start of struct to ensure proper alignment on 32-bit
	// architectures.
	id uint64

	// nindex is a node number incremented for each new node.
	// It is used for debug (AST and CFG graphs). As it is atomically
	// incremented, keep it aligned on 64 bits boundary.
	nindex int64

	name string // name of the input source file (or main)

	opt                                         // user settable options
	cancelChan bool                             // enables cancellable chan operations
	fset       *token.FileSet                   // fileset to locate node in source code
	binPkg     Exports                          // binary packages used in interpreter, indexed by path
	rdir       map[string]bool                  // for src import cycle detection
	mapTypes   map[reflect.Value][]reflect.Type // special interfaces mapping for wrappers

	// compileMu serializes compiler mutations with execution-frame setup. A
	// canceled Eval may leave a worker blocked in native code while a later Eval
	// proceeds, but its compiler/setup phase must never race a new GTA pass.
	compileMu sync.Mutex
	// compileCancel is accessed only while compileMu is held. It propagates the
	// exact evaluation owner into source-package imports compiled recursively.
	compileCancel <-chan struct{}
	// executionGate serializes live executions while allowing a canceled API
	// call to relinquish ownership before a native call returns. A later run can
	// then detach the canceled root without sharing its mutable cancel owner.
	executionGate chan struct{}
	// gateHolderGoid is the goroutine id of the goroutine that currently
	// holds executionGate (0 when free). The holder may always re-enter its
	// own interpreter: a nested Eval from a host callback of an inline
	// execution — including a source-package init retry whose ambient
	// cancellation owner already fired — must not block on the gate its own
	// goroutine holds.
	gateHolderGoid atomic.Int64
	// zombieBarrier serializes the deferred phase of a canceled (zombie)
	// worker against the whole execution of a later evaluation. A canceled
	// worker's deferred calls still run (they drive ownership publication),
	// but the evaluation they belonged to has already returned, so the gate
	// no longer excludes them; the barrier closes that window. Active runs
	// hold it for their entire execution via their run token; a zombie's
	// deferred phase no longer matches the current token and must take it.
	// zombieDefers counts canceled workers currently unwinding their
	// deferred calls. While it is positive, every interpreted execution step
	// holds the funcSweep fence exclusively: a zombie's deferred writes must
	// not overlap a later evaluation's steps on shared containers. Native
	// stretches still release the fence, so a zombie defer blocked in a host
	// call never blocks later evaluations.
	zombieDefers       atomic.Int64
	funcSweepExclusive atomic.Int64 // depth of exclusive funcSweepMu holders (zombie deferred steps)
	// ownedGCPending requests one incremental ownership sweep once the
	// ownership registries cross ownedGCRegistryCap entries; arming happens
	// under funcMu, consumption under the exclusive funcSweepMu fence.
	ownedGCPending atomic.Bool
	// ownedGCInFlight makes sweep consumption one-shot: at most one goroutine
	// runs the sweep body at a time, and a fence TryLock loss leaves the
	// request pending for the next execution step.
	ownedGCInFlight atomic.Bool
	// frameDrains counts frames currently unwinding deferred calls inside the
	// runCfg exit path. Incremented under the exclusive funcSweepMu fence so a
	// sweep holding the fence reads it exactly: a draining frame's remaining
	// deferred call values are invisible to the sweep's root set, so the
	// incremental sweep must not run concurrently with any drain.
	frameDrains atomic.Int64
	mutex       sync.RWMutex
	funcMu      sync.RWMutex
	funcSweepMu sync.RWMutex
	// funcMeta is keyed by the funcval address of each registered interpreted
	// wrapper (see funcvalKeyOf). The key is a plain integer, so the registry
	// does not retain the wrapper: when the host drops the last reference, a
	// finalizer armed at insertion deletes the entry (guarded by the entry's
	// generation against funcval address reuse).
	funcMeta      map[uintptr]interpretedFuncMeta
	directFuncs   map[directFuncActivationKey]directFuncActivation
	ownedObjects  map[objectKey]*ownedObject
	ownedChannels map[uintptr]*ownedChannel
	panicTokens   map[*ownedPanicToken]struct{}
	// hostSharedEstimate counts owned objects currently flagged hostShared.
	// It is maintained exactly under funcMu so the per-write ownership scans
	// can return in constant time while no object is host-shared.
	hostSharedEstimate int
	// ownedRegistrations amortizes sweep arming: one request is raised at most
	// every ownedGCAmortizeRegistrations registry inserts past the cap. Guarded
	// by funcMu (all arming sites already hold it).
	ownedRegistrations int
	// activeFrames refcounts frames with a live runCfg activation, guarded by
	// funcMu. Refcounted because a root frame can re-enter runCfg through a
	// reentrant Eval while an outer activation is still on the stack. The
	// incremental ownership sweep walks every entry's whole ancestor chain as
	// its root set; frames of forever-blocked goroutines pin one entry each
	// (Go-like goroutine retention).
	activeFrames    map[*frame]int
	frame           *frame            // program data storage during execution
	universe        *scope            // interpreter global level scope
	scopes          map[string]*scope // package level scopes, indexed by import path
	srcPkg          imports           // source packages used in interpreter, indexed by path
	publishedSrcPkg imports           // immutable symbol snapshots exposed to host readers
	pkgNames        map[string]string // package names, indexed by import path
	srcPkgInit      map[string]*sourcePackageInit
	srcPkgBuild     map[string]*sourcePackageBuild
	// globalVarIndexes is an immutable-at-runtime snapshot of durable package
	// variable slots. Compiler passes refresh it before releasing compileMu so
	// canceled-worker cleanup never walks symbol maps while a later Eval mutates
	// them.
	globalVarIndexes map[int]struct{}
	done             <-chan struct{} // cancellation owner used by channel operations
	roots            []*node
	generic          map[string]*node

	hooks *hooks // symbol hooks

	debugger *Debugger
}

const (
	mainID     = "main"
	initID     = "init"
	selfPrefix = "github.com/traefik/yaegi"
	selfPath   = selfPrefix + "/interp/interp"
	// DefaultSourceName is the name used by default when the name of the input
	// source file has not been specified for an Eval.
	// TODO(mpl): something even more special as a name?
	DefaultSourceName = "_.go"

	// ownedGCRegistryCap is the ownership registry size (owned objects plus
	// owned channels) above which incremental sweeps are armed. Bounded
	// retention per interpreter is the goal; workloads whose live set exceeds
	// the cap pay one O(live+registry) sweep per amortization window.
	ownedGCRegistryCap = 1 << 16
	// ownedGCAmortizeRegistrations is the minimum number of registry inserts
	// between two armed sweeps, so the O(registry) sweep cost stays amortized.
	ownedGCAmortizeRegistrations = 1 << 12

	// Test is the value to pass to EvalPath to activate evaluation of test functions.
	Test = false
	// NoTest is the value to pass to EvalPath to skip evaluation of test functions.
	NoTest = true
)

// Self points to the current interpreter if accessed from within itself, or is nil.
var Self *Interpreter

// Symbols exposes interpreter values.
var Symbols = Exports{
	selfPath: map[string]reflect.Value{
		"New": reflect.ValueOf(New),

		"Interpreter": reflect.ValueOf((*Interpreter)(nil)),
		"Options":     reflect.ValueOf((*Options)(nil)),
		"Panic":       reflect.ValueOf((*Panic)(nil)),
	},
}

func init() { Symbols[selfPath]["Symbols"] = reflect.ValueOf(Symbols) }

// _error is a wrapper of error interface type.
type _error struct {
	IValue interface{}
	WError func() string
}

func (w _error) Error() string { return w.WError() }

// Panic is an error recovered from a panic call in interpreted code.
type Panic struct {
	// Value is the recovered value of a call to panic.
	Value interface{}

	// Callers is the call stack obtained from the recover call.
	// It may be used as the parameter to runtime.CallersFrames.
	Callers []uintptr

	// Stack is the call stack buffer for debug.
	Stack []byte
}

// TODO: Capture interpreter stack frames also and remove
// fmt.Fprintln(n.interp.stderr, oNode.cfgErrorf("panic")) in runCfg.

func (e Panic) Error() string { return fmt.Sprint(e.Value) }

// Walk traverses AST n in depth first order, call cbin function
// at node entry and cbout function at node exit.
func (n *node) Walk(in func(n *node) bool, out func(n *node)) {
	if in != nil && !in(n) {
		return
	}
	for _, child := range n.child {
		child.Walk(in, out)
	}
	if out != nil {
		out(n)
	}
}

// Options are the interpreter options.
type Options struct {
	// GoPath sets GOPATH for the interpreter.
	GoPath string

	// BuildTags sets build constraints for the interpreter.
	BuildTags []string

	// Standard input, output and error streams.
	// They default to os.Stdin, os.Stdout and os.Stderr respectively.
	Stdin          io.Reader
	Stdout, Stderr io.Writer

	// Cmdline args, defaults to os.Args.
	Args []string

	// Environment of interpreter. Entries are in the form "key=values".
	Env []string

	// SourcecodeFilesystem is where the _sourcecode_ is loaded from and does
	// NOT affect the filesystem of scripts when they run.
	// It can be any fs.FS compliant filesystem (e.g. embed.FS, or fstest.MapFS for testing)
	// See example/fs/fs_test.go for an example.
	SourcecodeFilesystem fs.FS

	// Unrestricted allows to run non sandboxed stdlib symbols such as os/exec and environment
	Unrestricted bool
}

// New returns a new interpreter.
func New(options Options) *Interpreter {
	i := Interpreter{
		opt:              opt{context: build.Default, filesystem: &realFS{}, env: map[string]string{}},
		frame:            newFrame(nil, 0, 0),
		fset:             token.NewFileSet(),
		universe:         initUniverse(),
		scopes:           map[string]*scope{},
		binPkg:           Exports{"": map[string]reflect.Value{"_error": reflect.ValueOf((*_error)(nil))}},
		mapTypes:         map[reflect.Value][]reflect.Type{},
		srcPkg:           imports{},
		publishedSrcPkg:  imports{},
		pkgNames:         map[string]string{},
		srcPkgInit:       map[string]*sourcePackageInit{},
		srcPkgBuild:      map[string]*sourcePackageBuild{},
		globalVarIndexes: map[int]struct{}{},
		rdir:             map[string]bool{},
		executionGate:    make(chan struct{}, 1),
		hooks:            &hooks{},
		generic:          map[string]*node{},
		funcMeta:         map[uintptr]interpretedFuncMeta{},
		directFuncs:      map[directFuncActivationKey]directFuncActivation{},
		ownedObjects:     map[objectKey]*ownedObject{},
		ownedChannels:    map[uintptr]*ownedChannel{},
		panicTokens:      map[*ownedPanicToken]struct{}{},
		activeFrames:     map[*frame]int{},
	}
	i.executionGate <- struct{}{}
	i.frame.interp = &i

	if i.opt.stdin = options.Stdin; i.opt.stdin == nil {
		i.opt.stdin = os.Stdin
	}

	if i.opt.stdout = options.Stdout; i.opt.stdout == nil {
		i.opt.stdout = os.Stdout
	}

	if i.opt.stderr = options.Stderr; i.opt.stderr == nil {
		i.opt.stderr = os.Stderr
	}

	if i.opt.args = options.Args; i.opt.args == nil {
		i.opt.args = os.Args
	}

	// unrestricted allows to use non sandboxed stdlib symbols and env.
	if options.Unrestricted {
		i.opt.unrestricted = true
	} else {
		for _, e := range options.Env {
			a := strings.SplitN(e, "=", 2)
			if len(a) == 2 {
				i.opt.env[a[0]] = a[1]
			} else {
				i.opt.env[a[0]] = ""
			}
		}
	}

	if options.SourcecodeFilesystem != nil {
		i.opt.filesystem = options.SourcecodeFilesystem
	}

	i.opt.context.GOPATH = options.GoPath
	if len(options.BuildTags) > 0 {
		i.opt.context.BuildTags = options.BuildTags
	}

	// astDot activates AST graph display for the interpreter
	i.opt.astDot, _ = strconv.ParseBool(os.Getenv("YAEGI_AST_DOT"))

	// cfgDot activates CFG graph display for the interpreter
	i.opt.cfgDot, _ = strconv.ParseBool(os.Getenv("YAEGI_CFG_DOT"))

	// dotCmd defines how to process the dot code generated whenever astDot and/or
	// cfgDot is enabled. It defaults to 'dot -Tdot -o<filename>.dot' where filename
	// is context dependent.
	i.opt.dotCmd = os.Getenv("YAEGI_DOT_CMD")

	// noRun disables the execution (but not the compilation) in the interpreter
	i.opt.noRun, _ = strconv.ParseBool(os.Getenv("YAEGI_NO_RUN"))

	// fastChan disables the cancellable version of channel operations in evalWithContext
	i.opt.fastChan, _ = strconv.ParseBool(os.Getenv("YAEGI_FAST_CHAN"))
	i.cancelChan = !i.opt.fastChan

	// specialStdio allows to assign directly io.Writer and io.Reader to os.Stdxxx,
	// even if they are not file descriptors.
	i.opt.specialStdio, _ = strconv.ParseBool(os.Getenv("YAEGI_SPECIAL_STDIO"))

	return &i
}

const (
	bltnAlignof  = "unsafe.Alignof"
	bltnAppend   = "append"
	bltnCap      = "cap"
	bltnClose    = "close"
	bltnComplex  = "complex"
	bltnImag     = "imag"
	bltnCopy     = "copy"
	bltnDelete   = "delete"
	bltnLen      = "len"
	bltnMake     = "make"
	bltnMax      = "max"
	bltnMin      = "min"
	bltnNew      = "new"
	bltnOffsetof = "unsafe.Offsetof"
	bltnPanic    = "panic"
	bltnPrint    = "print"
	bltnPrintln  = "println"
	bltnReal     = "real"
	bltnRecover  = "recover"
	bltnSizeof   = "unsafe.Sizeof"

	bltnUnsafeSlice  = "unsafe.Slice"
	bltnUnsafeString = "unsafe.String"
)

func initUniverse() *scope {
	sc := &scope{global: true, sym: map[string]*symbol{
		// predefined Go types
		"any":         {kind: typeSym, typ: &itype{cat: interfaceT, str: "any"}},
		"bool":        {kind: typeSym, typ: &itype{cat: boolT, name: "bool", str: "bool"}},
		"byte":        {kind: typeSym, typ: &itype{cat: uint8T, name: "uint8", str: "uint8"}},
		"comparable":  {kind: typeSym, typ: &itype{cat: comparableT, name: "comparable", str: "comparable"}},
		"complex64":   {kind: typeSym, typ: &itype{cat: complex64T, name: "complex64", str: "complex64"}},
		"complex128":  {kind: typeSym, typ: &itype{cat: complex128T, name: "complex128", str: "complex128"}},
		"error":       {kind: typeSym, typ: &itype{cat: errorT, name: "error", str: "error"}},
		"float32":     {kind: typeSym, typ: &itype{cat: float32T, name: "float32", str: "float32"}},
		"float64":     {kind: typeSym, typ: &itype{cat: float64T, name: "float64", str: "float64"}},
		"int":         {kind: typeSym, typ: &itype{cat: intT, name: "int", str: "int"}},
		"int8":        {kind: typeSym, typ: &itype{cat: int8T, name: "int8", str: "int8"}},
		"int16":       {kind: typeSym, typ: &itype{cat: int16T, name: "int16", str: "int16"}},
		"int32":       {kind: typeSym, typ: &itype{cat: int32T, name: "int32", str: "int32"}},
		"int64":       {kind: typeSym, typ: &itype{cat: int64T, name: "int64", str: "int64"}},
		"interface{}": {kind: typeSym, typ: &itype{cat: interfaceT, str: "interface{}"}},
		"rune":        {kind: typeSym, typ: &itype{cat: int32T, name: "int32", str: "int32"}},
		"string":      {kind: typeSym, typ: &itype{cat: stringT, name: "string", str: "string"}},
		"uint":        {kind: typeSym, typ: &itype{cat: uintT, name: "uint", str: "uint"}},
		"uint8":       {kind: typeSym, typ: &itype{cat: uint8T, name: "uint8", str: "uint8"}},
		"uint16":      {kind: typeSym, typ: &itype{cat: uint16T, name: "uint16", str: "uint16"}},
		"uint32":      {kind: typeSym, typ: &itype{cat: uint32T, name: "uint32", str: "uint32"}},
		"uint64":      {kind: typeSym, typ: &itype{cat: uint64T, name: "uint64", str: "uint64"}},
		"uintptr":     {kind: typeSym, typ: &itype{cat: uintptrT, name: "uintptr", str: "uintptr"}},

		// predefined Go constants
		"false": {kind: constSym, typ: untypedBool(nil), rval: reflect.ValueOf(false)},
		"true":  {kind: constSym, typ: untypedBool(nil), rval: reflect.ValueOf(true)},
		"iota":  {kind: constSym, typ: untypedInt(nil)},

		// predefined Go zero value
		"nil": {typ: &itype{cat: nilT, untyped: true, str: "nil"}},

		// predefined Go builtins
		bltnAppend:  {kind: bltnSym, builtin: _append},
		bltnCap:     {kind: bltnSym, builtin: _cap},
		bltnClose:   {kind: bltnSym, builtin: _close},
		bltnComplex: {kind: bltnSym, builtin: _complex},
		bltnImag:    {kind: bltnSym, builtin: _imag},
		bltnCopy:    {kind: bltnSym, builtin: _copy},
		bltnDelete:  {kind: bltnSym, builtin: _delete},
		bltnLen:     {kind: bltnSym, builtin: _len},
		bltnMake:    {kind: bltnSym, builtin: _make},
		bltnMax:     {kind: bltnSym, builtin: _max},
		bltnMin:     {kind: bltnSym, builtin: _min},
		bltnNew:     {kind: bltnSym, builtin: _new},
		bltnPanic:   {kind: bltnSym, builtin: _panic},
		bltnPrint:   {kind: bltnSym, builtin: _print},
		bltnPrintln: {kind: bltnSym, builtin: _println},
		bltnReal:    {kind: bltnSym, builtin: _real},
		bltnRecover: {kind: bltnSym, builtin: _recover},
	}}
	return sc
}

// resizeFrameTo grows the exact frame selected for an execution. Callers that
// have captured a root must not re-read interp.frame during preparation.
func (interp *Interpreter) resizeFrameTo(f *frame) {
	l := len(interp.universe.types)
	b := len(f.data)
	// A retry after a failed source-package init rewinds the type allocator
	// and recompiles from scratch, so it can reallocate an existing index
	// with a different type than the cell left behind by the canceled
	// attempt. Re-align drifted cells to the allocator before execution;
	// cells whose type matches keep their value, which preserves globals.
	for i := 0; i < b && i < l; i++ {
		t := interp.universe.types[i]
		if f.data[i].IsValid() && f.data[i].Type() == t {
			continue
		}
		if t != nil {
			f.data[i] = reflect.New(t).Elem()
		}
	}
	if l-b <= 0 {
		return
	}
	data := make([]reflect.Value, l)
	copy(data, f.data)
	for j, t := range interp.universe.types[b:] {
		data[b+j] = reflect.New(t).Elem()
	}
	f.data = data
}

func (interp *Interpreter) acquireExecution() func() {
	select {
	case <-interp.executionGate:
		id := currentGoroutineID()
		interp.gateHolderGoid.Store(int64(id))
		return func() {
			interp.gateHolderGoid.CompareAndSwap(int64(id), 0)
			interp.executionGate <- struct{}{}
		}
	default:
		if executionMayReenter(interp) {
			// Writers, Stringers, and conversion hooks may synchronously call
			// back into the same interpreter. This goroutine either holds the
			// gate itself or runs a live (not canceled) execution (it carries
			// the reentrancy token), so it can inherit the active root and
			// cancel owner. A canceled worker draining deferred calls after
			// the API call returned runs outside the gate and must not
			// bypass: its prepareExecutionFrame would rewrite the live
			// root's owner concurrently with the gated execution now holding
			// the gate.
			return func() {}
		}
		<-interp.executionGate
		id := currentGoroutineID()
		interp.gateHolderGoid.Store(int64(id))
		return func() {
			interp.gateHolderGoid.CompareAndSwap(int64(id), 0)
			interp.executionGate <- struct{}{}
		}
	}
}

// executionMayReenter decides the gate bypass for the calling goroutine. It
// holds when the goroutine itself holds the execution gate, or when it runs
// a token whose execution owner is still live. The owner is read at decision
// time, not sampled once when the execution unwinds: a cancellation that
// lands while the goroutine is parked inside a deferred host call must
// remove the bypass for the rest of the drain, so a sampled flag would be a
// TOCTOU hole.
func executionMayReenter(interp *Interpreter) bool {
	id := currentGoroutineID()
	if id != 0 && interp.gateHolderGoid.Load() == int64(id) {
		return true
	}
	token, ok := innermostExecutionToken(interp)
	if !ok {
		return false
	}
	return token.owner == nil || !ownerCanceled(token.owner)
}

func ownerCanceled(cancel <-chan struct{}) bool {
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

func (interp *Interpreter) acquireExecutionWithContext(ctx context.Context) (func(), error) {
	// Mirror the plain gate: a host callback on the paused execution (a
	// writer, Stringer, or conversion hook calling back with a context)
	// must not wait for the gate its own outer evaluation holds. Canceled
	// (zombie) workers do not bypass — see executionMayReenter.
	if executionMayReenter(interp) {
		return func() {}, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-interp.executionGate:
		id := currentGoroutineID()
		interp.gateHolderGoid.Store(int64(id))
		return func() {
			interp.gateHolderGoid.CompareAndSwap(int64(id), 0)
			interp.executionGate <- struct{}{}
		}, nil
	}
}

// executionToken records that a goroutine is running one interpreter
// execution. The reentrancy bypass for the execution gate is scoped to the
// goroutine, not probed from the native stack: any goroutine running
// interpreted code has runCfg on its stack, including goroutines spawned by
// an interpreted `go` statement, so a stack probe misreads an Eval from such
// a goroutine as reentrant and lets it run concurrently with the gated
// execution. With an explicit token, a host callback running synchronously
// on the execution's goroutine (a writer, Stringer, or conversion hook) is
// recognized at any native depth, while an Eval from a `go`-statement
// goroutine waits for the gate like any unrelated goroutine.
type executionToken struct {
	interp *Interpreter
	depth  int
	// owner is the execution's cancellation channel, read live by
	// executionMayReenter: once the owner fires, the goroutine is unwinding
	// outside the gate and must stop bypassing it.
	owner <-chan struct{}
}

type executionTokenStack struct {
	stack []*executionToken
}

var executionTokens sync.Map // goroutine id (int) -> *executionTokenStack

// currentGoroutineID extracts the running goroutine's id from its own
// stack dump header ("goroutine N [running]:"). Only execution entry points
// and the gate's contended path call this, never per-step code.
func currentGoroutineID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := buf[:n]
	const prefix = "goroutine "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return 0
	}
	line = line[len(prefix):]
	i := bytes.IndexByte(line, ' ')
	if i < 0 {
		return 0
	}
	id, err := strconv.Atoi(string(line[:i]))
	if err != nil {
		return 0
	}
	return id
}

// acquireExecutionToken marks the calling goroutine as running an execution
// of interp. Nested synchronous re-entry of the same interpreter increments
// the depth; a call crossing to another interpreter pushes a separate token
// so the outer execution's bypass survives the inner release. A goroutine
// never pops more than it pushed: every acquisition is released by a defer
// in the same function of the same goroutine.
func acquireExecutionToken(interp *Interpreter, owner <-chan struct{}) {
	id := currentGoroutineID()
	if id == 0 {
		return
	}
	entry, _ := executionTokens.LoadOrStore(id, &executionTokenStack{})
	stack := entry.(*executionTokenStack)
	if n := len(stack.stack); n > 0 && stack.stack[n-1].interp == interp {
		stack.stack[n-1].depth++
		return
	}
	stack.stack = append(stack.stack, &executionToken{interp: interp, depth: 1, owner: owner})
}

func releaseExecutionToken(interp *Interpreter) {
	id := currentGoroutineID()
	if id == 0 {
		return
	}
	entry, ok := executionTokens.Load(id)
	if !ok {
		return
	}
	stack := entry.(*executionTokenStack)
	if n := len(stack.stack); n > 0 && stack.stack[n-1].interp == interp {
		stack.stack[n-1].depth--
		if stack.stack[n-1].depth <= 0 {
			stack.stack = stack.stack[:len(stack.stack)-1]
		}
	}
	// Goroutine ids are never reused, so an empty stack means this
	// goroutine is done with executions for good: drop the entry entirely
	// or a long-lived embedding process leaks one map entry per goroutine
	// that ever ran an evaluation. The entry is only ever touched by its
	// owning goroutine, so the delete cannot race another user.
	if len(stack.stack) == 0 {
		executionTokens.Delete(id)
	}
}

// innermostExecutionToken returns the calling goroutine's innermost token
// for interp, if any.
func innermostExecutionToken(interp *Interpreter) (*executionToken, bool) {
	entry, ok := executionTokens.Load(currentGoroutineID())
	if !ok {
		return nil, false
	}
	stack := entry.(*executionTokenStack)
	if len(stack.stack) == 0 || stack.stack[len(stack.stack)-1].interp != interp {
		return nil, false
	}
	return stack.stack[len(stack.stack)-1], true
}

// Eval evaluates Go code represented as a string. Eval returns the last result
// computed by the interpreter, and a non nil error in case of failure.
//
// Evaluations on the same interpreter are serialized: an Eval blocked in
// interpreted code (for example a channel receive) prevents later Evals from
// starting until it completes. Use EvalWithContext to bound or cancel a
// blocking evaluation. An Eval issued from a host callback running
// synchronously on the paused execution's goroutine (a writer, a Stringer,
// or a conversion hook) is recognized as reentrant and does not wait, at any
// native call depth. Goroutines that run interpreted code outside an
// evaluation's goroutine — goroutines spawned by an interpreted `go`
// statement, and host goroutines invoking retained interpreted function
// values — hold no token: an Eval from them waits for the gate like any
// unrelated goroutine, instead of running concurrently with the gated
// execution. If the gated evaluation is transitively waiting on such a
// goroutine (for example through a channel handshake), that wait is a
// user-level deadlock, exactly like blocking on an already-held mutex in
// Go; earlier versions bypassed the gate in this situation and raced the
// running evaluation instead.
func (interp *Interpreter) Eval(src string) (res reflect.Value, err error) {
	release := interp.acquireExecution()
	defer release()
	return interp.eval(src, "", true)
}

// EvalPath evaluates Go code located at path and returns the last result computed
// by the interpreter, and a non nil error in case of failure.
// The main function of the main package is executed if present.
func (interp *Interpreter) EvalPath(path string) (res reflect.Value, err error) {
	release := interp.acquireExecution()
	defer release()
	return interp.evalPathWithCancel(path, nil)
}

func (interp *Interpreter) evalPathWithCancel(path string, cancel <-chan struct{}) (res reflect.Value, err error) {
	return interp.evalPathWithCancelPublication(path, cancel, nil)
}

func (interp *Interpreter) evalPathWithCancelPublication(path string, cancel <-chan struct{}, publication *executionPublication) (res reflect.Value, err error) {
	path = filepath.ToSlash(path) // Ensure path is in Unix format. Since we work with fs.FS, we need to use Unix path.
	if !isFile(interp.opt.filesystem, path) {
		_, err := interp.importSrcWithCancel(mainID, path, NoTest, cancel)
		return res, err
	}

	b, err := fs.ReadFile(interp.filesystem, path)
	if err != nil {
		return res, err
	}
	return interp.evalWithCancelPublication(string(b), path, false, cancel, publication)
}

// EvalPathWithContext evaluates Go code located at path and returns the last
// result computed by the interpreter, and a non nil error in case of failure.
// The main function of the main package is executed if present.
func (interp *Interpreter) EvalPathWithContext(ctx context.Context, path string) (res reflect.Value, err error) {
	if err := ctx.Err(); err != nil {
		return reflect.Value{}, err
	}
	release, err := interp.acquireExecutionWithContext(ctx)
	if err != nil {
		return reflect.Value{}, err
	}
	defer release()
	// Keep the execution owner alive after a successful return. Values crossing
	// this API can contain interpreted functions, and the caller context governs
	// only the in-flight evaluation, not later calls through those values.
	runDone := make(chan struct{})
	publication := newExecutionPublication()

	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err = interp.evalPathWithCancelPublication(path, runDone, publication)
	}()

	select {
	case <-ctx.Done():
		close(runDone)
		publication.decision <- false
		return reflect.Value{}, ctx.Err()
	case <-publication.ready:
		if err := ctx.Err(); err != nil {
			close(runDone)
			publication.decision <- false
			return reflect.Value{}, err
		}
		publication.decision <- true
		<-done
	case <-done:
		if err := ctx.Err(); err != nil {
			return reflect.Value{}, err
		}
	}
	return res, err
}

// EvalTest evaluates Go code located at path, including test files with "_test.go" suffix.
// A non nil error is returned in case of failure.
// The main function, test functions and benchmark functions are internally compiled but not
// executed. Test functions can be retrieved using the Symbol() method.
//
// Like Eval, it is serialized with other evaluations: it waits for any
// in-flight evaluation to complete before running package initialization.
func (interp *Interpreter) EvalTest(path string) error {
	// Package-level initialization runs interpreted source, so it must not
	// overlap an in-flight evaluation: take the execution gate like Eval.
	release := interp.acquireExecution()
	defer release()
	_, err := interp.importSrc(mainID, path, Test)
	return err
}

func isFile(filesystem fs.FS, path string) bool {
	fi, err := fs.Stat(filesystem, path)
	return err == nil && fi.Mode().IsRegular()
}

func (interp *Interpreter) eval(src, name string, inc bool) (res reflect.Value, err error) {
	return interp.evalWithCancel(src, name, inc, nil)
}

func (interp *Interpreter) evalWithCancel(src, name string, inc bool, cancel <-chan struct{}) (res reflect.Value, err error) {
	return interp.evalWithCancelPublication(src, name, inc, cancel, nil)
}

func (interp *Interpreter) evalWithCancelPublication(src, name string, inc bool, cancel <-chan struct{}, publication *executionPublication) (res reflect.Value, err error) {
	prog, err := interp.compileSrcWithCancel(src, name, inc, cancel)
	if err != nil {
		return res, err
	}

	if interp.noRun {
		return res, err
	}

	return interp.executeWithPublication(prog, cancel, publication)
}

// EvalWithContext evaluates Go code represented as a string. It returns
// a map on current interpreted package exported symbols.
func (interp *Interpreter) EvalWithContext(ctx context.Context, src string) (reflect.Value, error) {
	if err := ctx.Err(); err != nil {
		return reflect.Value{}, err
	}
	release, err := interp.acquireExecutionWithContext(ctx)
	if err != nil {
		return reflect.Value{}, err
	}
	defer release()
	var v reflect.Value

	// Do not close the execution owner on success: a returned interpreted
	// function must remain callable after EvalWithContext itself has returned.
	runDone := make(chan struct{})
	publication := newExecutionPublication()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var pc [64]uintptr
				n := runtime.Callers(1, pc[:])
				err = Panic{Value: r, Callers: pc[:n], Stack: debug.Stack()}
			}
			close(done)
		}()
		v, err = interp.evalWithCancelPublication(src, "", true, runDone, publication)
	}()

	select {
	case <-ctx.Done():
		close(runDone)
		publication.decision <- false
		return reflect.Value{}, ctx.Err()
	case <-publication.ready:
		if contextErr := ctx.Err(); contextErr != nil {
			close(runDone)
			publication.decision <- false
			return reflect.Value{}, contextErr
		}
		publication.decision <- true
		<-done
	case <-done:
		if contextErr := ctx.Err(); contextErr != nil {
			return reflect.Value{}, contextErr
		}
	}
	return v, err
}

func (interp *Interpreter) runid() uint64 { return atomic.LoadUint64(&interp.id) }

// ignoreScannerError returns true if the error from Go scanner can be safely ignored
// to let the caller grab one more line before retrying to parse its input.
func ignoreScannerError(e *scanner.Error, s string) bool {
	msg := e.Msg
	if strings.HasSuffix(msg, "found 'EOF'") {
		return true
	}
	if msg == "raw string literal not terminated" {
		return true
	}
	if strings.HasPrefix(msg, "expected operand, found '}'") && !strings.HasSuffix(s, "}") {
		return true
	}
	return false
}

// ImportUsed automatically imports pre-compiled packages included by Use().
// This is mainly useful for REPLs, or single command lines. In case of an ambiguous default
// package name, for example "rand" for crypto/rand and math/rand, the package name is
// constructed by replacing the last "/" by a "_", producing crypto_rand and math_rand.
// ImportUsed should not be called more than once, and not after a first Eval, as it may
// rename packages.
//
// ImportUsed mutates the interpreter symbol tables, so it is serialized with
// evaluations on the same interpreter like Eval: it waits for an in-flight
// evaluation to complete, and a host callback of a paused evaluation is
// recognized as reentrant.
func (interp *Interpreter) ImportUsed() {
	release := interp.acquireExecution()
	defer release()
	sc := interp.universe
	// Count base names first: when several packages share the same base
	// name, the plain name is ambiguous (and would depend on map iteration
	// order), so it must not be bound at all. Explicit imports of these
	// packages bind their own file-level symbol and resolve deterministically.
	ambiguous := map[string]bool{}
	count := map[string]int{}
	for k := range interp.binPkg {
		base := path.Base(k)
		count[base]++
		if count[base] > 1 {
			ambiguous[base] = true
		}
	}
	for k := range interp.binPkg {
		// By construction, the package name is the last path element of the key.
		name := path.Base(k)
		if ambiguous[name] {
			// Bind only the renamed, path-based symbol for ambiguous names.
			sc.sym[key2name(fixKey(k))] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT, path: k, scope: sc}}
			continue
		}
		if sym, ok := sc.sym[name]; ok && sym.kind == pkgSym && sym.typ != nil {
			// Handle collision with a pre-existing symbol by renaming the old
			// entry, then bind the plain name to the new package.
			name2 := key2name(fixKey(sym.typ.path))
			sc.sym[name2] = sym
			if name2 != name {
				delete(sc.sym, name)
			}
			name = key2name(fixKey(k))
			sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT, path: k, scope: sc}}
			continue
		}
		sc.sym[name] = &symbol{kind: pkgSym, typ: &itype{cat: binPkgT, path: k, scope: sc}}
	}
}

func key2name(name string) string {
	return path.Join(name, DefaultSourceName)
}

func fixKey(k string) string {
	i := strings.LastIndex(k, "/")
	if i >= 0 {
		k = k[:i] + "_" + k[i+1:]
	}
	return k
}

// REPL performs a Read-Eval-Print-Loop on input reader.
// Results are printed to the output writer of the Interpreter, provided as option
// at creation time. Errors are printed to the similarly defined errors writer.
// The last interpreter result value and error are returned.
func (interp *Interpreter) REPL() (reflect.Value, error) {
	in, out, errs := interp.stdin, interp.stdout, interp.stderr
	ctx, cancel := context.WithCancel(context.Background())
	end := make(chan struct{})     // channel to terminate the REPL
	sig := make(chan os.Signal, 1) // channel to trap interrupt signal (Ctrl-C)
	lines := make(chan string)     // channel to read REPL input lines
	prompt := getPrompt(in, out)   // prompt activated on tty like IO stream
	s := bufio.NewScanner(in)      // read input stream line by line
	var v reflect.Value            // result value from eval
	var err error                  // error from eval
	src := ""                      // source string to evaluate

	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	prompt(v)

	go func() {
		defer close(end)
		for s.Scan() {
			lines <- s.Text()
		}
		if e := s.Err(); e != nil {
			fmt.Fprintln(errs, e)
		}
	}()

	go func() {
		for {
			select {
			case <-sig:
				cancel()
				lines <- ""
			case <-end:
				return
			}
		}
	}()

	for {
		var line string

		select {
		case <-end:
			cancel()
			return v, err
		case line = <-lines:
			src += line + "\n"
		}

		v, err = interp.EvalWithContext(ctx, src)
		if err != nil {
			switch e := err.(type) {
			case scanner.ErrorList:
				if len(e) > 0 && ignoreScannerError(e[0], line) {
					continue
				}
				fmt.Fprintln(errs, strings.TrimPrefix(e[0].Error(), DefaultSourceName+":"))
			case Panic:
				fmt.Fprintln(errs, e.Value)
				fmt.Fprintln(errs, string(e.Stack))
			default:
				fmt.Fprintln(errs, err)
			}
		}
		if errors.Is(err, context.Canceled) {
			ctx, cancel = context.WithCancel(context.Background())
		}
		src = ""
		prompt(v)
	}
}

func doPrompt(out io.Writer) func(v reflect.Value) {
	return func(v reflect.Value) {
		if v.IsValid() {
			fmt.Fprintln(out, ":", v)
		}
		fmt.Fprint(out, "> ")
	}
}

// getPrompt returns a function which prints a prompt only if input is a terminal.
func getPrompt(in io.Reader, out io.Writer) func(reflect.Value) {
	forcePrompt, _ := strconv.ParseBool(os.Getenv("YAEGI_PROMPT"))
	if forcePrompt {
		return doPrompt(out)
	}
	s, ok := in.(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return func(reflect.Value) {}
	}
	stat, err := s.Stat()
	if err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		return doPrompt(out)
	}
	return func(reflect.Value) {}
}
