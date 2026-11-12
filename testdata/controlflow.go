// Control flow: labeled break and continue across for, switch and select,
// goto forward and backward, fallthrough, switch without a condition, switch
// with init statements, range over func iterators (with break, early
// return, defer and panic inside the loop body), range over int, and
// shadowing in nested scopes.
package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

func countTo(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 1; i <= n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func pairs(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for i, f := range strings.Fields(s) {
			if !yield(i, f) {
				return
			}
		}
	}
}

var cleanups []string

func withCleanup(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer func() { cleanups = append(cleanups, fmt.Sprint("cleanup ", n)) }()
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

func findFirst(target string) (int, bool) {
	for i, w := range pairs("a b target c target") {
		if w == target {
			return i, true // return from inside a range-over-func body
		}
	}
	return -1, false
}

func deferInBody() (out []string) {
	for i := range countTo(3) {
		defer func() { out = append(out, fmt.Sprint("defer ", i)) }() // runs when deferInBody returns
	}
	out = append(out, "body done")
	return out
}

func panicInBody() (msg string) {
	defer func() { msg = fmt.Sprint("recovered ", recover(), " ", cleanups) }()
	for i := range withCleanup(5) {
		if i == 2 {
			panic("boom in body")
		}
	}
	return "no"
}

func gotoLoop() []int {
	var out []int
	i := 0
loop:
	if i < 5 {
		out = append(out, i)
		i++
		goto loop
	}
	goto end
end:
	return out
}

func main() {
	// Labeled break and continue.
	var found [][2]int
outer:
	for i := range 5 {
		for j := range 5 {
			switch {
			case j == i:
				continue outer
			case i+j == 6:
				break outer
			}
			found = append(found, [2]int{i, j})
		}
	}
	fmt.Println(found)

	// break inside a switch inside a for leaves only the switch.
	n := 0
	for i := range 4 {
		switch i {
		case 1:
			break
		default:
			n += i
		}
	}
	fmt.Println(n)

	// break out of a select with a label.
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)
	total := 0
recv:
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				break recv
			}
			total += v
		}
	}
	fmt.Println(total)

	// fallthrough.
	for _, v := range []int{0, 1, 2, 3} {
		var s []string
		switch v {
		case 0:
			s = append(s, "zero")
			fallthrough
		case 1:
			s = append(s, "one")
			fallthrough
		case 2:
			s = append(s, "two")
		default:
			s = append(s, "other")
		}
		fmt.Println(v, s)
	}

	// switch with init and no tag; case lists.
	switch x := len("abc"); {
	case x > 5:
		fmt.Println("big")
	case x == 3, x == 4:
		fmt.Println("three or four")
	}
	switch r := 'é'; r {
	case 'a', 'e', 'é':
		fmt.Println("vowel", string(r))
	}

	fmt.Println(gotoLoop())

	// range over func.
	sum := 0
	for v := range countTo(10) {
		if v%2 == 0 {
			continue
		}
		if v > 7 {
			break
		}
		sum += v
	}
	fmt.Println(sum)
	fmt.Println(findFirst("target"))
	fmt.Println(findFirst("missing"))
	fmt.Println(deferInBody())
	for i := range withCleanup(10) {
		if i == 1 {
			break
		}
	}
	fmt.Println(cleanups)
	fmt.Println(panicInBody())

	// Nested range-over-func with labels.
	var hits []string
rows:
	for i := range countTo(4) {
		for j, w := range pairs("x y z") {
			if j == i {
				continue rows
			}
			if i == 3 && j == 1 {
				break rows
			}
			hits = append(hits, fmt.Sprint(i, w))
		}
	}
	fmt.Println(hits)

	// slices and maps iterators.
	m := map[string]int{"b": 2, "a": 1, "c": 3}
	keys := slices.Sorted(maps.Keys(m))
	fmt.Println(keys)
	for i, v := range slices.Backward([]string{"x", "y", "z"}) {
		fmt.Print(i, v, " ")
	}
	fmt.Println()
	fmt.Println(slices.Collect(countTo(4)))
	for c := range slices.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		fmt.Print(c, " ")
	}
	fmt.Println()

	// iter.Pull.
	next, stop := iter.Pull(countTo(3))
	for {
		v, ok := next()
		if !ok {
			break
		}
		fmt.Print("pull ", v, " ")
	}
	stop()
	fmt.Println()
	next2, stop2 := iter.Pull2(pairs("p q r"))
	k, v, ok := next2()
	fmt.Println(k, v, ok)
	stop2()
	k, v, ok = next2()
	fmt.Println(k, v, ok)

	// Shadowing.
	x := 1
	{
		x := 2
		x++
		_ = x
	}
	if x := 10; x > 5 {
		fmt.Println("inner x", x)
	}
	for x := 0; x < 1; x++ {
		fmt.Println("loop x", x)
	}
	fmt.Println("outer x", x)
	err := fmt.Errorf("outer")
	if true {
		_, err := fmt.Sscan("12", new(int))
		_ = err
	}
	fmt.Println(err)
	fmt := "shadowed fmt package name"
	println2(fmt)
}

func println2(s string) { fmt.Println(len(s)) }
