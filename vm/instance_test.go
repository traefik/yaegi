package vm

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// instancePlugin starts goroutines in the ways that instance.go lists: a
// go statement, a goroutine started by another one, a callback of
// time.AfterFunc that starts a goroutine, and an iter.Pull sequence.
const instancePlugin = `package p

import (
	"iter"
	"time"
)

var next func() (int, bool)

func Start() {
	go func() {
		go func() { select {} }() // a child: same instance
		select {}
	}()
	time.AfterFunc(time.Millisecond, func() {
		go func() { select {} }() // started by a callback of native code
	})
	seq := func(yield func(int) bool) {
		for i := 0; yield(i); i++ {
		}
	}
	next, _ = iter.Pull(iter.Seq[int](seq))
	next()
}

func Spawn(n int) {
	for i := 0; i < n; i++ {
		go func() { select {} }()
	}
}
`

// TestInstanceGoroutines checks which goroutines belong to an Instance: the
// ones that a call with its context starts, their children, the goroutines
// that a callback of native code (time.AfterFunc) of the call starts, and
// an iter.Pull sequence. MaxGoroutines bounds each instance and the
// goroutines of no instance apart. Instance.Close stops the goroutines of
// its instance only, and Plugin.Close the ones of all instances.
func TestInstanceGoroutines(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, instancePlugin)
	p, err := Load(Options{Dir: dir, Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 5}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	a, b := p.NewInstance(), p.NewInstance()
	if _, err := p.Call(a.Context(context.Background()), "Start"); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return a.Goroutines() == 4 }) {
		t.Fatalf("instance a: %d goroutines, want 4", a.Goroutines())
	}
	// The cap of 5 is per instance: b and the calls of no instance get 5
	// each, whatever a holds.
	if _, err := p.Call(b.Context(context.Background()), "Spawn", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(context.Background(), "Spawn", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(b.Context(context.Background()), "Spawn", 1); !errors.Is(err, ErrTooManyGoroutines) {
		t.Fatalf("a sixth goroutine in instance b: %v, want %v", err, ErrTooManyGoroutines)
	}
	if _, err := p.Call(context.Background(), "Spawn", 1); !errors.Is(err, ErrTooManyGoroutines) {
		t.Fatalf("a sixth goroutine of no instance: %v, want %v", err, ErrTooManyGoroutines)
	}
	if g := p.M.Goroutines(); g != 14 {
		t.Fatalf("plugin: %d goroutines, want 14", g)
	}
	a.Close()
	if !waitFor(2*time.Second, func() bool { return a.Goroutines() == 0 }) {
		t.Fatalf("after Close of instance a: %d goroutines of it", a.Goroutines())
	}
	if _, err := p.Call(a.Context(context.Background()), "Spawn", 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("a call in the closed instance a: %v, want %v", err, ErrClosed)
	}
	if b.Goroutines() != 5 || p.M.Goroutines() != 10 {
		t.Fatalf("after Close of instance a: instance b %d, plugin %d goroutines, want 5 and 10", b.Goroutines(), p.M.Goroutines())
	}
	p.Close()
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after Plugin.Close: %d goroutines (instance b: %d)", p.M.Goroutines(), b.Goroutines())
	}
}

// janitorPlugin starts a goroutine that calls time.AfterFunc after Start
// returned, as a cache janitor started in New does.
const janitorPlugin = `package p

import "time"

var sink int

func Start(n int) {
	go func() {
		time.Sleep(20 * time.Millisecond) // Start returned: a background goroutine
		for i := 0; i < n; i++ {
			for j := 0; j < 100000; j++ { // steps: refills in the background
				sink += j
			}
			time.AfterFunc(time.Millisecond, func() { select {} })
		}
		select {}
	}()
}
`

// TestInstanceBackgroundAfterFunc checks that a callback of time.AfterFunc
// that a background goroutine of an instance starts, after the call from
// the host returned, belongs to the instance: it counts in the cap of the
// instance and not in the cap of no instance, and Instance.Close stops it.
// Before, the background goroutine had no account, so the callback had no
// instance.
func TestInstanceBackgroundAfterFunc(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, janitorPlugin)
	p, err := Load(Options{Dir: dir, Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 5}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	inst := p.NewInstance()
	if _, err := p.Call(inst.Context(context.Background()), "Start", 3); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return inst.Goroutines() == 4 }) {
		t.Fatalf("instance: %d goroutines, plugin %d, want 4 and 4", inst.Goroutines(), p.M.Goroutines())
	}
	if g := p.M.Goroutines(); g != 4 {
		t.Fatalf("plugin: %d goroutines, want 4 (all in the instance)", g)
	}
	inst.Close()
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after Close of the instance: %d goroutines of it, %d of the plugin", inst.Goroutines(), p.M.Goroutines())
	}
}

// finalizerPlugin sets a finalizer and a cleanup that start a goroutine.
const finalizerPlugin = `package p

import "runtime"

func Start() {
	x := new([16]int)
	runtime.SetFinalizer(x, func(*[16]int) {
		go func() { select {} }()
	})
	y := new([16]int)
	runtime.AddCleanup(y, func(int) {
		go func() { select {} }()
	}, 0)
}
`

