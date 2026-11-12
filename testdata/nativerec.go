package main

// Recursion through callbacks that native code calls back into the VM:
// deep, but within the limits of the VM (each nested run continues the
// depth of the run below it, see vm/nest.go).

import (
	"fmt"
	"sort"
	"strings"
)

// viaMap goes n levels deep through strings.Map, with a few interpreted
// frames between two levels.
func viaMap(n int) int {
	if n == 0 {
		return 0
	}
	depth := 0
	strings.Map(func(r rune) rune {
		depth = down(3, n-1) + 1
		return r
	}, "x")
	return depth
}

func down(k, n int) int {
	if k == 0 {
		return viaMap(n)
	}
	return down(k-1, n)
}

func viaSort(n int) int {
	if n == 0 {
		return 0
	}
	s := []int{2, 1}
	depth := 0
	sort.Slice(s, func(i, j int) bool {
		if depth == 0 {
			depth = viaSort(n-1) + 1
		}
		return s[i] < s[j]
	})
	return depth
}

type chain int

func (c chain) String() string {
	if c == 0 {
		return "end"
	}
	s := fmt.Sprint(c - 1)
	if c%500 == 0 {
		return fmt.Sprintf("%d:%d", int(c), len(s))
	}
	return s
}

func viaDefer(n int, out *int) {
	if n == 0 {
		return
	}
	defer viaDefer(n-1, out)
	*out++
}

func deepThenMap(n, m int) int {
	if n > 0 {
		return deepThenMap(n-1, m) + 1
	}
	k := 0
	strings.Map(func(r rune) rune { k = count(m); return r }, "x")
	return k
}

func count(m int) int {
	if m == 0 {
		return 0
	}
	return count(m-1) + 1
}

func main() {
	fmt.Println("map", viaMap(3000))
	fmt.Println("sort", viaSort(3000))
	fmt.Println("stringer", chain(2000))
	n := 0
	viaDefer(3000, &n)
	fmt.Println("defer", n)
	done := make(chan int)
	go func() { done <- viaMap(3000) }()
	fmt.Println("goroutine", <-done)
	// Deeper than the plugin limit (100000 frames), within the limit of
	// programs (2M frames), with a callback at the bottom.
	fmt.Println("deep", deepThenMap(150000, 1000))
	// The same recursion again: the depth of the first one is gone.
	fmt.Println("again", viaMap(3000))
}
