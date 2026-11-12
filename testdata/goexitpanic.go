package main

// runtime.Goexit called by a callback of native code (sort.Slice) inside a
// deferred call that runs during a panic. As in gc, the Goexit ends the
// panic, the deferred calls of all the frames still run (recover sees
// nothing), and the goroutine ends. The VM lost the deferred calls of the
// frames below: the deferred wg.Done never ran and the program hung.

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func sortExit() {
	s := []int{3, 1, 2}
	sort.Slice(s, func(i, j int) bool {
		defer fmt.Println("less: deferred call runs")
		runtime.Goexit()
		return s[i] < s[j]
	})
	fmt.Println("not reached: after sort.Slice")
}

func inner() {
	defer fmt.Println("inner: deferred call runs")
	defer sortExit()
	panic("boom")
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { fmt.Println("outer: recover sees", recover()) }()
		inner()
		fmt.Println("not reached: after inner")
	}()
	wg.Wait()

	// Two Goexits: the second one runs in a deferred call of the first.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("second: last deferred call runs")
		defer func() {
			strings.Map(func(r rune) rune { runtime.Goexit(); return r }, "x")
		}()
		defer func() {
			strings.Map(func(r rune) rune { runtime.Goexit(); return r }, "y")
		}()
		panic("boom 2")
	}()
	wg.Wait()
	fmt.Println("main: done")
}
