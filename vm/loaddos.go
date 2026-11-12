package vm

// Bounds against load-time denial of service. Options.Budget covers VM
// execution only; the parse, type check and SSA build phases run on the
// plugin source before any VM code runs, with no budget. A malicious plugin
// can make these phases take unbounded time or memory and block the
// configuration load or reload of the host for ever.
//
// Two kinds of bound live here:
//   - a deadline on the whole Load (LoadContext and Options.LoadTimeout):
//     Load returns an error when the deadline passes, so a reload is never
//     blocked for ever;
//   - cheap static limits applied before type checking (total source size,
//     number of files and packages): they reject the inputs that are large
//     by construction (huge files, many packages, deep import chains).
//
// A third kind lives in loadbomb.go and loadinst.go: static checks that
// refuse the small but pathological packages that are known to make
// go/types, go/constant or the SSA builder spend minutes or gigabytes (types,
// constants and generic instances of exponential size).
//
// What stays unbounded is a slow path of go/types that these checks do not
// know: go/types has no cancellation, so the deadline bounds the time Load
// waits for it, but the abandoned Check goroutine keeps running in the
// background until it returns (the load checks the deadline between
// packages, so it stops at the end of the package being checked or built).
// The gc compiler has the same cost on these inputs (it is just as slow),
// which is why yaegi v2 cannot do better without a separate process. Two
// bounds keep this cost from adding up: a plugin is loaded once at a time
// (loadSlots in plugin.go: a new load waits for the abandoned one), and the
// source size limit bounds the memory of one load (DefaultLoadLimits).

import (
	"context"
	"fmt"
	"go/types"
	"time"
)

// LoadLimits bounds the load of a plugin. A nil *LoadLimits (the Options
// default) uses DefaultLoadLimits. A zero field means "no limit for this
// one".
//
// The limits are generous: the largest plugin of the measured catalog is a
// few hundred kilobytes of source in a handful of packages, far below every
// default. They are a guard rail against source that is large by
// construction, not a tuning knob for honest plugins.
//
// They do NOT bound the cost of type checking a small but crafted package:
// nested generic aliases such as type D[T any] = P[T, T] chained tens of
// times, or a string constant doubled with s = s + s tens of times, make
// go/types or go/constant allocate gigabytes from a few lines of source.
// The source size limit does not catch them (the source is tiny); the
// static checks of loadbomb.go refuse the known shapes, and go/types cannot
// be cancelled. Use LoadTimeout (or LoadContext) so that Load returns on
// such a plugin instead of blocking for ever.
type LoadLimits struct {
	// MaxTotalSourceBytes is the sum of the sizes of all the .go files
	// parsed (the plugin, its module and its vendored modules). 0 means no
	// limit. It also bounds, apart, the total size of the //go:embed files
	// of a load, which never passes 64 MiB (vm/embed.go).
	MaxTotalSourceBytes int64
	// MaxFiles is the number of .go files parsed. 0 means no limit. It
	// also bounds, apart, the number of files and directories that the
	// //go:embed patterns of a load visit, which never passes 65536.
	MaxFiles int
	// MaxPackages is the number of source packages type-checked (the
	// standard library packages, type-checked once and embedded, do not
	// count). 0 means no limit.
	MaxPackages int
}

// DefaultLoadLimits is used when Options.LoadLimits is nil.
//
// The source size bounds the memory of the load: go/types and the SSA
// builder use about 60 to 90 MiB of heap at their peak per MiB of real
// source (measured on October 23, 2026: 1.7 GiB for the 30 MiB of the
// Kubernetes client of compare/complex, the largest plugin measured; 0.3 to
// 0.8 GiB for grpc, otel, openai and goja, 5 to 9 MiB each), and up to 150
// MiB per MiB for dense generated code. The largest plugin of the Traefik
// catalog reads 1.3 MiB. 64 MiB keeps twice the Kubernetes client and bounds
// a crafted plugin to about 10 GiB, where 256 MiB allowed about 38 GiB.
var DefaultLoadLimits = LoadLimits{
	MaxTotalSourceBytes: 1 << 26, // 64 MiB of Go source
	MaxFiles:            50000,
	MaxPackages:         5000,
}

func (opts Options) loadLimits() LoadLimits {
	if opts.LoadLimits != nil {
		return *opts.LoadLimits
	}
	return DefaultLoadLimits
}

// loadErrorf returns a *LoadError (run.go): the plugin is refused before it
// runs, it is never a bug in the plugin code.
func loadErrorf(format string, a ...any) *LoadError {
	return &LoadError{Msg: fmt.Sprintf(format, a...)}
}

