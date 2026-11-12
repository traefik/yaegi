// Composed range-over-func iterators: generic filter/map/take/zip/chain
// adapters, nested loops over the same iterator, defers inside nested
// range-over-func bodies (run at function exit, in LIFO order), early exits
// that stop every layer, and the cleanup order of each layer.
package main

import (
	"fmt"
	"iter"
	"strings"
)

var events []string

func gen(name string, n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer func() { events = append(events, name+" closed") }()
		for i := 1; i <= n; i++ {
			if !yield(i) {
				events = append(events, fmt.Sprint(name, " stop at ", i))
				return
			}
		}
	}
}

func filter[T any](seq iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

func mapSeq[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			i++
			if i == n {
				return
			}
		}
	}
}

func chain[T any](seqs ...iter.Seq[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, s := range seqs {
			for v := range s {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	return func(yield func(A, B) bool) {
		nb, stop := iter.Pull(b)
		defer stop()
		for va := range a {
			vb, ok := nb()
			if !ok || !yield(va, vb) {
				return
			}
		}
	}
}

func collect[T any](seq iter.Seq[T]) []T {
	var out []T
	for v := range seq {
		out = append(out, v)
	}
	return out
}

func flush(label string) {
	fmt.Println(label, events)
	events = nil
}

func deferNested() (out []string) {
	for i := range gen("outer", 2) {
		defer func() { out = append(out, fmt.Sprint("outer defer ", i)) }()
		for j := range gen("inner", 2) {
			defer func() { out = append(out, fmt.Sprint("inner defer ", i, j)) }()
		}
	}
	out = append(out, "body end")
	return out
}

func main() {
	even := func(n int) bool { return n%2 == 0 }
	sq := func(n int) int { return n * n }
	fmt.Println(collect(mapSeq(filter(gen("g", 10), even), sq)))
	flush("events:")
	fmt.Println(collect(take(mapSeq(gen("g", 100), sq), 4)))
	flush("events:")
	fmt.Println(collect(take(gen("g", 3), 0)))
	flush("events:")
	fmt.Println(collect(chain(gen("a", 2), gen("b", 3))))
	flush("events:")
	for v := range chain(gen("a", 2), gen("b", 3)) {
		if v == 2 && len(events) > 0 {
			break
		}
	}
	flush("chain break:")
	for a, b := range zip(gen("za", 5), mapSeq(gen("zb", 3), func(n int) string { return strings.Repeat("*", n) })) {
		fmt.Print(a, b, " ")
	}
	fmt.Println()
	flush("zip:")
	for i := range gen("x", 3) {
		for j := range gen("y", 3) {
			if j > i {
				break
			}
			fmt.Print(i, j, " ")
		}
	}
	fmt.Println()
	flush("nested:")
	fmt.Println(deferNested())
	flush("defer nested:")
	g := gen("same", 2)
	for a := range g {
		for b := range g {
			fmt.Print(a*10+b, " ")
		}
	}
	fmt.Println()
	flush("same iterator nested:")
	words := mapSeq(gen("w", 5), func(n int) string { return string(rune('a' + n - 1)) })
	fmt.Println(strings.Join(collect(words), ","))
	flush("words:")
	func() {
		defer func() { fmt.Println("recovered", recover()) }()
		for v := range mapSeq(gen("p", 5), func(n int) int { return 10 / (3 - n) }) {
			fmt.Print(v, " ")
		}
	}()
	flush("panic in adapter:")
}
