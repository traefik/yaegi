package vm

// Stack introspection of interpreted code: runtime.Caller, runtime.Callers,
// runtime.CallersFrames, runtime.FuncForPC of the PCs they give,
// runtime.Stack, runtime/debug.Stack and debug.PrintStack, and the
// traceback of an unrecovered panic.
//
// Native calls of these functions would see the Go frames of the VM
// (exec.go, reflect...). The VM knows its own frames instead:
//
//   - The compiler records the source position of each SSA instruction
//     in a compact table per function (Func.pos: one entry per change of
//     position), so the cost is paid at compile time only.
//   - A context keeps the index of the instruction it runs (ctx.at), and
//     each caller frame keeps its return index (frame.pc). A context that
//     runs a deferred call links to the context of the function that
//     deferred it (ctx.parent), so the walk goes on through defers, with a
//     runtime.gopanic frame between them during a panic, as in gc.
//   - A frame gets a made-up program counter: a tag bit, a number for the
//     function (Func.pcID) and the instruction index. These numbers are
//     not addresses: they cannot be turned into pointers to VM memory.
//     FuncForPC and CallersFrames decode them; other PCs go to the
//     runtime. The few runtime frames that programs look for
//     (runtime.Callers, runtime.gopanic, runtime.main, runtime.goexit)
//     are real PCs of these functions, taken from the runtime itself.
//
// A callback from native code (sort.Slice, a String method called by fmt)
// links to the context that called the native code, so the walk goes on
// below it; the native frames between them are not shown. The walk stops
// at a call from the host (a plugin's ServeHTTP). Calls of these functions
// through a function value (f := runtime.Caller) still go to the runtime.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/tools/go/ssa"
)

// readsStack reports whether one of the interpreted packages can read the
// stack: it imports runtime or runtime/debug.
func readsStack(interp map[*types.Package]bool) bool {
	for p, ok := range interp {
		if !ok {
			continue
		}
		for _, i := range p.Imports() {
			if i.Path() == "runtime" || i.Path() == "runtime/debug" {
				return true
			}
		}
	}
	return false
}

// posRun says that the instructions from index pc on come from the source
// position pos (until the next posRun).
type posRun struct {
	pc  int32
	pos token.Pos
}

// notePos records that the next instruction comes from pos.
func (c *compiler) notePos(pos token.Pos) {
	if !pos.IsValid() {
		return // same position as the previous instruction
	}
	pc := int32(len(c.code))
	if n := len(c.pos); n > 0 {
		if c.pos[n-1].pc >= pc {
			// The previous instruction emitted no code (or its code was
			// dropped): the run starts here.
			c.pos = c.pos[:n-1]
			for n = len(c.pos); n > 0 && c.pos[n-1].pc >= pc; n = len(c.pos) {
				c.pos = c.pos[:n-1]
			}
		}
		if n > 0 && c.pos[n-1].pos == pos {
			return
		}
	}
	c.pos = append(c.pos, posRun{pc: pc, pos: pos})
}

// posOf returns the position of the code of ins: a load or store fused with
// its address computation can panic on the index or the nil pointer of the
// address, at the position of the address.
func (c *compiler) posOf(ins ssa.Instruction) token.Pos {
	switch v := ins.(type) {
	case *ssa.UnOp:
		if a, ok := v.X.(ssa.Instruction); ok && v.Op == token.MUL && c.fused[v.X] && a.Pos().IsValid() {
			return a.Pos()
		}
	case *ssa.Store:
		if a, ok := v.Addr.(ssa.Instruction); ok && c.fused[v.Addr] && a.Pos().IsValid() {
			return a.Pos()
		}
	case *ssa.RunDefers:
		return c.exitPos(v.Block())
	case *ssa.DebugRef:
		return token.NoPos
	}
	return ins.Pos()
}