// loadBudget tracks the static limits across the whole load. Its methods
// return a *LoadError when a limit is passed, and nil otherwise.
type loadBudget struct {
	limits LoadLimits
	bytes  int64
	files  int
	pkgs   int
	// constBytes is the running total of the bytes of the string constants
	// of the source packages (checkSourceSize, maxConstBytes).
	constBytes int64
	// tripped holds the first limit error. go/types wraps the errors a
	// types.Importer returns into "could not import X (...)", so the first
	// limit error, raised from inside type checking, would lose its type.
	// loadSource reports this one instead when the load fails.
	tripped error
}

func newLoadBudget(l LoadLimits) *loadBudget { return &loadBudget{limits: l} }

func (b *loadBudget) trip(err error) error {
	if b.tripped == nil {
		b.tripped = err
	}
	return err
}

func (b *loadBudget) addPackage() error {
	b.pkgs++
	if b.limits.MaxPackages > 0 && b.pkgs > b.limits.MaxPackages {
		return b.trip(loadErrorf("vm load: too many source packages (limit %d): the plugin module and its vendored modules have more packages than yaegi v2 loads", b.limits.MaxPackages))
	}
	return nil
}

func (b *loadBudget) addFile(size int64) error {
	b.files++
	if b.limits.MaxFiles > 0 && b.files > b.limits.MaxFiles {
		return b.trip(loadErrorf("vm load: too many source files (limit %d)", b.limits.MaxFiles))
	}
	b.bytes += size
	if b.limits.MaxTotalSourceBytes > 0 && b.bytes > b.limits.MaxTotalSourceBytes {
		return b.trip(loadErrorf("vm load: source too large (limit %d bytes)", b.limits.MaxTotalSourceBytes))
	}
	return nil
}

// Deadline helpers.

// ctxErr returns a *LoadError wrapping the context error, or nil. It is
// called between phases and between packages so that a cancelled or timed-out
// load stops at the next boundary. It cannot interrupt a single go/types
// Check call, which has no cancellation.
//
// It also compares the deadline with the clock: ctx.Err() is set by a timer,
// and on Windows that timer can fire well after the deadline.
func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return loadErrorf("vm load: %v", err)
	}
	if d, ok := ctx.Deadline(); ok && !time.Now().Before(d) {
		return loadErrorf("vm load: %v", context.DeadlineExceeded)
	}
	return nil
}

// Huge values.

// maxZeroElems bounds the number of yaegi v2 Value entries that one zero value
// allocates: an array, a struct, or any nesting of them. A plugin that
// declares var X [1<<40]byte, a local [1<<34]byte, new([1<<34]byte), or an
// array composite literal with a huge index such as [...]byte{1<<40: 1},
// would otherwise make yaegi v2 allocate trillions of entries and the host dies
// with a fatal out-of-memory, which no recover and no Budget can stop (the
// allocation is not plugin code running, it is the VM building the value).
// gc refuses most of these declarations at compile time ("symbol too
// large"). 1<<26 entries are about 1.5 GiB of Value: far above any honest
// plugin, below a fatal allocation on a small host. The same bound applies to
// one conversion of a native value to VM values (natSeen.grow).
//
// It bounds one value, not the plugin: many values just under it (64 times
// new([1<<25]byte)) still exhaust memory, like any "allocate until out of
// memory" plugin (docs/SANDBOX.md, Memory row). Native allocations
// (strings.Repeat, a native []byte) are not counted.
const maxZeroElems = 1 << 26

// tooLarge returns the error raised when a value of type t is too large for
// the VM. what says which value it is.
func tooLarge(what string, t types.Type) *LoadError {
	return loadErrorf("vm: %s of type %s is too large for the VM (more than %d entries): gc refuses it or needs as much memory", what, t.String(), maxZeroElems)
}

// checkZero panics with a *LoadError when the zero value of t would allocate
// more than maxZeroElems Value entries. The panic is turned into an error by
// the recover of Load, LoadCached and the call wrappers, so a huge value is
// a load error (or, if it is only reached when the plugin runs, a call
// error) instead of a fatal OOM. It is called on every zero value (m.zero)
// and at compile time on every allocation site that does not build its value
// with m.zero (OpAlloc of a flat array, see compiler.mem). The count of
// each type is kept in m.zeroCounts, so a zero value at run time (clear of a
// slice of structs, reflect.New, decoders) pays one map lookup.
func (m *Machine) checkZero(t types.Type) {
	switch t.Underlying().(type) {
	case *types.Struct, *types.Array:
	default:
		return
	}
	var n int64
	if c, ok := m.zeroCounts.Load(t); ok {
		n = c.(int64)
	} else {
		n = m.zeroEntries(t)
		m.zeroCounts.Store(t, n)
	}
	if n > maxZeroElems {
		panic(tooLarge("value", t))
	}
}

