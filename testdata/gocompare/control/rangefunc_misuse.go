// Range-over-func misuse: yield called after the loop body returned false,
// after the whole loop (also left by break, return or goto), during a body
// panic, and an iterator that recovers the panic of the body.
package main

import (
	"fmt"
	"runtime"
)

func show(name string, f func()) {
	defer func() {
		r := recover()
		_, ok := r.(runtime.Error)
		fmt.Printf("%s %T %v %v\n", name, r, ok, r)
	}()
	f()
	fmt.Println(name, "no panic")
}

func twice(yield func(int) bool) {
	yield(1)
	yield(2)
}

func recovers(yield func(int) bool) {
	defer func() { recover() }()
	yield(0)
	yield(1)
}

func main() {
	show("a", func() {
		for range twice {
			break
		}
	})
	show("b", func() {
		for x := range twice {
			if x == 1 {
				return
			}
		}
	})
	show("c", func() {
		var keep func(int) bool
		for range func(yield func(int) bool) { keep = yield } {
		}
		keep(3)
	})
	show("d", func() {
		for range func(yield func(int) bool) {
			defer func() { recover(); yield(5) }()
			yield(4)
		} {
			panic("body")
		}
	})
	show("e", func() {
		for range recovers {
			break
		}
		fmt.Println("after loop")
	})
	show("f", func() {
		for range func(yield func(int) bool) {
			defer func() { recover() }()
			yield(0)
		} {
			panic("body")
		}
	})
	show("g", func() {
		n := 0
		for x := range twice {
			n += x
		}
		fmt.Println("g sum", n)
	})
	show("h", func() {
	outer:
		for range twice {
			for range twice {
				break outer
			}
		}
	})
	// a yield kept and called after the loop that the body left with
	// break, return or goto: the loop is exhausted
	var keep func(int) bool
	keeps := func(yield func(int) bool) { keep = yield; yield(1) }
	show("i", func() {
		for range keeps {
			break
		}
		keep(3)
	})
	show("j", func() {
		func() {
			for range keeps {
				return
			}
		}()
		keep(3)
	})
	show("k", func() {
		for range keeps {
			goto out
		}
	out:
		keep(3)
	})
}
