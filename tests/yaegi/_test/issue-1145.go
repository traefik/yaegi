package main

import "sync"

type F func()

func main() {
	var wg sync.WaitGroup
	var f F = wg.Done
	println(f != nil)
}

// Output:
// true
