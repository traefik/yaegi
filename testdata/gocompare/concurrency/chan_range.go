// Range over channels: a producer goroutine closes the channel, range stops
// after the last buffered value, break and continue in a range, range over
// a closed empty channel, and nested producers.
package main

import "fmt"

func gen(n int) <-chan int {
	c := make(chan int)
	go func() {
		defer close(c)
		for i := 0; i < n; i++ {
			c <- i
		}
	}()
	return c
}

func genBuf(n, size int) <-chan string {
	c := make(chan string, size)
	go func() {
		for i := 0; i < n; i++ {
			c <- fmt.Sprintf("s%d", i)
		}
		close(c)
	}()
	return c
}

func main() {
	for v := range gen(5) {
		fmt.Print(v, " ")
	}
	fmt.Println()

	var all []string
	for s := range genBuf(7, 3) {
		all = append(all, s)
	}
	fmt.Println(all, len(all))

	// continue and break.
	sum := 0
	for v := range gen(20) {
		if v%2 == 0 {
			continue
		}
		sum += v
	}
	fmt.Println("odd sum", sum)

	c := make(chan int, 10)
	for i := 0; i < 10; i++ {
		c <- i
	}
	close(c)
	for v := range c {
		if v == 4 {
			break
		}
		fmt.Print(v, ",")
	}
	fmt.Println(" left:", len(c))
	for v := range c {
		fmt.Print(v, ";")
	}
	fmt.Println()

	// Range over a closed empty channel does no iteration.
	e := make(chan int)
	close(e)
	count := 0
	for range e {
		count++
	}
	fmt.Println("empty count", count)

	// Range with no variable.
	n := 0
	for range gen(13) {
		n++
	}
	fmt.Println("n", n)

	// Nested ranges: for each outer value, a new inner producer.
	for i := range gen(4) {
		var row []int
		for j := range gen(i + 1) {
			row = append(row, i*j)
		}
		fmt.Println(i, row)
	}

	// Range assigns to an existing variable.
	var last int
	cc := gen(6)
	for last = range cc {
	}
	fmt.Println("last", last)

	// Labeled break out of a range over channel.
	src := gen(100)
	total := 0
outer:
	for v := range src {
		switch {
		case v > 10:
			break outer
		default:
			total += v
		}
	}
	for range src {
		// drain so that the producer ends
	}
	fmt.Println("total", total)
}
