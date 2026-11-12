// defer: LIFO order, arguments evaluated when the defer statement runs
// (receivers and function values too), defers in loops, named results
// modified by defers after return, defer of method values, closures and
// builtins, and defer inside nested blocks running at function end.
package main

import "fmt"

type rec struct{ name string }

func (r rec) show(tag string)     { fmt.Println("show", r.name, tag) }
func (r *rec) showPtr(tag string) { fmt.Println("showPtr", r.name, tag) }

func order() {
	for i := 0; i < 4; i++ {
		defer fmt.Print(i, " ")
	}
	fmt.Print("loop done: ")
}

func args() {
	x := 1
	defer fmt.Println("deferred arg x =", x)
	defer func() { fmt.Println("closure sees x =", x) }()
	defer func(v int) { fmt.Println("param v =", v, "x =", x) }(x * 10)
	x = 2
}

func receivers() {
	r := rec{"first"}
	defer r.show("value receiver evaluated early")
	defer r.showPtr("pointer receiver sees late change")
	r.name = "second"
}

func funcValue() {
	f := func() { fmt.Println("original f") }
	defer f()
	f = func() { fmt.Println("replaced f") }
	f()
}

func double() (r int) {
	defer func() { r *= 2 }()
	return 21
}

func triple() (r int) {
	defer func() { r += 1 }()
	defer func() { r *= 3 }()
	r = 5
	return r + 1
}

func unnamed() int {
	r := 5
	defer func() { r = 100 }()
	return r
}

func errWrap() (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("wrapped: %w", err)
		}
	}()
	return fmt.Errorf("inner")
}

func nested() {
	defer fmt.Println("outer defer")
	{
		defer fmt.Println("block defer (runs at function end)")
	}
	if true {
		defer fmt.Println("if defer")
	}
	for i := 0; i < 2; i++ {
		func() {
			defer fmt.Println("func-literal defer", i)
		}()
	}
	fmt.Println("nested body end")
}

func loopDefers() (log []string) {
	for i := 0; i < 3; i++ {
		defer func(n int) { log = append(log, fmt.Sprint("arg", n, "cap", i)) }(i)
	}
	return []string{"start"}
}

func deferBuiltins() (s []int) {
	m := map[int]int{1: 1, 2: 2}
	defer delete(m, 1)
	defer func() { fmt.Println("map before delete", len(m)) }()
	ch := make(chan int, 1)
	defer close(ch)
	return []int{len(m)}
}

func argCalls() {
	trace := func(s string) string { fmt.Println("eval", s); return s }
	defer fmt.Println("deferred", trace("a"), trace("b"))
	fmt.Println("body")
}

func deferNilFunc() (msg string) {
	defer func() { msg = fmt.Sprint("recovered: ", recover()) }()
	var f func()
	defer f()
	fmt.Println("defer of nil func registered")
	return "normal"
}

func main() {
	order()
	fmt.Println()
	args()
	receivers()
	funcValue()
	fmt.Println(double(), triple(), unnamed())
	fmt.Println(errWrap())
	nested()
	fmt.Println(loopDefers())
	fmt.Println(deferBuiltins())
	argCalls()
	fmt.Println(deferNilFunc())
	i := 0
	func() {
		defer func() { i++ }()
		defer func() { i *= 10 }()
		i = 1
	}()
	fmt.Println("i", i)
}
