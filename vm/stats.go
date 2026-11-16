package vm

// The counters of a Machine for the metrics of the host (Machine.Stats).
//
// A host such as Traefik keeps one Plugin per plugin for the life of the
// process (the reload model of docs/RELOAD.md), so these counters are the
// only view it has of a plugin that goes wrong weeks after its load: a loop
// that the budget stops at each request, a panic per request, a background
// goroutine that the BackgroundRate slows down. Each counter is an atomic
// add on a path that is not hot: once per call from the host (Requests,
// Calls), once per stop or panic, once per chunk of steps (Steps, at most
// once per minChunk to maxChunk steps), once per sleep of the
// BackgroundRate. The step loop itself does not change.

import (
	"errors"
	"sync/atomic"
	"time"
)

// Stats is a snapshot of the counters of a Machine (Machine.Stats). The
// counters only grow, from the load of the plugin, except Goroutines and
// Memory, which are the current values. A host exports them as metrics
// labelled by plugin (a counter for each growing field, a gauge for the
// others) and polls them: see the doc of Machine.Stats.
type Stats struct {
	// Requests is the number of ServeHTTP calls of the handlers of the
	// plugin (the http.Handler that Object.Interface returns).
	Requests int64
	// Calls is the number of Plugin.Call and Object.Call calls that ran
	// plugin code (CreateConfig, New, the methods of a provider).
	Calls int64
	// Panics is the number of panics of the plugin that ended a call from
	// the host (ServeHTTP, Plugin.Call, Object.Call) or went to GoPanic (a
	// goroutine of the plugin, a callback on a goroutine of native code).
	// The stops below are not in it, nor a canceled call (ErrCanceled,
	// ErrClosed, http.ErrAbortHandler) or a runtime.Goexit.
	Panics int64
	// BudgetStops, MemoryStops and StackStops count the same calls and
	// goroutines as Panics, stopped by ErrBudget, ErrMemory and
	// ErrStackOverflow. BudgetStops is not Machine.BudgetStops, which counts
	// the ServeHTTP calls only.
	BudgetStops int64
	MemoryStops int64
	StackStops  int64
	// LostMethodErrors is the number of *LostMethodError results: calls of
	// encoding/json or encoding/xml that would ignore a method of a type of
	// the plugin in default mode, refused at run time (lostmethods.go).
	// Never more than 0 in AOT mode with fresh copies.
	LostMethodErrors int64
	// Steps is the number of steps the plugin used, all goroutines
	// together, counted in chunks (a goroutine that runs counts its chunk
	// when it takes it; the unused part of a chunk is taken back when the
	// goroutine ends). It is 0 without a Budget and a BackgroundRate. A loop
	// runs about 4e8 steps per second on one core: the rate of Steps over
	// time is a rough estimate of the CPU of the plugin (native code, a
	// network read, a regexp, counts less than its CPU: see the weights in
	// budget.go).
	Steps int64
	// BackgroundWait is the time that the background goroutines of the
	// plugin slept because they used their BackgroundRate: a plugin whose
	// background work is slowed down.
	BackgroundWait time.Duration
	// Goroutines is Machine.Goroutines: the live goroutines of the plugin.
	Goroutines int
	// MissingEnv is the number of names of Machine.MissingEnv: variables
	// the plugin read and the host did not pass.
	MissingEnv int
	// Memory is Machine.Memory: the bytes that the memory quota counts.
	// Today the quota only counts the allocations of 64 KiB or more: it is
	// 0 for most plugins, and it is not the heap of the plugin.
	Memory int64
}

// machineStats are the counters of Stats in the Machine.
type machineStats struct {
	requests, calls                     atomic.Int64
	panics, budget, memory, stack, lost atomic.Int64
	steps                               atomic.Int64
	bgWait                              atomic.Int64 // ns
}

// Stats returns the counters of the plugin, for the metrics of the host.
// It is safe to call at any time, from any goroutine, also during calls;
// it does not stop them. Every field but Memory is a few atomic loads;
// Memory takes the lock of the memory quota.
//
// A host polls it (every 10 to 60 seconds, or when its metrics are
// scraped) and exports the fields as metrics labelled by plugin. This is
// the way to see a plugin that goes wrong long after its load: with
// Options.QuietBudget, GoPanic gets the first budget stop of the machine,
// then at most one report per BudgetReportInterval (*BudgetStopsError);
// the deltas of BudgetStops, Panics and Steps between two polls show the
// rest.
func (m *Machine) Stats() Stats {
	s := &m.stats
	n := 0
	if e := m.env; e != nil {
		e.repMu.Lock()
		n = len(e.names)
		e.repMu.Unlock()
	}
	return Stats{
		Requests:         s.requests.Load(),
		Calls:            s.calls.Load(),
		Panics:           s.panics.Load(),
		BudgetStops:      s.budget.Load(),
		MemoryStops:      s.memory.Load(),
		StackStops:       s.stack.Load(),
		LostMethodErrors: s.lost.Load(),
		Steps:            s.steps.Load(),
		BackgroundWait:   time.Duration(s.bgWait.Load()),
		Goroutines:       m.Goroutines(),
		MissingEnv:       n,
		Memory:           m.Memory(),
	}
}

// countFault counts the panic value r that ended a call from the host or a
// goroutine of the plugin (see Stats.Panics). nil, a stop that is not a
// fault of the plugin and a runtime.Goexit are not counted, nor the
// markers of a callback whose panic was already counted.
func (m *Machine) countFault(r any) {
	if r == nil || r == errGoexit || quietStop(r) || r == errAbortCallback || r == errCallbackPanic {
		return
	}
	s := &m.stats
	err, _ := r.(error)
	switch {
	case err == nil:
		s.panics.Add(1)
	case errors.Is(err, ErrCanceled) || errors.Is(err, ErrClosed):
	case errors.Is(err, ErrBudget):
		s.budget.Add(1)
	case errors.Is(err, ErrMemory):
		s.memory.Add(1)
	case errors.Is(err, ErrStackOverflow):
		s.stack.Add(1)
	default:
		s.panics.Add(1)
	}
}

// goPanic gives r, the panic of a goroutine of the plugin or of a callback,
// to GoPanic, and counts it (Stats).
func (m *Machine) goPanic(r any) {
	m.countFault(r)
	m.GoPanic(r)
}