// TestInstanceFinalizer checks that a finalizer and a cleanup run in the
// instance of the call that set them: the goroutines that they start
// belong to the instance, and Instance.Close stops them.
func TestInstanceFinalizer(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, finalizerPlugin)
	p, err := Load(Options{Dir: dir, Budget: 1e8, BackgroundRate: 5e7, MaxGoroutines: 5}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	inst := p.NewInstance()
	if _, err := p.Call(inst.Context(context.Background()), "Start"); err != nil {
		t.Fatal(err)
	}
	if !waitFor(5*time.Second, func() bool { runtime.GC(); return p.M.Goroutines() == 2 }) {
		t.Fatalf("plugin: %d goroutines, want 2", p.M.Goroutines())
	}
	if g := inst.Goroutines(); g != 2 {
		t.Fatalf("instance: %d goroutines, want 2 (the ones of the finalizer and the cleanup)", g)
	}
	inst.Close()
	if !waitFor(2*time.Second, func() bool { return p.M.Goroutines() == 0 }) {
		t.Fatalf("after Close of the instance: %d goroutines of it, %d of the plugin", inst.Goroutines(), p.M.Goroutines())
	}
}

// spinPlugin starts a goroutine that loops with no blocking point. The
// loop counts in its own variable: a shared global would be a data race
// between the goroutines of two instances, which -race reports.
const spinPlugin = `package p

func Spin() {
	go func(n int) {
		for {
			n++
		}
	}(0)
}

func Spawn(n int) {
	for i := 0; i < n; i++ {
		go func() { select {} }()
	}
}
`

// TestInstanceOtherMachine checks that the Instance of a machine does not
// apply to a call into another machine with its context: Traefik loads
// each plugin in its own machine, and a host may pass the context of New
// of one plugin to another one. The goroutines of that call belong to no
// instance of the other machine, and closing the instance does not stop
// them.
func TestInstanceOtherMachine(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, spinPlugin)
	p1, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p1.Close()
	p2, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()
	p2.NewInstance() // p2 uses instances too (instanceOf looks at the context)
	i1 := p1.NewInstance()
	if _, err := p2.Call(i1.Context(context.Background()), "Spawn", 2); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return p2.M.Goroutines() == 2 }) {
		t.Fatalf("plugin 2: %d goroutines, want 2", p2.M.Goroutines())
	}
	if g := i1.Goroutines(); g != 0 {
		t.Fatalf("instance of plugin 1: %d goroutines of plugin 2, want 0", g)
	}
	i1.Close()
	time.Sleep(20 * time.Millisecond)
	if g := p2.M.Goroutines(); g != 2 {
		t.Fatalf("after Close of an instance of plugin 1: plugin 2 has %d goroutines, want 2", g)
	}
	if _, err := p2.Call(i1.Context(context.Background()), "Spawn", 1); err != nil {
		t.Fatalf("a call of plugin 2 with the context of a closed instance of plugin 1: %v", err)
	}
}

// TestInstanceCloseSpin checks that Instance.Close stops a goroutine of the
// instance that loops with no blocking point, at its next refill of steps
// (BackgroundRate), while the machine stays open: the refill checks the
// closed flag of the instance, not only the one of the machine.
func TestInstanceCloseSpin(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, spinPlugin)
	p, err := Load(Options{Dir: dir, BackgroundRate: 5e7}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	a, b := p.NewInstance(), p.NewInstance()
	for _, i := range []*Instance{a, b} {
		if _, err := p.Call(i.Context(context.Background()), "Spin"); err != nil {
			t.Fatal(err)
		}
	}
	if !waitFor(2*time.Second, func() bool { return a.Goroutines() == 1 && b.Goroutines() == 1 }) {
		t.Fatalf("instances: %d and %d goroutines, want 1 each", a.Goroutines(), b.Goroutines())
	}
	a.Close()
	if !waitFor(5*time.Second, func() bool { return a.Goroutines() == 0 }) {
		t.Fatalf("after Close of instance a: its spinning goroutine still runs")
	}
	if p.M.Closed() || b.Goroutines() != 1 {
		t.Fatalf("after Close of instance a: machine closed %v, instance b %d goroutines, want open and 1", p.M.Closed(), b.Goroutines())
	}
}

// TestInstanceStopChanMachineClosed checks the first wait of an instance
// after Machine.Close: Close walked the instances before the wait made
// the channel, so stopChan must see the closed machine and give a closed
// channel. Else a goroutine of the instance that waits for the first time
// then (a select on a channel that nobody sends to) waits for ever.
func TestInstanceStopChanMachineClosed(t *testing.T) {
	needSource(t)
	dir, pkg := writePlugin(t, spinPlugin)
	p, err := Load(Options{Dir: dir}, pkg)
	if err != nil {
		t.Fatal(err)
	}
	i := p.NewInstance()
	p.Close()
	select {
	case <-i.stopChan():
	default:
		t.Fatal("stopChan of an instance after Machine.Close: not closed")
	}
	if i.Closed() {
		t.Error("Machine.Close marked the instance as closed by Instance.Close")
	}
}
