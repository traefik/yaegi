package interp

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
	"sort"
	"sync"
)

var rNodeType = reflect.TypeOf((*node)(nil)).Elem()

type Debugger struct {
	interp  *Interpreter
	events  func(*DebugEvent)
	context context.Context
	cancel  context.CancelFunc

	gWait *sync.WaitGroup
	gLock *sync.Mutex
	gID   int
	gLive map[int]*debugRoutine

	result reflect.Value
	err    error
}

type debugRoutine struct {
	id int

	mode   DebugEventReason
	resume chan struct{}

	fDepth int
	fStep  int
}

type frameDebugData struct {
	g     *debugRoutine
	pos   token.Pos
	name  string
	kind  frameKind
	scope *scope
}

type frameKind int

const (
	frameUnknown frameKind = iota
	frameRoot
	frameCall
	frameClosure
)

type DebugOptions struct {
	GoRoutineStartAt1 bool
}

type DebugEvent struct {
	debugger *Debugger
	reason   DebugEventReason
	frame    *frame
}

type DebugFrame struct {
	event  *DebugEvent
	frames []*frame
}

type DebugFrameScope struct {
	frame *frame
}

type DebugVariable struct {
	Name  string
	Value reflect.Value
}

type DebugGoRoutine struct {
	id int
}

type Breakpoint struct {
	Line, Column int
}

type DebugEventReason int

const (
	debugRun DebugEventReason = iota
	DebugPause
	DebugBreak
	DebugEntry
	DebugStepInto
	DebugStepOver
	DebugStepOut
	DebugTerminate

	DebugEnterGoRoutine
	DebugExitGoRoutine
)

func (interp *Interpreter) Debug(ctx context.Context, prog *Program, events func(*DebugEvent), opts *DebugOptions) *Debugger {
	dbg := new(Debugger)
	dbg.interp = interp
	dbg.events = events
	dbg.context, dbg.cancel = context.WithCancel(ctx)
	dbg.gWait = new(sync.WaitGroup)
	dbg.gLock = new(sync.Mutex)
	dbg.gLive = make(map[int]*debugRoutine, 1)

	if opts.GoRoutineStartAt1 {
		dbg.gID = 1
	}

	mainG := dbg.enterGoRoutine()
	mainG.mode = DebugEntry

	interp.debugger = dbg
	interp.frame.debug = &frameDebugData{kind: frameRoot, g: mainG}

	if opts == nil {
		opts = new(DebugOptions)
	}

	go func() {
		defer func() { interp.debugger = nil }()
		defer events(&DebugEvent{reason: DebugTerminate})
		defer dbg.cancel()

		<-mainG.resume
		dbg.events(&DebugEvent{dbg, DebugEnterGoRoutine, interp.frame})
		dbg.result, dbg.err = interp.ExecuteWithContext(ctx, prog)
		dbg.exitGoRoutine(mainG)
		dbg.events(&DebugEvent{dbg, DebugExitGoRoutine, interp.frame})
		dbg.gWait.Wait()
	}()

	return dbg
}

func (dbg *Debugger) Wait() (reflect.Value, error) {
	<-dbg.context.Done()
	return dbg.result, dbg.err
}

func (dbg *Debugger) enterGoRoutine() *debugRoutine {
	g := new(debugRoutine)
	g.resume = make(chan struct{})

	dbg.gWait.Add(1)

	dbg.gLock.Lock()
	g.id = dbg.gID
	dbg.gID++
	dbg.gLive[g.id] = g
	dbg.gLock.Unlock()

	return g
}

func (dbg *Debugger) exitGoRoutine(g *debugRoutine) {
	dbg.gLock.Lock()
	delete(dbg.gLive, g.id)
	dbg.gLock.Unlock()

	dbg.gWait.Done()
}

func (dbg *Debugger) getGoRoutine(id int) (*debugRoutine, bool) {
	dbg.gLock.Lock()
	g, ok := dbg.gLive[id]
	dbg.gLock.Unlock()
	return g, ok
}

