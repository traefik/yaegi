// range over functions with panics: a panic in the loop body goes through the
// iterator's defers; recover in the body; return from the body runs the
// iterator's cleanup; a panic raised by the iterator itself reaches the caller.
package main

import (
	"fmt"
	"runtime"
)

func cleanup(yield func(int) bool) {
	defer fmt.Println("iterator cleanup")
	for i := 0; i < 5; i++ {
		if !yield(i) {
			fmt.Println("iterator saw false at", i)
			return
		}
	}
}

func try(name string, f func()) {
	defer func() {
		r := recover()
		if r == nil {
			fmt.Println(name, "no panic")
			return
		}
		_, isRT := r.(runtime.Error)
		fmt.Printf("%s: %T runtime.Error=%v: %v\n", name, r, isRT, r)
	}()
	f()
}

func main() {
	try("panic in body", func() {
		for v := range cleanup {
			if v == 2 {
				panic(fmt.Sprint("body panic ", v))
			}
			fmt.Println("body", v)
		}
	})
	try("break with cleanup", func() {
		for v := range cleanup {
			if v == 1 {
				break
			}
		}
		fmt.Println("after break")
	})
	try("recover in body defer", func() {
		for v := range cleanup {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Println("body recovered", r)
					}
				}()
				if v == 3 {
					panic("inner")
				}
			}()
		}
		fmt.Println("loop finished")
	})
	try("return from body", func() {
		res := func() int {
			for v := range cleanup {
				if v == 2 {
					return v * 100
				}
			}
			return -1
		}()
		fmt.Println("res", res)
	})
	try("iterator panics", func() {
		it := func(yield func(int) bool) {
			yield(1)
			panic("iterator boom")
		}
		for v := range it {
			fmt.Println("body", v)
		}
	})
	try("runtime error in body", func() {
		var m map[string]int
		for v := range cleanup {
			if v == 1 {
				m["x"] = v
			}
		}
	})
	try("nested", func() {
		for i := range cleanup {
			for j := range cleanup {
				if j == 1 {
					panic(fmt.Sprint("nested ", i, j))
				}
			}
		}
	})
}
