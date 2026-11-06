package main

import "fmt"

func main() {
	ch := make(chan int, 4)
	ch <- 1
	ch <- 2
	ch <- 3
	ch <- 4

	// A variable set from a receive can be set again.
	n := <-ch
	n++
	fmt.Println(n)

	var m int
	m = <-ch
	m += 10
	fmt.Println(m)

	var i interface{}
	i = <-ch
	i = "c"
	fmt.Println(i)

	s := make(chan string, 1)
	s <- "a"
	x := <-s
	x = "b"
	fmt.Println(x)
}

// Output:
// 2
// 12
// c
// b
