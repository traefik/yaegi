package dbg

import (
	"sync"

	"github.com/traefik/yaegi/interp"
)

type events struct {
	mu     *sync.Mutex
	values map[int]*interp.DebugEvent
}

func newEvents() *events {
	e := new(events)
	e.mu = new(sync.Mutex)
	e.values = map[int]*interp.DebugEvent{}
	return e
}

func (t *events) Get(id int) (*interp.DebugEvent, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.values[id]
	return e, ok
}

func (t *events) Retain(e *interp.DebugEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.values[e.GoRoutine()] = e
}

func (t *events) Release(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.values, id)
}

type frames struct {
	mu     *sync.Mutex
	values []*interp.DebugFrame
	id     int
}

func newFrames() *frames {
	f := new(frames)
	f.mu = new(sync.Mutex)
	return f
}

func (r *frames) Purge() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.id = 0
	if r.values != nil {
		r.values = r.values[:0]
	}
}

func (r *frames) Add(v *interp.DebugFrame) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.id++
	r.values = append(r.values, v)
	return r.id
}

func (r *frames) Get(i int) (*interp.DebugFrame, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if i < 1 || i > len(r.values) {
		return nil, false
	}
	return r.values[i-1], true
}
