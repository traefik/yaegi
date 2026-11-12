// break and continue inside select and switch nested in for loops: a plain
// break leaves only the select or switch; labeled break/continue leave the
// loop; select with default in a loop; nil channels disabled in a loop;
// closing channels to end range loops; and for-range over a channel.
package main

import "fmt"

func main() {
	ch := make(chan int, 5)
	for i := 1; i <= 5; i++ {
		ch <- i
	}
	close(ch)
	var got []int
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				goto done
			}
			if v == 3 {
				break // leaves the select only
			}
			got = append(got, v)
		}
	}
done:
	fmt.Println(got)

	polls := 0
	empty := make(chan int)
	for i := 0; i < 3; i++ {
		select {
		case <-empty:
			fmt.Println("never")
		default:
			polls++
			continue
		}
		fmt.Println("never reached")
	}
	fmt.Println("polls", polls)

	a := make(chan int, 3)
	b := make(chan int, 3)
	for i := 0; i < 3; i++ {
		a <- i
		b <- i * 10
	}
	close(a)
	close(b)
	sumA, sumB := 0, 0
	for a != nil || b != nil {
		select {
		case v, ok := <-a:
			if !ok {
				a = nil
				continue
			}
			sumA += v
		case v, ok := <-b:
			if !ok {
				b = nil
				continue
			}
			sumB += v
		}
	}
	fmt.Println(sumA, sumB)

	c := make(chan string)
	go func() {
		for _, s := range []string{"x", "y", "z"} {
			c <- s
		}
		close(c)
	}()
	var all string
	for s := range c {
		all += s
	}
	fmt.Println(all)

	work := make(chan int)
	results := make(chan int)
	go func() {
		defer close(results)
		for w := range work {
			results <- w * w
		}
	}()
	go func() {
		for i := 1; i <= 4; i++ {
			work <- i
		}
		close(work)
	}()
	total := 0
	for r := range results {
		total += r
	}
	fmt.Println("total", total)

	counts := map[string]int{}
loop:
	for i := 0; i < 10; i++ {
		switch {
		case i%5 == 4:
			counts["break"]++
			break loop
		case i%2 == 0:
			counts["even"]++
			continue loop
		}
		counts["odd"]++
	}
	fmt.Println(counts)

	timeout := make(chan bool, 1)
	data := make(chan int, 2)
	data <- 1
	data <- 2
	timeout <- true
	n := 0
recv:
	for {
		select {
		case v := <-data:
			n += v
		case <-timeout:
			if len(data) > 0 {
				timeout <- true
				continue recv
			}
			break recv
		}
	}
	fmt.Println("n", n)
}