func (dbg *Debugger) enterCall(nFunc, nCall *node, f *frame) {
	if f.debug != nil {
		f.debug.g.fDepth++
		return
	}

	f.debug = new(frameDebugData)
	f.debug.g = f.anc.debug.g
	f.debug.scope = nFunc.scope

	switch nFunc.kind {
	case funcLit:
		f.debug.kind = frameCall
		if nFunc.frame != nil {
			nFunc.frame.debug.kind = frameClosure
			nFunc.frame.debug.pos = nFunc.pos
		}

	case funcDecl:
		f.debug.kind = frameCall
		f.debug.name = nFunc.child[1].ident
	}

	if nCall != nil && nCall.anc.kind == goStmt {
		f.debug.g = dbg.enterGoRoutine()
		dbg.events(&DebugEvent{dbg, DebugEnterGoRoutine, f})
	}

	f.debug.g.fDepth++
}

func (dbg *Debugger) exitCall(nFunc, nCall *node, f *frame) {
	f.debug.g.fDepth--

	if nCall != nil && nCall.anc.kind == goStmt {
		dbg.exitGoRoutine(f.debug.g)
		dbg.events(&DebugEvent{dbg, DebugExitGoRoutine, f})
	}
}

func (dbg *Debugger) exec(n *node, f *frame) {
	if n == nil {
		f.debug.pos = token.NoPos
	} else {
		f.debug.pos = n.pos
	}

	if n != nil && n.pos == token.NoPos {
		return
	}

	g := f.debug.g
	switch g.mode {
	case debugRun:
		if n == nil || !n.bkp {
			return
		}

	case DebugTerminate:
		dbg.cancel()
		return

	case DebugStepOut:
		if g.fDepth >= g.fStep {
			return
		}

	case DebugStepOver:
		if g.fDepth > g.fStep {
			return
		}
	}
	dbg.events(&DebugEvent{dbg, g.mode, f})

	select {
	case <-g.resume:
	case <-dbg.context.Done():
	}
}

func (dbg *Debugger) Continue(id int) bool {
	g, ok := dbg.getGoRoutine(id)
	if !ok {
		return false
	}

	g.mode = debugRun
	g.resume <- struct{}{}
	return true
}

func (g *debugRoutine) setMode(reason DebugEventReason) {
	if g.mode == DebugEntry && reason == DebugEntry {
		return
	}

	switch reason {
	case DebugStepInto, DebugStepOver, DebugStepOut:
		g.mode, g.fStep = reason, g.fDepth
	default:
		g.mode = DebugPause
	}
}

func (dbg *Debugger) Step(id int, reason DebugEventReason) bool {
	g, ok := dbg.getGoRoutine(id)
	if !ok {
		return false
	}

	g.setMode(reason)
	g.resume <- struct{}{}
	return true
}

func (dbg *Debugger) Interrupt(id int, reason DebugEventReason) bool {
	g, ok := dbg.getGoRoutine(id)
	if !ok {
		return false
	}

	g.setMode(reason)
	return true
}

func (dbg *Debugger) Terminate() {
	dbg.gLock.Lock()
	g := dbg.gLive
	dbg.gLive = nil
	dbg.gLock.Unlock()

	for _, g := range g {
		close(g.resume)
	}
}

func (dbg *Debugger) SetBreakpoints(path string, bp []*Breakpoint) []*Breakpoint {
	found := make([]*Breakpoint, len(bp))

	var root *node
	for _, r := range dbg.interp.roots {
		f := dbg.interp.fset.File(r.pos)
		if f != nil && f.Name() == path {
			root = r
			break
		}
	}

	if root == nil {
		return found
	}

	lines := map[int]int{}
	for i, bp := range bp {
		lines[bp.Line] = i
	}

	root.Walk(func(n *node) bool {
		if !n.pos.IsValid() {
			return true
		}

		if n.action == aNop || getExec(n) == nil {
			return true
		}

		var ok bool
		pos := dbg.interp.fset.Position(n.pos)
		i, ok := lines[pos.Line]
		if !ok || found[i] != nil {
			return true
		}

		found[i] = bp[i]
		n.bkp = true
		return true
	}, nil)

	return found
}

