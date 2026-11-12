// Stack introspection of interpreted code (vm/callers.go): runtime.Caller,
// runtime.Callers, runtime.CallersFrames, runtime.FuncForPC of their PCs,
// runtime.Stack and runtime/debug.Stack must see the frames of the program,
// with the lines of gc, in deferred calls during a panic too. The output
// keeps only the frames of this file, by base name.
package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// here prints the frames of this file from the caller of here, as
// runtime.Callers and runtime.CallersFrames give them.
func here(label string) {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var out []string
	for {
		f, more := frames.Next()
		if strings.HasPrefix(f.Function, "main.") {
			out = append(out, fmt.Sprintf("%s %s:%d", f.Function, filepath.Base(f.File), f.Line))
		}
		if !more {
			break
		}
	}
	fmt.Println(label+":", strings.Join(out, " <- "))
}

// stackLines keeps the file:line lines of this file of a traceback.
func stackLines(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "main.") || strings.HasPrefix(l, "goroutine ") {
			out = append(out, strings.Fields(l)[0])
		}
		if strings.HasPrefix(l, "main.") {
			out[len(out)-1] = l[:strings.LastIndexByte(l, '(')] // gc prints the arguments
		}
		if i := strings.Index(l, "callers.go:"); i >= 0 {
			out = append(out, strings.Fields(l[i:])[0])
		}
	}
	return strings.Join(out, " ")
}

type T struct{ n int }

func (t *T) M() { here("method") }

func (t T) V() int {
	_, file, line, ok := runtime.Caller(1)
	fmt.Println("value method called from", filepath.Base(file), line, ok)
	return t.n
}

type I interface{ V() int }

func g[X any](x X) { here("generic") }

func nilDeref() {
	defer func() {
		recover()
		here("nil deref")
	}()
	var p *T
	fmt.Println(p.n) // the panic is here
}

func index(i int) {
	defer func() {
		recover()
		var buf [4096]byte
		n := runtime.Stack(buf[:], false)
		fmt.Println("stack:", stackLines(string(buf[:n])))
	}()
	a := []int{1, 2, 3}
	a[i] = 4 // the panic is here
	fmt.Println("not reached")
}

func normal() {
	defer here("deferred at return")
	fmt.Println("normal")
}

func caller() {
	for i := 0; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		f := runtime.FuncForPC(pc)
		if !strings.HasPrefix(f.Name(), "main.") {
			continue
		}
		ffile, fline := f.FileLine(pc)
		fmt.Println("caller", i, f.Name(), filepath.Base(file), line, filepath.Base(ffile) == filepath.Base(file), fline == line)
	}
}

// The init functions of gc are main.init.0, main.init.1..., and a closure
// of a package variable belongs to main.init.
func init() {
	func() { here("init") }()
}

func init() {
	here("second init")
}

var initClosure = func() string {
	pc, _, _, _ := runtime.Caller(0)
	return runtime.FuncForPC(pc).Name()
}()

// bottom prints the runtime frame below main.main: runtime.main, at the
// line where it calls main.main.
func bottom() {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(1, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function == "runtime.main" {
			fmt.Println("bottom:", f.Function, filepath.Base(f.File), f.Line)
		}
		if !more {
			break
		}
	}
}

func main() {
	fmt.Println("package closure:", initClosure)
	bottom()
	here("main")
	(&T{}).M()
	var i I = T{n: 1}
	i.V()
	func() {
		here("closure")
	}()
	g(1)
	nilDeref()
	index(5)
	normal()
	caller()
	// A callback of native code: the walk goes on below the native frames
	// (sort's frames are not shown: only this file is printed).
	s := []int{2, 1}
	sort.Slice(s, func(i, j int) bool {
		if i == 1 {
			here("callback")
		}
		return s[i] < s[j]
	})
	fmt.Println("debug.Stack:", stackLines(string(debug.Stack())))
	// FuncForPC past the end of a function finds another function (or
	// none), as in gc.
	var pc [1]uintptr
	runtime.Callers(1, pc[:])
	for i := uintptr(0); ; i++ {
		if f := runtime.FuncForPC(pc[0] + i); f.Name() != "main.main" {
			fmt.Println("end of main.main found")
			break
		}
	}
	done := make(chan bool)
	go func() {
		here("goroutine")
		done <- true
	}()
	<-done
	lineDirective()
}

// lineDirective is the last function: the directive changes the positions
// up to the end of the file.
func lineDirective() {
//line other.go:100
	here("line directive")
}
