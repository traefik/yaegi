package interp

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
)

type Debugger struct {
	fset    *token.FileSet
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
	event *DebugEvent
	frame *frame
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

func (interp *Interpreter) Debug(ctx context.Context, prog *Program, events func(*DebugEvent), opts *DebugOptions) *Debugger {
	dbg := new(Debugger)
	dbg.fset = interp.fset
	dbg.events = events
	dbg.context, dbg.cancel = context.WithCancel(ctx)
	dbg.resume = make(chan struct{})

	interp.frame.dbg = dbg

	if opts == nil {
		opts = new(DebugOptions)
	}

	go func() {
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
	if f == f.root || f.name != "" {
		return
	}

	switch n.kind {
	case funcLit:
		f.name = "anonymous"
	case funcDecl:
		f.name = n.child[1].ident
	}
}

func (dbg *Debugger) exitCall(n *node, f *frame) {
	dbg.fDepth--
}

func (dbg *Debugger) exec(n *node, f *frame) {
	f.pos = n.pos
	if f.pos == token.NoPos {
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

func (evt *DebugEvent) Reason() DebugStopReason {
	return evt.reason
}

func (evt *DebugEvent) FrameDepth() int {
	if evt.frame == evt.frame.root {
		return 1
	}

	var n int
	for f := evt.frame; f != nil && f != f.root; f = f.anc {
		n++
	}
	return n
}

func (evt *DebugEvent) Frames(start, end int) []*DebugFrame {
	count := end - start
	if count < 0 {
		return nil
	}

	if evt.frame == evt.frame.root && start == 0 && end > 0 {
		return []*DebugFrame{{evt, evt.frame}}
	}

	f := evt.frame
	for start > 0 && f != nil {
		f = f.anc
		start--
	}
	if f == nil {
		return nil
	}

	frames := make([]*DebugFrame, 0, count)
	for f := evt.frame; f != nil && f != f.root && len(frames) < count; f = f.anc {
		frames = append(frames, &DebugFrame{evt, f})
	}
	return frames
}

func (evt *DebugEvent) Frame(n int) *DebugFrame {
	f := evt.frame
	for f != nil && n > 0 {
		f = f.anc
		n--
	}
	if f == nil {
		return nil
	}
	return &DebugFrame{evt, f}
}

func (f *DebugFrame) Name() string {
	if f.frame == f.frame.root {
		return "init"
	}
	return f.frame.name
}

func (f *DebugFrame) Position() token.Position {
	return f.event.debugger.fset.Position(f.frame.pos)
}

func (f *DebugFrame) Variables() map[string]reflect.Value {
	m := make(map[string]reflect.Value, len(f.frame.data))
	for i, v := range f.frame.data {
		m[fmt.Sprint(i)] = v
	}
	return m
}
