// Pipelines of goroutines connected by channels: generator -> square ->
// filter -> sum, with order kept along a single chain, generic stages, and a
// pipeline stopped early through a done channel.
package main

import (
	"fmt"
	"strings"
)

func gen(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		for _, n := range nums {
			out <- n
		}
		close(out)
	}()
	return out
}

func sq(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			out <- n * n
		}
		close(out)
	}()
	return out
}

func filter(in <-chan int, keep func(int) bool) <-chan int {
	out := make(chan int, 2)
	go func() {
		defer close(out)
		for n := range in {
			if keep(n) {
				out <- n
			}
		}
	}()
	return out
}

func Map[T, U any](in <-chan T, f func(T) U) <-chan U {
	out := make(chan U)
	go func() {
		defer close(out)
		for v := range in {
			out <- f(v)
		}
	}()
	return out
}

func Collect[T any](in <-chan T) []T {
	var r []T
	for v := range in {
		r = append(r, v)
	}
	return r
}

func genDone(done <-chan struct{}) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 1; ; i++ {
			select {
			case out <- i:
			case <-done:
				return
			}
		}
	}()
	return out
}

func main() {
	fmt.Println(Collect(sq(gen(1, 2, 3, 4, 5))))
	fmt.Println(Collect(sq(sq(gen(1, 2, 3)))))

	sum := 0
	for v := range filter(sq(gen(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)), func(n int) bool { return n%2 == 0 }) {
		sum += v
	}
	fmt.Println("even squares sum", sum)

	words := Map(Map(gen(1, 22, 333), func(n int) string { return fmt.Sprint(n) }), strings.ToUpper)
	lens := Map(words, func(s string) string { return fmt.Sprintf("%s:%d", s, len(s)) })
	fmt.Println(Collect(lens))

	// Long pipeline of +1 stages.
	var c <-chan int = gen(0, 100, 200)
	for i := 0; i < 20; i++ {
		c = Map(c, func(n int) int { return n + 1 })
	}
	fmt.Println(Collect(c))

	// Stop early: take the first 10 values, then close done.
	done := make(chan struct{})
	src := genDone(done)
	squares := Map(src, func(n int) int64 { return int64(n) * int64(n) })
	var first []int64
	for v := range squares {
		first = append(first, v)
		if len(first) == 10 {
			break
		}
	}
	close(done)
	// The Map stage may hold one more value: drain it so it ends.
	for range squares {
	}
	fmt.Println(first)

	// Empty pipeline.
	fmt.Println(len(Collect(sq(gen()))), Collect(sq(gen())) == nil)

	// Struct values through a pipeline.
	type item struct {
		name  string
		price float64
	}
	items := make(chan item)
	go func() {
		for i, n := range []string{"apple", "pear", "plum"} {
			items <- item{n, float64(i+1) * 1.25}
		}
		close(items)
	}()
	withTax := Map((<-chan item)(items), func(it item) item { it.price *= 1.2; return it })
	for it := range withTax {
		fmt.Printf("%s %.2f\n", it.name, it.price)
	}
}
