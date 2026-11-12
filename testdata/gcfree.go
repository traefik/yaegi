package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// Objects that the program cannot reach any more must be collected by
// runtime.GC (see also gcdead.go). Each function below checks one place
// where the VM used to keep a dead object:
//   - the argument registers of a caller that waits for an interpreted
//     callee (GOROOT tests stackobj.go, stackobj3.go);
//   - the frame that runs its defers, on return or during a panic
//     (fixedbugs/issue32477.go);
//   - runtime.KeepAlive, which made a native copy, and the zero template of
//     a large array (fixedbugs/issue15277.go);
//   - a finalizer set on a *byte from unsafe.StringData
//     (fixedbugs/issue57823.go).

type T struct{ name string }

var done = make(chan string, 10)

func newT(name string) *T {
	t := &T{name: name}
	runtime.SetFinalizer(t, func(t *T) { done <- t.name })
	return t
}

// wait runs the GC until a finalizer reports, or gives up.
func wait() string {
	for i := 0; i < 50; i++ {
		runtime.GC()
		select {
		case s := <-done:
			return s
		case <-time.After(20 * time.Millisecond):
		}
	}
	return "not collected"
}

//go:noinline
func use(t *T) {
	fmt.Println("using", t.name)
	// t is dead here, in this frame and in the frame of the caller.
	fmt.Println(wait())
}

func callArgs() {
	t := newT("argument of a call")
	use(t)
}

func deferReturn() {
	t := newT("frame running its defers")
	fmt.Println("using", t.name)
	defer func() { fmt.Println(wait()) }()
}

func deferPanic() {
	t := newT("panicking frame")
	fmt.Println("using", t.name)
	defer func() {
		fmt.Println(wait())
		fmt.Println("recovered:", recover())
	}()
	panic("boom")
}

type big [1 << 20]byte

func inuse() int64 {
	runtime.GC()
	var st runtime.MemStats
	runtime.ReadMemStats(&st)
	return int64(st.Alloc)
}

//go:noinline
func drop(x *big, start int64) {
	x[0] = 1
	runtime.KeepAlive(x)
	x = nil
	fmt.Println("large array freed:", inuse()-start < 1<<19)
}

func keepAlive() {
	start := inuse()
	drop(new(big), start)
}

func stringData() {
	s := fmt.Sprint("AAAAAAAA") + "BBBBBBBBB"
	p := unsafe.StringData(s)
	runtime.SetFinalizer(p, func(*byte) { done <- "string bytes" })
	fmt.Println("first byte:", string(rune(*p)))
	fmt.Println(wait())
}

// A deferred call of a //go:uintptrescapes function keeps the objects of
// its uintptr arguments alive until it runs, also through a method value
// wrapper (fixedbugs/issue24491a.go).
type S struct{}

//go:noinline
//go:uintptrescapes
func (S) check(p uintptr) {
	runtime.GC()
	runtime.GC()
	fmt.Println("deferred:", *(*string)(unsafe.Pointer(p)))
}

func newStr() unsafe.Pointer {
	s := "alive"
	runtime.SetFinalizer(&s, func(p *string) { *p = "collected too early" })
	return unsafe.Pointer(&s)
}

func deferUintptr() {
	s := &S{}
	defer s.check(uintptr(newStr()))
}

// kept runs the GC a few times and reports whether a finalizer ran.
func kept() string {
	for i := 0; i < 5; i++ {
		runtime.GC()
		select {
		case s := <-done:
			return "freed too early: " + s
		case <-time.After(10 * time.Millisecond):
		}
	}
	return "live object kept"
}

// A deferred call that runs during runtime.Goexit or during a panic that is
// recovered later runs the GC. The frame that runs its defers is cleared
// through ctx.parent: its dead object is freed, but an object still used
// by an outer frame (or by a deferred call of an outer frame) is kept.

//go:noinline
func goexitInner(name string) {
	dead := newT(name + ": dead object")
	fmt.Println("using", dead.name)
	defer func() {
		fmt.Println(wait())
		fmt.Println(name+":", kept())
	}()
	runtime.Goexit()
}

//go:noinline
func goexitOuter(name string, ended chan bool) {
	live := newT(name + ": live object")
	defer func() {
		fmt.Println(name+": outer defer uses", live.name)
		ended <- true
	}()
	goexitInner(name)
}

// goexitCallee: runtime.Goexit in a function called by the goroutine.
func goexitCallee() {
	ended := make(chan bool)
	go goexitOuter("goexit in a callee", ended)
	<-ended
	fmt.Println(wait())
}

// goexitSelf: runtime.Goexit in the function of the goroutine itself.
func goexitSelf() {
	ended := make(chan bool)
	go func() {
		live := newT("goexit in the goroutine: live object")
		dead := newT("goexit in the goroutine: dead object")
		fmt.Println("using", dead.name)
		defer func() { ended <- true }()
		defer func() {
			fmt.Println(wait())
			fmt.Println("goexit in the goroutine:", kept())
			fmt.Println("defer uses", live.name)
		}()
		runtime.Goexit()
	}()
	<-ended
	fmt.Println(wait())
}

//go:noinline
func panicInner(name string) {
	dead := newT(name + ": dead object")
	fmt.Println("using", dead.name)
	defer func() {
		fmt.Println(wait())
		fmt.Println(name+":", kept())
		fmt.Println("recovered:", recover())
	}()
	panic("boom")
}

// recoveredPanic: the outer frame goes on after the recover and still uses
// its object.
func recoveredPanic() {
	live := newT("recovered panic: live object")
	panicInner("recovered panic")
	fmt.Println("after the recover, using", live.name)
	fmt.Println(wait())
}

func main() {
	goexitCallee()
	goexitSelf()
	recoveredPanic()
	deferUintptr()
	callArgs()
	deferReturn()
	deferPanic()
	keepAlive()
	stringData()
}
