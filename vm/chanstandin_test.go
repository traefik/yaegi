package vm

import (
	"go/types"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"
)

// TestChanStandIn checks the stand-in of an interpreted channel: one per
// channel and type, of the same capacity and direction, closed, it gives
// back the channel, and the tables forget both once nothing uses them.
func TestChanStandIn(t *testing.T) {
	m, _ := buildSrc(t, "package p\n")
	ct := types.NewChan(types.SendRecv, types.Typ[types.Int])
	rct := types.NewChan(types.RecvOnly, types.Typ[types.Int])
	ch := make(chan Value, 2)

	a := m.standIn(ch, ct, m.reflectType(ct))
	if b := m.standIn(ch, ct, m.reflectType(ct)); a.Pointer() != b.Pointer() {
		t.Errorf("two stand-ins for the same channel and type")
	}
	if a.Type() != reflect.TypeFor[chan int]() || a.Cap() != 2 {
		t.Errorf("stand-in is %s with cap %d, want chan int with cap 2", a.Type(), a.Cap())
	}
	if _, ok := a.Recv(); ok {
		t.Errorf("stand-in is not closed")
	}
	if s := m.standInOf(a); s == nil || s.ch != ch {
		t.Errorf("standInOf does not give back the channel")
	}
	r := m.standIn(ch, rct, m.reflectType(rct))
	if r.Type() != reflect.TypeFor[<-chan int]() {
		t.Errorf("receive-only stand-in is %s", r.Type())
	}
	if iv, ok := m.standInIface(r); !ok || iv.V.R != any(ch) || !types.Identical(iv.T.T, rct) {
		t.Errorf("standInIface does not give back the channel with its type")
	}
	if m.standInOf(reflect.ValueOf(make(chan int))) != nil {
		t.Errorf("a native channel is seen as a stand-in")
	}

	a, r, ch = reflect.Value{}, reflect.Value{}, nil
	count := func(sm *sync.Map) (n int) {
		sm.Range(func(any, any) bool { n++; return true })
		return n
	}
	for i := 0; i < 100 && count(m.standInChans)+count(m.chanStandIns) > 0; i++ {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if n, k := count(m.standInChans), count(m.chanStandIns); n+k > 0 {
		t.Errorf("after GC: %d stand-ins and %d channel entries left, want 0", n, k)
	}
}

// TestChanStandInMachineFreed checks that a stand-in does not keep its
// Machine alive. The cleanups of the stand-in tables must not point to the
// Machine: a cleanup is a GC root while its object lives, and here the
// objects (a global channel, a stand-in in a global sync.Map) live as long
// as the Machine. Traefik builds a new Machine on each plugin reload.
func TestChanStandInMachineFreed(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// control: the stand-in dies with F, so the Machine is free
		{"local chan to json", `package p
import "encoding/json"
type S struct{ C chan int }
func F() { json.Marshal(S{make(chan int)}) }
`},
		{"global chan to json", `package p
import "encoding/json"
type S struct{ C chan int }
var c = make(chan int)
func F() { json.Marshal(S{c}) }
`},
		{"stand-in in a global sync.Map", `package p
import "sync"
type S struct{ C chan int }
var sm sync.Map
func F() { sm.Store("k", S{make(chan int)}) }
`},
		// the cleanup argument held the *RType of the channel type, and
		// its methods lead to the Machine
		{"named chan type with a method", `package p
import "encoding/json"
type C chan int
func (C) M() {}
type S struct{ X C }
var c = make(C)
func F() { json.Marshal(S{c}) }
`},
		// the stand-in record held the channel, and its buffered func
		// leads to the Machine
		{"buffered func in a stand-in", `package p
import "sync"
type S struct{ C chan func() }
var sm sync.Map
func G() {}
func F() { c := make(chan func(), 1); c <- G; sm.Store("k", S{c}) }
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freed := make(chan struct{})
			func() {
				m, spkg := buildSrc(t, tc.src)
				if init := spkg.Func("init"); init != nil {
					m.Call(m.funcOf(init), nil, nil)
				}
				m.Call(m.funcOf(spkg.Func("F")), nil, nil)
				runtime.AddCleanup(m, func(ch chan struct{}) { close(ch) }, freed)
			}()
			for i := 0; i < 200; i++ {
				runtime.GC()
				select {
				case <-freed:
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
			t.Errorf("the Machine is never collected")
		})
	}
}

// TestChanReaderFuncReuse checks that the mark of chanReaderFunc follows
// the wrapper, not its address. A cleanup runs after its object is freed,
// so a new func can get the address of a dead wrapper before the cleanup
// of the wrapper runs: it must not be marked, and the cleanup must not
// remove the mark of a new wrapper at that address. The test stalls the
// cleanups (as a slow cleanup of the host would) to keep that window open.
func TestChanReaderFuncReuse(t *testing.T) {
	const n = 2000
	tt := reflect.TypeFor[*template.Template]()
	tmpl := template.Must(template.New("x").Parse("x"))
	mk := func() reflect.Value {
		return chanReaderFunc(tt, "Execute", reflect.ValueOf(tmpl).MethodByName("Execute"))
	}

	// Stall the cleanup goroutines.
	release := make(chan struct{})
	var stalled sync.WaitGroup
	for i := 0; i < 8; i++ {
		stalled.Add(1)
		runtime.AddCleanup(new([64]byte), func(int) { stalled.Done(); <-release }, i)
	}
	released := false
	unstall := func() {
		if !released {
			released = true
			close(release)
		}
	}
	defer unstall()
	done := make(chan struct{})
	go func() { stalled.Wait(); close(done) }()
	for i := 0; i < 100; i++ {
		runtime.GC()
		select {
		case <-done:
			i = 100
		case <-time.After(5 * time.Millisecond):
		}
	}

	func() {
		ws := make([]reflect.Value, n)
		for i := range ws {
			ws[i] = mk()
		}
	}()
	runtime.GC()
	runtime.GC()

	upper := reflect.ValueOf(strings.ToUpper)
	others := make([]reflect.Value, n)
	marked := 0
	for i := range others {
		others[i] = reflect.MakeFunc(upper.Type(), upper.Call)
		if isChanReaderFunc(others[i]) {
			marked++
		}
	}
	if marked > 0 {
		t.Errorf("%d of %d unrelated funcs are marked as chan readers", marked, n)
	}
	others = nil
	runtime.GC()
	runtime.GC()

	live := make([]reflect.Value, n)
	for i := range live {
		live[i] = mk()
	}
	unstall()
	for i := 0; i < 20; i++ {
		runtime.GC()
		time.Sleep(5 * time.Millisecond)
	}
	lost := 0
	for _, w := range live {
		if !isChanReaderFunc(w) {
			lost++
		}
	}
	if lost > 0 {
		t.Errorf("%d of %d live wrappers lost their mark", lost, n)
	}
	runtime.KeepAlive(live)
}
