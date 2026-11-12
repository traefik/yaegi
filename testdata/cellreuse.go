// The cells of the locals that stay in their frame (vm/escape.go) are used
// again by the next call of the same function, once the frame that had
// them returned. Each call must still start from a zero value, a call
// that a panic stops must not give its cells to the next call, and
// recursive calls that are all running must have distinct locals.
package main

import (
	"fmt"
	"testing"
)

type req struct {
	path  string
	parts [4]int // the length of each part
	n     int
}

func split(r *req) {
	for i := 0; i < len(r.path) && r.n < len(r.parts); i++ {
		if r.path[i] == '/' {
			r.n++
			continue
		}
		r.parts[r.n]++
	}
}

func count(r *req) int { return r.n }

// handle has a local whose address only goes to split and count.
func handle(path string) int {
	var r req
	if r.n != 0 || r.parts[0]+r.parts[1]+r.parts[2]+r.parts[3] != 0 {
		panic("not zero")
	}
	r.path = path
	split(&r)
	return count(&r)*10 + r.parts[0]
}

// depth: each running frame has its own a, and sees it unchanged after
// the deeper calls returned.
func depth(n int) int {
	var a [3]int
	a[0], a[1], a[2] = n, n*2, n*3
	if n > 0 {
		depth(n - 1)
	}
	if a[0] != n || a[1] != n*2 || a[2] != n*3 {
		panic(fmt.Sprint("changed at ", n, ": ", a))
	}
	return a[0] + a[1] + a[2]
}

// stopped panics while its local is in use; recover in the caller.
func stopped(fail bool) (n int) {
	var r req
	r.path = "a/b"
	split(&r)
	if fail {
		panic("stop")
	}
	return count(&r)
}

func try(fail bool) (n int) {
	defer func() { recover() }()
	return stopped(fail)
}

var sink int

func main() {
	fmt.Println(handle("a/b/c"), handle("xy/z"), handle(""), handle("/"))
	fmt.Println(depth(5), depth(0))
	fmt.Println(try(true), try(false), try(true), try(false))

	// gc keeps these locals on the stack: no allocation per call.
	fmt.Println("handle allocates:", testing.AllocsPerRun(100, func() { sink = handle("p/q") }) >= 1)
	fmt.Println("depth allocates:", testing.AllocsPerRun(100, func() { sink = depth(20) }) >= 1)
}
