// Channels of funcs and closures: a goroutine runs the funcs it receives
// (an executor), closures that capture variables, and funcs returning
// values through their own channel.
package main

import "fmt"

func main() {
	jobs := make(chan func() int)
	results := make(chan int)
	go func() {
		for f := range jobs {
			results <- f()
		}
		close(results)
	}()
	go func() {
		for i := 0; i < 6; i++ {
			jobs <- func() int { return i * i }
		}
		close(jobs)
	}()
	var got []int
	for v := range results {
		got = append(got, v)
	}
	fmt.Println(got)

	// The executor runs funcs that change shared state, in order.
	exec := make(chan func(), 10)
	state := []string{}
	done := make(chan struct{})
	go func() {
		for f := range exec {
			f()
		}
		close(done)
	}()
	for _, w := range []string{"a", "b", "c", "d"} {
		exec <- func() { state = append(state, w+w) }
	}
	close(exec)
	<-done
	fmt.Println(state)

	// Channel of func(int) (int, error).
	type op func(int) (int, error)
	ops := make(chan op, 3)
	ops <- func(x int) (int, error) { return x + 1, nil }
	ops <- func(x int) (int, error) { return 0, fmt.Errorf("bad %d", x) }
	ops <- func(x int) (int, error) { return x * 2, nil }
	close(ops)
	x := 5
	for f := range ops {
		v, err := f(x)
		fmt.Println(v, err)
		if err == nil {
			x = v
		}
	}
	fmt.Println("x", x)

	// Nil func from a closed channel.
	fc := make(chan func())
	close(fc)
	f, ok := <-fc
	fmt.Println(f == nil, ok)

	// Method values through a channel.
	type counter struct{ n int }
	c := &counter{}
	inc := func(k int) func() { return func() { c.n += k } }
	mc := make(chan func(), 3)
	mc <- inc(1)
	mc <- inc(10)
	mc <- inc(100)
	close(mc)
	for g := range mc {
		g()
	}
	fmt.Println("counter", c.n)

	// Result channels per call.
	async := func(a, b int) <-chan int {
		r := make(chan int, 1)
		go func() { r <- a * b }()
		return r
	}
	f1, f2, f3 := async(2, 3), async(4, 5), async(6, 7)
	fmt.Println(<-f3, <-f1, <-f2)
}