// exitPos returns the position where the function returns from block b:
// the return statement, or the closing brace of the body, where gc runs
// the deferred calls.
func (c *compiler) exitPos(b *ssa.BasicBlock) token.Pos {
	if n := len(b.Instrs); n > 0 {
		if r, ok := b.Instrs[n-1].(*ssa.Return); ok && r.Pos().IsValid() {
			return r.Pos()
		}
	}
	switch s := c.f.ssa.Syntax().(type) {
	case *ast.FuncDecl:
		if s.Body != nil {
			return s.Body.Rbrace
		}
	case *ast.FuncLit:
		return s.Body.Rbrace
	}
	return token.NoPos
}

// posAt returns the source position of instruction k of f.
func (f *Func) posAt(k int) token.Pos {
	i := sort.Search(len(f.pos), func(i int) bool { return int(f.pos[i].pc) > k })
	if i == 0 {
		return f.ssa.Pos()
	}
	return f.pos[i-1].pos
}

// hidden reports whether the stack walks skip the frames of f: like gc,
// they do not show the method wrappers (an instance of a generic function
// is not a wrapper).
func (f *Func) hidden() bool {
	return f.wrapper && f.ssa.Origin() == nil
}

// isInitFunc reports whether f is an init function of its package (ssa
// names them init#1, init#2...).
func isInitFunc(f *ssa.Function) bool {
	return f.Parent() == nil && strings.HasPrefix(f.Name(), "init#")
}

// frameAt is a frame that a panic left (ctx.unwound).
type frameAt struct {
	fn *Func
	at int
}

// ctxKind tells what started a context that has no parent.
type ctxKind uint8

const (
	ctxNative ctxKind = iota // a call from native code (Machine.Call)
	ctxMain                  // the main goroutine of RunMain
	ctxGo                    // a go statement
)

// Program counters of interpreted frames: pcTag, then the function number,
// then the instruction index.
const (
	ptrBits  = 32 << (^uintptr(0) >> 63)
	pcKBits  = 14 << (ptrBits / 64) // 28 bits on 64-bit platforms, 14 on 32-bit ones
	pcIDBits = ptrBits - 2 - pcKBits
	pcTag    = uintptr(1) << (ptrBits - 2)
)

// vmPC returns the program counter of instruction k of fn.
func (m *Machine) vmPC(fn *Func, k int) uintptr {
	id := fn.pcID.Load()
	if id == 0 {
		m.pcMu.Lock()
		if id = fn.pcID.Load(); id == 0 && len(m.pcFuncs) < 1<<pcIDBits-1 {
			m.pcFuncs = append(m.pcFuncs, fn)
			id = int32(len(m.pcFuncs))
			fn.pcID.Store(id)
		}
		m.pcMu.Unlock()
	}
	k = max(0, min(k, 1<<pcKBits-2)) // room for the +1 of a return address
	return pcTag | uintptr(id)<<pcKBits | uintptr(k)
}

// decodePC returns the function and the instruction index of a program
// counter made by vmPC.
func (m *Machine) decodePC(pc uintptr) (*Func, int, bool) {
	if pc>>(ptrBits-2) != 1 {
		return nil, 0, false
	}
	id := int(pc >> pcKBits & (1<<pcIDBits - 1))
	m.pcMu.Lock()
	defer m.pcMu.Unlock()
	if id < 1 || id > len(m.pcFuncs) {
		return nil, 0, false
	}
	// Past the end of the code (a return address may be one past the
	// last instruction), the PC is not in fn: like gc's past the end of
	// a function.
	fn, k := m.pcFuncs[id-1], int(pc&(1<<pcKBits-1))
	if k > len(fn.code) {
		return nil, 0, false
	}
	return fn, k, true
}

// Real PCs of runtime frames, as runtime.Callers gives them.
var rtPCs struct {
	// main: runtime.main, where it calls main.main (noteMainPC); until
	// then, where it runs the package initialization.
	main       atomic.Uintptr
	mainOnce   sync.Once
	goexit     uintptr // bottom of any goroutine
	once       sync.Once
	callers    uintptr // runtime.Callers
	gopanic    uintptr // runtime.gopanic, that runs the deferred calls of a panic
	debugStack string  // the runtime/debug.Stack frame, as debug.Stack prints it
}

