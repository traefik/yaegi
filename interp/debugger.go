package interp

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
)

type Debugger struct {
	interp  *Interpreter
	events  func(*DebugEvent)
	context context.Context
	cancel  context.CancelFunc

	mode   DebugStopReason
	resume chan struct{}

	fDepth int
	fStep  int

	result reflect.Value
	err    error
}

type DebugOptions struct {
}

type DebugEvent struct {
	debugger  *Debugger
	reason    DebugStopReason
	statement *node
	frame     *frame
}

type DebugFrame struct {
	event  *DebugEvent
	frames []*frame
}

type DebugFrameScope struct {
	frame *frame
}

type Breakpoint struct {
	Line, Column int
}

type DebugStopReason int

const (
	debugRun DebugStopReason = iota
	DebugPause
	DebugBreak
	DebugEntry
	DebugStepInto
	DebugStepOver
	DebugStepOut
	DebugTerminate
)

type frameDebugData struct {
	pos  token.Pos
	name string
	kind frameKind
}

type frameKind int

const (
	frameUnknown frameKind = iota
	frameRoot
	frameCall
	frameClosure
)

func (interp *Interpreter) Debug(ctx context.Context, prog *Program, events func(*DebugEvent), opts *DebugOptions) *Debugger {
	dbg := new(Debugger)
	dbg.interp = interp
	dbg.events = events
	dbg.context, dbg.cancel = context.WithCancel(ctx)
	dbg.resume = make(chan struct{})

	interp.debugger = dbg

	if opts == nil {
		opts = new(DebugOptions)
	}

	go func() {
		defer func() { interp.debugger = nil }()
		defer events(&DebugEvent{reason: DebugTerminate})
		defer dbg.cancel()

		dbg.mode = DebugEntry
		<-dbg.resume

		dbg.result, dbg.err = interp.ExecuteWithContext(ctx, prog)
	}()

	return dbg
}

func (dbg *Debugger) Wait() (reflect.Value, error) {
	<-dbg.context.Done()
	return dbg.result, dbg.err
}

func (dbg *Debugger) enterCall(n *node, f *frame) {
	dbg.fDepth++

	if f.debug != nil {
		return
	}

	f.debug = new(frameDebugData)
	if f == f.root {
		f.debug.kind = frameRoot
		return
	}

	f.debug.kind = frameCall
	switch n.kind {
	case funcLit:
		if n.frame != nil && n.frame.debug == nil {
			n.frame.debug = new(frameDebugData)
			n.frame.debug.kind = frameClosure
			n.frame.debug.pos = n.pos
		}
	case funcDecl:
		f.debug.name = n.child[1].ident
	}
}

func (dbg *Debugger) exitCall(n *node, f *frame) {
	dbg.fDepth--
}

func (dbg *Debugger) exec(n *node, f *frame) {
	if f.debug == nil {
		f.debug = new(frameDebugData)
	}
	f.debug.pos = n.pos
	if f.debug.pos == token.NoPos {
		return
	}

	switch dbg.mode {
	case debugRun:
		if !n.bkp {
			return
		}

	case DebugTerminate:
		dbg.cancel()
		return

	case DebugStepOut:
		if dbg.fDepth >= dbg.fStep {
			return
		}

	case DebugStepOver:
		if dbg.fDepth > dbg.fStep {
			return
		}
	}
	dbg.events(&DebugEvent{dbg, dbg.mode, n, f})

	select {
	case <-dbg.resume:
	case <-dbg.context.Done():
	}
}

func (dbg *Debugger) Continue() {
	dbg.mode = debugRun
	dbg.resume <- struct{}{}
}

func (dbg *Debugger) Step(reason DebugStopReason) {
	if dbg.mode != DebugEntry || reason != DebugEntry {
		dbg.Interrupt(reason)
	}

	dbg.resume <- struct{}{}
}

func (dbg *Debugger) Interrupt(reason DebugStopReason) {
	switch reason {
	case DebugStepInto, DebugStepOver, DebugStepOut, DebugTerminate:
		dbg.mode, dbg.fStep = reason, dbg.fDepth
	default:
		dbg.mode = DebugPause
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

	claimed := map[*node]bool{}
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
		claimed[n.start] = true
		return true
	}, func(n *node) {
		n.bkp = claimed[n]
	})

	return found
}

func (evt *DebugEvent) Reason() DebugStopReason {
	return evt.reason
}

func (evt *DebugEvent) walkFrames(fn func([]*frame) bool) {
	if evt.frame == evt.frame.root {
		fn([]*frame{evt.frame})
		return
	}

	var frames []*frame
	for f := evt.frame; f != nil && f != f.root; f = f.anc {
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

func (evt *DebugEvent) Frame(n int) *DebugFrame {
	var df *DebugFrame
	evt.walkFrames(func(f []*frame) bool {
		if n > 0 {
			n--
			return true
		}

		df = &DebugFrame{evt, f}
		return false
	})
	return df
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

func (f *DebugFrameScope) Variables() map[string]reflect.Value {
	// f.event.debugger.interp.scopes
	m := make(map[string]reflect.Value, len(f.frame.data))
	for i, v := range f.frame.data {
		m[fmt.Sprint(i)] = v
	}
	return m
}
