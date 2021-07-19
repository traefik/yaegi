package interp

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
)

type Debugger struct {
	events  func(*DebugEvent)
	context context.Context
	cancel  context.CancelFunc

	mode   DebugStopReason
	resume chan struct{}

	result reflect.Value
	err    error
}

type DebugOptions struct {
}

type DebugEvent struct {
	reason    DebugStopReason
	statement *node
	frame     *frame
}

type DebugFrame struct {
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
	dbg.events = events
	dbg.context, dbg.cancel = context.WithCancel(ctx)
	dbg.resume = make(chan struct{})

	interp.frame.debug = dbg.run

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

func (dbg *Debugger) run(n *node, f *frame) {
	switch dbg.mode {
	case debugRun:
		if !n.bkp {
			return
		}

	case DebugTerminate:
		dbg.cancel()
		return

	default:
		dbg.events(&DebugEvent{dbg.mode, n, f})
	}

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
	dbg.Interrupt(reason)
	dbg.resume <- struct{}{}
}

func (dbg *Debugger) Interrupt(reason DebugStopReason) {
	if dbg.mode == DebugEntry && reason == DebugEntry {
		return
	}

	switch reason {
	case DebugStepInto, DebugStepOver, DebugStepOut, DebugTerminate:
		dbg.mode = reason
	default:
		dbg.mode = DebugPause
	}
}

func (evt *DebugEvent) Reason() DebugStopReason {
	return evt.reason
}

func (evt *DebugEvent) FrameDepth() int {
	var n int
	for f := evt.frame; f != nil; f = f.anc {
		n++
	}
	return n
}

func (evt *DebugEvent) Frames(start, end int) []*DebugFrame {
	count := end - start
	if count < 0 {
		return nil
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
	for f := evt.frame; f != nil && len(frames) < count; f = f.anc {
		frames = append(frames, &DebugFrame{f})
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
	return &DebugFrame{f}
}

func (f *DebugFrame) Position() token.Position {
	return token.Position{}
}

func (f *DebugFrame) Variables() map[string]reflect.Value {
	m := make(map[string]reflect.Value, len(f.frame.data))
	for i, v := range f.frame.data {
		m[fmt.Sprint(i)] = v
	}
	return m
}
