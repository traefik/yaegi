// range over functions (iter.Seq, iter.Seq2 and plain func types): early
// break, continue, return from inside the loop body, labeled break out of
// nested range-over-func loops, defers in the body (run when the enclosing
// function returns), and iterators from the slices and maps packages.
package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
)

var log []string

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				log = append(log, fmt.Sprint("stopped at ", i))
				return
			}
		}
		log = append(log, "count done")
	}
}

func pairs(s []string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for i, v := range s {
			if !yield(i, v) {
				return
			}
		}
	}
}

func empty(yield func() bool) {}

func three(yield func() bool) {
	_ = yield() && yield() && yield()
}

func findFirst(seq iter.Seq[int], pred func(int) bool) (int, bool) {
	for v := range seq {
		if pred(v) {
			return v, true
		}
	}
	return -1, false
}

func withDefers() (out []string) {
	for v := range count(3) {
		defer func() { out = append(out, fmt.Sprint("defer ", v)) }()
	}
	out = append(out, "body end")
	return out
}

type tree struct {
	left, right *tree
	val         int
}

func (t *tree) all(yield func(int) bool) bool {
	if t == nil {
		return true
	}
	return t.left.all(yield) && yield(t.val) && t.right.all(yield)
}

func (t *tree) All() iter.Seq[int] {
	return func(yield func(int) bool) { t.all(yield) }
}

func insert(t *tree, v int) *tree {
	if t == nil {
		return &tree{val: v}
	}
	if v < t.val {
		t.left = insert(t.left, v)
	} else {
		t.right = insert(t.right, v)
	}
	return t
}

func main() {
	for v := range count(5) {
		fmt.Print(v, " ")
	}
	fmt.Println(log)
	log = nil
	for v := range count(10) {
		if v == 3 {
			break
		}
		if v == 1 {
			continue
		}
		fmt.Print(v, " ")
	}
	fmt.Println(log)
	log = nil
	for i, s := range pairs([]string{"a", "b", "c"}) {
		fmt.Print(i, s, " ")
	}
	fmt.Println()
	for i := range pairs([]string{"x", "y"}) {
		fmt.Print(i, " ")
	}
	fmt.Println()
	for range empty {
		fmt.Println("never")
	}
	n := 0
	for range three {
		n++
	}
	fmt.Println("three", n)
	fmt.Println(findFirst(count(100), func(v int) bool { return v*v > 50 }))
	fmt.Println(findFirst(count(5), func(v int) bool { return v > 50 }))
	fmt.Println(log)
	log = nil
	fmt.Println(withDefers())
outer:
	for i := range count(4) {
		for j := range count(4) {
			if j > i {
				continue outer
			}
			if i == 3 {
				break outer
			}
			fmt.Print(i, j, " ")
		}
	}
	fmt.Println()
	fmt.Println(log)
	var root *tree
	for _, v := range []int{5, 3, 8, 1, 4, 7, 9, 2, 6} {
		root = insert(root, v)
	}
	for v := range root.All() {
		fmt.Print(v, " ")
		if v == 6 {
			break
		}
	}
	fmt.Println()
	fmt.Println(slices.Collect(root.All()))
	s := []string{"c", "a", "b"}
	for i, v := range slices.All(s) {
		fmt.Print(i, v, " ")
	}
	for i, v := range slices.Backward(s) {
		fmt.Print(i, v, " ")
	}
	fmt.Println()
	m := map[string]int{"z": 26, "a": 1, "m": 13}
	keys := slices.Sorted(maps.Keys(m))
	fmt.Println(keys)
	sum := 0
	for v := range maps.Values(m) {
		sum += v
	}
	fmt.Println(sum)
	next, stop := iter.Pull(count(3))
	for {
		v, ok := next()
		if !ok {
			break
		}
		fmt.Print("pull ", v, " ")
	}
	stop()
	fmt.Println()
	next2, stop2 := iter.Pull2(pairs([]string{"p", "q", "r"}))
	k, v, ok := next2()
	fmt.Println(k, v, ok)
	stop2()
	k, v, ok = next2()
	fmt.Println(k, v, ok)
	// A function value stored in a variable.
	var seq func(func(string) bool) = func(yield func(string) bool) {
		for _, w := range []string{"one", "two", "three"} {
			if !yield(w) {
				return
			}
		}
	}
	for w := range seq {
		fmt.Print(w, len(w), " ")
	}
	fmt.Println()
	// goto out of a range-over-func loop
	for v := range count(10) {
		if v == 2 {
			goto done
		}
		fmt.Print("g", v, " ")
	}
done:
	fmt.Println()
	fmt.Println(log)
}