func init() {
	// Package initialization runs on the main goroutine, under
	// runtime.main.
	var pcs [64]uintptr
	n := runtime.Callers(0, pcs[:])
	fr := runtime.CallersFrames(pcs[:n])
	for i := 0; ; i++ {
		f, more := fr.Next()
		switch f.Function {
		case "runtime.main":
			rtPCs.main.Store(f.PC + 1)
		case "runtime.goexit":
			rtPCs.goexit = f.PC + 1
		}
		if !more {
			break
		}
	}
}

// noteMainPC takes the PC of runtime.main where it calls main.main, the
// frame that gc shows below the main function of the program. It must run
// under main.main (RunMain called by the main function of the host).
func noteMainPC() {
	rtPCs.mainOnce.Do(func() {
		var pcs [64]uintptr
		n := runtime.Callers(0, pcs[:])
		fr := runtime.CallersFrames(pcs[:n])
		for {
			f, more := fr.Next()
			if f.Function == "runtime.main" {
				rtPCs.main.Store(f.PC + 1)
				return
			}
			if !more {
				return
			}
		}
	})
}

// initRTPCs finds the runtime frames that only a call can show.
func initRTPCs() {
	var pcs [1]uintptr
	if runtime.Callers(0, pcs[:]) == 1 {
		rtPCs.callers = pcs[0]
	}
	func() {
		defer func() {
			recover()
			var pcs [16]uintptr
			n := runtime.Callers(0, pcs[:])
			for _, pc := range pcs[:n] {
				if f := runtime.FuncForPC(pc - 1); f != nil && f.Name() == "runtime.gopanic" {
					rtPCs.gopanic = pc
					break
				}
			}
		}()
		panic("probe")
	}()
	lines := strings.SplitN(string(debug.Stack()), "\n", 4)
	if len(lines) == 4 && strings.HasPrefix(lines[1], "runtime/debug.Stack(") {
		rtPCs.debugStack = lines[1] + "\n" + lines[2] + "\n"
	}
}

// stackPCs returns the PCs of the frames of the goroutine of cx, innermost
// first, as runtime.Callers gives them (return addresses: one more than the
// PC of the call), after skipping skip frames, at most max of them.
func (cx *ctx) stackPCs(skip, max int) []uintptr {
	rtPCs.once.Do(initRTPCs)
	var out []uintptr
	add := func(pc uintptr) bool {
		switch {
		case pc == 0:
		case skip > 0:
			skip--
		case len(out) < max:
			out = append(out, pc)
		default:
			return false
		}
		return true
	}
	m := cx.m
	var prev *Func // the frame above
	frame := func(fn *Func, k int) bool {
		// gc does not show the method wrappers, nor a package
		// initializer below an init function: the runtime calls them.
		hide := fn.hidden() || prev != nil && fn.ssa.Synthetic == "package initializer" && isInitFunc(prev.ssa)
		prev = fn
		return hide || add(m.vmPC(fn, k)+1)
	}
	for c := cx; c != nil; c = c.parent {
		for _, f := range c.unwound {
			if !frame(f.fn, f.at) {
				return out
			}
		}
		if c.fn != nil && !frame(c.fn, c.at) {
			return out
		}
		for i := len(c.frames) - 1; i >= 0; i-- {
			if f := c.frames[i]; !frame(f.fn, f.pc-1) {
				return out
			}
		}
		if c.panic != nil && !add(rtPCs.gopanic) {
			return out
		}
		if c.parent == nil {
			switch c.kind {
			case ctxMain:
				add(rtPCs.main.Load())
				add(rtPCs.goexit)
			case ctxGo:
				add(rtPCs.goexit)
			}
		}
	}
	return out
}

