// select with several ready cases: the choice is random, so the program
// only prints facts that do not depend on it (totals after draining, the
// number of times each value is seen in all).
package main

import (
	"fmt"
	"sort"
)

func main() {
	a := make(chan int, 50)
	b := make(chan int, 50)
	for i := 0; i < 50; i++ {
		a <- i
		b <- 1000 + i
	}
	close(a)
	close(b)
	var got []int
	sumA, sumB := 0, 0
	for a != nil || b != nil {
		select {
		case v, ok := <-a:
			if !ok {
				a = nil
				break
			}
			sumA += v
			got = append(got, v)
		case v, ok := <-b:
			if !ok {
				b = nil
				break
			}
			sumB += v
			got = append(got, v)
		}
	}
	sort.Ints(got)
	fmt.Println(len(got), sumA, sumB, got[0], got[49], got[50], got[99])

	// Each channel keeps its own FIFO order whatever the choice.
	x := make(chan int, 20)
	y := make(chan int, 20)
	for i := 0; i < 20; i++ {
		x <- i
		y <- i
	}
	var fromX, fromY []int
	for i := 0; i < 40; i++ {
		select {
		case v := <-x:
			fromX = append(fromX, v)
		case v := <-y:
			fromY = append(fromY, v)
		}
	}
	inOrder := func(s []int) bool {
		for i := range s {
			if s[i] != i {
				return false
			}
		}
		return true
	}
	fmt.Println(len(fromX)+len(fromY), inOrder(fromX), inOrder(fromY))

	// Several send cases ready: count of sent values per channel adds up.
	s1 := make(chan int, 30)
	s2 := make(chan int, 30)
	for i := 0; i < 30; i++ {
		select {
		case s1 <- i:
		case s2 <- i:
		}
	}
	fmt.Println("sent", len(s1)+len(s2))
	close(s1)
	close(s2)
	total := 0
	for v := range s1 {
		total += v
	}
	for v := range s2 {
		total += v
	}
	fmt.Println("total", total)

	// Mixed send and receive ready together.
	in := make(chan int, 1)
	out := make(chan int, 100)
	in <- 5
	recv, sent := 0, 0
	for i := 0; i < 10; i++ {
		select {
		case v := <-in:
			recv += v
		case out <- i:
			sent++
		}
	}
	fmt.Println("recv+sent", recv/5+sent, len(out) == sent)

	// The same channel in two cases.
	d := make(chan int, 10)
	for i := 0; i < 10; i++ {
		d <- i
	}
	close(d)
	dsum, dn := 0, 0
loop:
	for {
		select {
		case v, ok := <-d:
			if !ok {
				break loop
			}
			dsum += v
			dn++
		case v, ok := <-d:
			if !ok {
				break loop
			}
			dsum += v
			dn++
		}
	}
	fmt.Println("same channel", dn, dsum)
}
