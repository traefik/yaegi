package main

import "fmt"

func one() string {
	ch := make(chan string, 1)
	ch <- "ok"
	return <-ch
}

func two() (string, int) {
	ch := make(chan string, 1)
	ch <- "ok"
	n := 7
	return <-ch, n
}

func main() {
	fmt.Println(one())
	fmt.Println(two())
}

// Output:
// ok
// ok 7