// frames returns the frames of pcs, like runtime.CallersFrames.
func (m *Machine) frames(pcs []uintptr) []runtime.Frame {
	var out []runtime.Frame
	for i := 0; i < len(pcs); {
		if fn, k, ok := m.decodePC(pcs[i]); ok {
			if k > 0 {
				k-- // the call, not the return address
			}
			file, line := m.position(fn.posAt(k))
			out = append(out, runtime.Frame{
				PC:       m.vmPC(fn, k),
				Func:     m.lineFunc(fn, k),
				Function: funcName(fn.ssa),
				File:     file,
				Line:     line,
				Entry:    uintptr(m.funcPC(fn)),
			})
			i++
			continue
		}
		j := i + 1
		for j < len(pcs) && pcs[j]>>(ptrBits-2) != 1 {
			j++
		}
		fr := runtime.CallersFrames(pcs[i:j])
		for {
			f, more := fr.Next()
			if f.PC != 0 {
				out = append(out, f)
			}
			if !more {
				break
			}
		}
		i = j
	}
	return out
}

// position returns the file and line of pos as gc prints them. The
// position honors the //line directives. go/scanner puts a relative file
// name of a directive in the directory of the file; gc keeps it as
// written, and prints "??" for an empty one.
func (m *Machine) position(pos token.Pos) (string, int) {
	if !pos.IsValid() {
		return "?", 0
	}
	p := m.prog.Fset.Position(pos)
	file := p.Filename
	if file == "" {
		return "??", p.Line
	}
	if raw := m.prog.Fset.PositionFor(pos, false).Filename; file != raw {
		dir := filepath.Dir(raw) + string(filepath.Separator)
		if rel, ok := strings.CutPrefix(file, dir); ok && !m.sourceHas(raw, "line "+file+":") {
			file = rel
		}
	}
	return file, p.Line
}

// sourceKey is a question of sourceHas: does the file name contain s?
type sourceKey struct{ name, s string }

// sourceHas reports whether the source file name contains s. The machine
// keeps the answer (one per //line directive of its code), not the file:
// a process-wide cache of whole files grew with every plugin load and was
// never freed. The file is read once per question.
func (m *Machine) sourceHas(name, s string) bool {
	k := sourceKey{name, s}
	if v, ok := m.sourceAnswers.Load(k); ok {
		return v.(bool)
	}
	data, _ := os.ReadFile(name)
	v, _ := m.sourceAnswers.LoadOrStore(k, bytes.Contains(data, []byte(s)))
	return v.(bool)
}

type lineKey struct {
	fn  *Func
	pos token.Pos
}

// lineFunc returns the stand-in *runtime.Func (funcpc.go) of fn whose
// FileLine gives the position of instruction k. The runtime's funcinl
// ignores the pc argument of FileLine: one stand-in per line.
func (m *Machine) lineFunc(fn *Func, k int) *runtime.Func {
	if !funcInlOK {
		return nil
	}
	key := lineKey{fn, fn.posAt(k)}
	if v, ok := m.lineStandIns.Load(key); ok {
		return (*runtime.Func)(unsafe.Pointer(v.(*funcInl)))
	}
	file, line := m.position(key.pos)
	_, start := m.position(fn.ssa.Pos())
	s := &funcInl{
		ones:      ^uint32(0),
		entry:     uintptr(m.funcPC(fn)),
		name:      funcName(fn.ssa),
		file:      file,
		line:      int32(line),
		startLine: int32(start),
	}
	v, _ := m.lineStandIns.LoadOrStore(key, s) // keeps the stand-in alive
	return (*runtime.Func)(unsafe.Pointer(v.(*funcInl)))
}

// framesMirror has the layout of runtime.Frames (runtime/symtab.go).
// CallersFrames returns a *runtime.Frames whose frames are already
// expanded: its real Next method returns them one by one.
type framesMirror struct {
	callers    []uintptr
	nextPC     uintptr
	frames     []runtime.Frame
	frameStore [2]runtime.Frame
}