func (dbg *Debugger) GoRoutines() []*DebugGoRoutine {
	dbg.gLock.Lock()
	r := make([]*DebugGoRoutine, 0, len(dbg.gLive))
	for id := range dbg.gLive {
		r = append(r, &DebugGoRoutine{id})
	}
	dbg.gLock.Unlock()
	sort.Slice(r, func(i, j int) bool { return r[i].id < r[j].id })
	return r
}

func (r *DebugGoRoutine) ID() int      { return r.id }
func (r *DebugGoRoutine) Name() string { return fmt.Sprintf("Goroutine %d", r.id) }

func (evt *DebugEvent) GoRoutine() int {
	if evt.frame.debug == nil {
		return 0
	}
	return evt.frame.debug.g.id
}

func (evt *DebugEvent) Reason() DebugEventReason {
	return evt.reason
}

func (evt *DebugEvent) walkFrames(fn func([]*frame) bool) {
	if evt.frame == evt.frame.root {
		fn([]*frame{evt.frame})
		return
	}

	var g *debugRoutine
	if evt.frame.debug != nil {
		g = evt.frame.debug.g
	}

	var frames []*frame
	for f := evt.frame; f != nil && f != f.root && (f.debug == nil || f.debug.g == g); f = f.anc {
		if f.debug == nil || f.debug.kind != frameCall {
			frames = append(frames, f)
			continue
		}

		if len(frames) > 0 {
			if !fn(frames) {
				return
			}
		}

		frames = frames[:0]
		frames = append(frames, f)
	}

	if len(frames) > 0 {
		fn(frames)
	}
}

func (evt *DebugEvent) FrameDepth() int {
	if evt.frame == evt.frame.root {
		return 1
	}

	var n int
	evt.walkFrames(func([]*frame) bool { n++; return true })
	return n
}

func (evt *DebugEvent) Frames(start, end int) []*DebugFrame {
	count := end - start
	if count < 0 {
		return nil
	}

	frames := []*DebugFrame{}
	evt.walkFrames(func(f []*frame) bool {
		df := &DebugFrame{evt, make([]*frame, len(f))}
		copy(df.frames, f)
		frames = append(frames, df)
		return len(frames) < count
	})
	return frames
}

func (f *DebugFrame) Name() string {
	d := f.frames[0].debug
	if d == nil {
		return "<unknown>"
	}
	switch d.kind {
	case frameRoot:
		return "<init>"
	case frameClosure:
		return "<closure>"
	case frameCall:
		if d.name == "" {
			return "<anonymous>"
		}
		return d.name
	default:
		return "<unknown>"
	}
}

func (f *DebugFrame) Position() token.Position {
	if f.frames[0].debug == nil {
		return token.Position{}
	}
	return f.event.debugger.interp.fset.Position(f.frames[0].debug.pos)
}

func (f *DebugFrame) Scopes() []*DebugFrameScope {
	s := make([]*DebugFrameScope, len(f.frames))
	for i, f := range f.frames {
		s[i] = &DebugFrameScope{f}
	}
	return s
}

func (f *DebugFrameScope) IsClosure() bool {
	return f.frame.debug != nil && f.frame.debug.kind == frameClosure
}

func (f *DebugFrameScope) Variables() []*DebugVariable {
	d := f.frame.debug
	if d == nil || d.scope == nil {
		return nil
	}

	index := map[int]string{}
	scanScope(d.scope, index)

	m := make([]*DebugVariable, 0, len(f.frame.data))
	for i, v := range f.frame.data {
		if typ := v.Type(); typ.AssignableTo(rNodeType) || typ.Kind() == reflect.Ptr && typ.Elem().AssignableTo(rNodeType) {
			continue
		}
		name, ok := index[i]
		if !ok {
			continue
		}

		m = append(m, &DebugVariable{name, v})
	}
	return m
}

func scanScope(sc *scope, index map[int]string) {
	for name, sym := range sc.sym {
		if _, ok := index[sym.index]; ok {
			continue
		}
		index[sym.index] = name
	}

	for _, ch := range sc.child {
		if ch.def != sc.def {
			continue
		}
		scanScope(ch, index)
	}
}
