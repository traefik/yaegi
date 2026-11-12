package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		print("test")
	}()

	wg.Wait()
}

func print(state string) {
	fmt.Println(state)
}

// Output:
// test
