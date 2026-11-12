// panic and recover: recover only works when called directly by a deferred
// function; nested defers; re-panic with the same and another value; panics
// in deferred calls replacing earlier panics (what recover sees); recover
// returning nil when not panicking; panic values of many types and their fmt.
package main

import (
	"errors"
	"fmt"
)

type myErr struct {
	op   string
	code int
}

func (e *myErr) Error() string { return fmt.Sprintf("%s failed: %d", e.op, e.code) }

type strVal string

func (s strVal) String() string { return "strVal(" + string(s) + ")" }

func catch(name string, f func()) {
	defer func() {
		r := recover()
		fmt.Printf("%s: %T %v\n", name, r, r)
	}()
	f()
}

func helperRecover() interface{} { return recover() }

func notDirect() (res string) {
	defer func() {
		r := recover()
		res = fmt.Sprint("outer got: ", r)
	}()
	defer func() {
		// recover called by a helper, not directly by the deferred function.
		r := helperRecover()
		fmt.Println("helper recover returned", r)
	}()
	panic("p1")
}

func replaced() {
	defer func() {
		panic("second")
	}()
	panic("first")
}

func replacedThrice() {
	defer func() { panic(errors.New("third")) }()
	defer func() { panic(2) }()
	panic("first")
}

func repanic() {
	defer func() {
		r := recover()
		panic(fmt.Sprint("re: ", r))
	}()
	panic("orig")
}

func repanicSame() {
	defer func() {
		r := recover()
		panic(r)
	}()
	panic(&myErr{"open", 2})
}

func recoverInNestedDefer() (out string) {
	defer func() {
		r := recover()
		out += fmt.Sprint("; outer recovered: ", r)
	}()
	defer func() {
		defer func() {
			r := recover()
			out = fmt.Sprint("nested defer recovered: ", r)
		}()
		panic("from defer")
	}()
	panic("main")
}

func recoverTwice() {
	defer func() {
		fmt.Println("first recover:", recover())
		fmt.Println("second recover:", recover())
	}()
	panic("once")
}

func continueAfterRecover() (n int) {
	defer func() {
		if r := recover(); r != nil {
			n = -1
		}
	}()
	n = 10
	panic("x")
}

func recoverNotPanicking() {
	r := recover()
	fmt.Println("recover outside panic:", r)
	defer func() { fmt.Println("deferred recover without panic:", recover()) }()
}

func deferredRunDuringPanic() {
	defer fmt.Println("defer 1 runs")
	defer func() {
		fmt.Println("defer 2 runs")
	}()
	panic("go")
}

func main() {
	catch("string", func() { panic("hello") })
	catch("int", func() { panic(42) })
	catch("float", func() { panic(1.5) })
	catch("error", func() { panic(errors.New("an error")) })
	catch("custom error", func() { panic(&myErr{"read", 5}) })
	catch("stringer", func() { panic(strVal("v")) })
	catch("struct", func() { panic(struct{ A, B int }{1, 2}) })
	catch("slice", func() { panic([]string{"a", "b"}) })
	catch("wrapped", func() { panic(fmt.Errorf("ctx: %w", &myErr{"w", 1})) })
	fmt.Println(notDirect())
	catch("replaced", replaced)
	catch("replacedThrice", replacedThrice)
	catch("repanic", repanic)
	catch("repanicSame", repanicSame)
	fmt.Println(recoverInNestedDefer())
	recoverTwice()
	fmt.Println(continueAfterRecover())
	recoverNotPanicking()
	catch("deferred run", deferredRunDuringPanic)
	catch("errors.As on recovered", func() {
		defer func() {
			r := recover()
			if err, ok := r.(error); ok {
				var me *myErr
				fmt.Println("errors.As:", errors.As(err, &me), me.code)
				panic(fmt.Sprintf("converted %v", me.op))
			}
		}()
		panic(fmt.Errorf("layer: %w", &myErr{"conv", 9}))
	})
	// Printing recovered values with several verbs.
	func() {
		defer func() {
			r := recover()
			fmt.Printf("%v|%+v|%s|%q|%T\n", r, r, r, r, r)
		}()
		panic(&myErr{"verbs", 3})
	}()
	func() {
		defer func() {
			r := recover()
			fmt.Printf("%v|%d|%x|%T\n", r, r, r, r)
		}()
		panic(int64(255))
	}()
	// A goroutine that recovers its own panic.
	done := make(chan string)
	go func() {
		defer func() { done <- fmt.Sprint("goroutine recovered: ", recover()) }()
		panic("in goroutine")
	}()
	fmt.Println(<-done)
}