// grow counts n more Value entries for the conversion of a native value to
// the VM (fromNativeS, fromNativeDeepC, natToVMs, retVM), before they are
// allocated. It panics with a *LoadError when the conversion would allocate
// more than maxZeroElems entries in total: a native [1<<28]byte costs 256 MiB
// in Go and 6 GiB as VM values (one 24 byte Value per byte), and a native
// slice of zero-sized elements costs nothing in Go. Like checkZero, the panic
// becomes a load error or a call error instead of a fatal out-of-memory.
func (s *natSeen) grow(n int) {
	if int64(n) > maxZeroElems-s.entries {
		panic(loadErrorf("vm: native value is too large to convert for the VM (more than %d entries): gc needs much less memory for it, the VM refuses it", maxZeroElems))
	}
	s.entries += int64(n)
}

// checkMakeSlice panics with a *LoadError when make([]T, n) would allocate
// more than maxZeroElems Value entries for the zero values of its n elements
// (each element has elem entries: an array or a struct). The slice itself, one
// Value per element, is bounded by makeSliceCheck like with gc.
func checkMakeSlice(n int64, elem int64, t types.Type) {
	if elem > 0 && satMul(n, elem) > maxZeroElems {
		panic(tooLarge(fmt.Sprintf("make of %d elements", n), types.NewSlice(t)))
	}
}

// zeroEntries returns the number of Value entries that the zero value of t
// allocates (see m.zero), not counting the Value of t itself. A struct has
// one entry per field plus the entries of each field; an array has one entry
// per element plus the entries of each element. The count is saturated:
// a result above maxZeroElems means "too large" and the exact number is not
// computed, so the arithmetic never overflows. The counts of nested structs
// and arrays are kept in a map, so a type whose size doubles at each level
// (type T2 struct{ a, b T1 }, and so on) costs one step per level.
func (m *Machine) zeroEntries(t types.Type) int64 {
	var memo map[types.Type]int64
	return m.zeroEntriesMemo(t, &memo, true)
}

// zeroEntriesMemo is zeroEntries with the map of counts (nil until a nested
// count is kept). top is true for the type of the whole value, whose count
// is not kept: the zero value of a flat struct allocates no map.
func (m *Machine) zeroEntriesMemo(t types.Type, memo *map[types.Type]int64, top bool) int64 {
	if m.isNativeNamed(t) {
		if _, ok := t.Underlying().(*types.Struct); ok {
			return 0 // a native Go value (reflect.New), sized by gc
		}
	}
	var n int64
	switch u := t.Underlying().(type) {
	case *types.Struct:
		if c, ok := (*memo)[t]; ok {
			return c
		}
		for i := 0; i < u.NumFields() && n <= maxZeroElems; i++ {
			n = satAdd(n, satAdd(1, m.zeroEntriesMemo(u.Field(i).Type(), memo, false)))
		}
	case *types.Array:
		if u.Len() <= 0 {
			return 0
		}
		if c, ok := (*memo)[t]; ok {
			return c
		}
		e := m.zeroEntriesMemo(u.Elem(), memo, false)
		if u.Len() >= zsliceMin && zeroSized(u.Elem()) {
			n = e // a zslice: one zero element for the whole array
		} else {
			n = satMul(u.Len(), satAdd(1, e))
		}
	default:
		return 0
	}
	if top {
		return n
	}
	if *memo == nil {
		*memo = map[types.Type]int64{}
	}
	(*memo)[t] = n
	return n
}

// satAdd and satMul compute on counts in [0, maxZeroElems+1]: the result is
// capped at maxZeroElems+1, so they never overflow. a and b are not
// negative.
func satAdd(a, b int64) int64 {
	if a > maxZeroElems || b > maxZeroElems || a+b > maxZeroElems {
		return maxZeroElems + 1
	}
	return a + b
}

func satMul(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > maxZeroElems || b > maxZeroElems || a*b > maxZeroElems {
		return maxZeroElems + 1 // a, b <= 1<<26: a*b does not overflow
	}
	return a * b
}