// framesOK reports whether runtime.Frames.Next reads framesMirror.
var framesOK = func() bool {
	if unsafe.Sizeof(framesMirror{}) != unsafe.Sizeof(runtime.Frames{}) {
		return false
	}
	fm := &framesMirror{frames: []runtime.Frame{{Function: "a", Line: 1}, {Function: "b"}, {Function: "c"}}}
	fr := (*runtime.Frames)(unsafe.Pointer(fm))
	var got []string
	for {
		f, more := fr.Next()
		got = append(got, f.Function+strconv.Itoa(f.Line)+strconv.FormatBool(more))
		if !more || len(got) > 3 {
			break
		}
	}
	return strings.Join(got, ",") == "a1true,b0true,c0false"
}()

// goroutineHeader returns the first line of a traceback of the current
// goroutine: "goroutine N [running]:".
func goroutineHeader() string {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	if i := bytes.IndexByte(buf[:n], '\n'); i > 0 {
		return string(buf[:i])
	}
	return "goroutine 1 [running]:"
}

// formatStack prints the frames of pcs like a gc traceback.
func (m *Machine) formatStack(b *strings.Builder, pcs []uintptr) {
	for _, f := range m.frames(pcs) {
		name, args, off := f.Function, "(...)", f.PC-f.Entry
		if fn, k, ok := m.decodePC(f.PC); ok {
			if sig := fn.ssa.Signature; sig.Params().Len() == 0 && sig.Recv() == nil {
				args = "()"
			}
			off = uintptr(k)
		} else {
			switch name {
			case "runtime.main", "runtime.goexit":
				continue // gc does not print them
			case "runtime.gopanic":
				name = "panic"
			}
		}
		fmt.Fprintf(b, "%s%s\n\t%s:%d +0x%x\n", name, args, f.File, f.Line, off)
	}
}

// stackText is runtime.Stack of the goroutine of cx, from its current frame.
func (cx *ctx) stackText(debugStack bool) string {
	var b strings.Builder
	b.WriteString(goroutineHeader())
	b.WriteByte('\n')
	pcs := cx.stackPCs(0, 100)
	if debugStack {
		b.WriteString(rtPCs.debugStack)
	}
	cx.m.formatStack(&b, pcs)
	return b.String()
}

// rootCtx returns the first context of the goroutine of cx.
func (cx *ctx) rootCtx() *ctx {
	r := cx
	for r.parent != nil {
		r = r.parent
	}
	return r
}

// notePanic keeps the stack of the panic val that cx caught, for the
// traceback, when the panic ends the program if nobody recovers it. As in
// gc, the traceback shows the stack of the last panic: each new panic
// replaces the stack of the previous one, which some code recovered (an
// interpreted recover, or native code such as fmt, which recovers the
// panics of String methods). A panic that left a nested context (escape)
// and reaches the context below it is the same panic: it keeps its stack.
func (cx *ctx) notePanic(val any) {
	r := cx.rootCtx()
	if r.kind != ctxMain && (r.kind != ctxGo || cx.m.GoPanic != nil) {
		return
	}
	e := &r.esc
	same := e.cx == cx && e.at == cx.at && sameValue(e.val, val)
	*e = escape{}
	if !same || r.ptrace == nil {
		r.ptrace = cx.stackPCs(0, 100)
	}
}

// escape records that the panic val leaves cx, to the context below it.
func (cx *ctx) escape(val any) {
	if cx.parent == nil {
		return
	}
	r := cx.rootCtx()
	if r.ptrace != nil {
		r.esc = escape{cx: cx.parent, at: cx.parent.at, val: val}
	}
}

// escape is a panic that left a nested context (ctx.escape).
type escape struct {
	cx  *ctx // the context below, which gets the panic
	at  int  // its instruction then
	val any
}

// sameValue reports whether a and b are the same panic value (false for
// values that cannot be compared).
func sameValue(a, b any) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// recovered drops the stack of the panic that was just recovered.
func (cx *ctx) recovered() {
	cx.unwound = cx.unwound[:0]
	r := cx.rootCtx()
	r.ptrace, r.esc = nil, escape{}
}

// traceback returns the traceback of the unrecovered panic of the
// goroutine whose first context is cx, or "".
func (cx *ctx) traceback() string {
	if cx.ptrace == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(goroutineHeader())
	b.WriteByte('\n')
	cx.m.formatStack(&b, cx.ptrace)
	return b.String()
}

// ctxHooks replace native functions that read the stack of the calling
// interpreted code.
var ctxHooks = map[string]func(cx *ctx, a []Value) Value{
	"runtime.Caller": func(cx *ctx, a []Value) Value {
		if pcs := cx.stackPCs(int(a[0].N), 1); len(pcs) == 1 {
			if f := cx.m.frames(pcs); len(f) > 0 {
				return Value{R: []Value{{N: int64(f[0].PC)}, {R: f[0].File}, {N: int64(f[0].Line)}, {N: 1}}}
			}
		}
		return Value{R: []Value{{}, {R: ""}, {}, {}}}
	},
	"runtime.Callers": func(cx *ctx, a []Value) Value {
		skip := int(a[0].N)
		dst := a[1]
		n := sliceLen(dst)
		var pcs []uintptr
		if skip <= 0 {
			rtPCs.once.Do(initRTPCs)
			pcs = append(pcs, rtPCs.callers)
			pcs = append(pcs, cx.stackPCs(0, n-1)...)
		} else {
			pcs = cx.stackPCs(skip-1, n)
		}
		pcs = pcs[:min(len(pcs), n)]
		for i, pc := range pcs {
			setUintptr(dst, i, pc)
		}
		return Value{N: int64(len(pcs))}
	},
	"runtime.Stack": func(cx *ctx, a []Value) Value {
		s := cx.stackText(false)
		n := 0
		switch b := a[0].R.(type) {
		case []byte:
			n = copy(b, s)
		case []Value:
			n = min(len(b), len(s))
			for i := 0; i < n; i++ {
				b[i] = Value{N: int64(s[i])}
			}
		case reflect.Value:
			n = reflect.Copy(b, reflect.ValueOf([]byte(s)))
		}
		return Value{N: int64(n)}
	},
	"runtime/debug.Stack": func(cx *ctx, a []Value) Value {
		return cx.m.fromNative(reflect.ValueOf([]byte(cx.stackText(true))), types.NewSlice(types.Typ[types.Byte]))
	},
	"runtime/debug.PrintStack": func(cx *ctx, a []Value) Value {
		os.Stderr.WriteString(cx.stackText(true))
		return Value{}
	},
}

func init() {
	vmHooks["runtime.CallersFrames"] = func(m *Machine, a []Value) Value {
		var pcs []uintptr
		for i, n := 0, sliceLen(a[0]); i < n; i++ {
			pcs = append(pcs, getUintptr(a[0], i))
		}
		if !framesOK {
			return Value{R: reflect.ValueOf(runtime.CallersFrames(pcs))}
		}
		fm := &framesMirror{frames: m.frames(pcs)}
		return Value{R: reflect.ValueOf((*runtime.Frames)(unsafe.Pointer(fm)))}
	}
}

// sliceLen, getUintptr and setUintptr access an interpreted []uintptr.
func sliceLen(v Value) int {
	switch s := v.R.(type) {
	case []Value:
		return len(s)
	case reflect.Value:
		return s.Len()
	}
	return 0
}

func getUintptr(v Value, i int) uintptr {
	switch s := v.R.(type) {
	case []Value:
		return uintptr(s[i].N)
	case reflect.Value:
		return uintptr(s.Index(i).Uint())
	}
	return 0
}

func setUintptr(v Value, i int, pc uintptr) {
	switch s := v.R.(type) {
	case []Value:
		s[i].N = int64(pc)
	case reflect.Value:
		s.Index(i).SetUint(uint64(pc))
	}
}
