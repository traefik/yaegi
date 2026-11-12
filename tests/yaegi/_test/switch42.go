package main

import "fmt"

type Pong struct{}

func main() {
	var msg interface{} = &Pong{}
	switch msg := msg.(type) {
	case *Pong:
	case string:
		fmt.Println("string", msg)
	}
	fmt.Println("done")
}

// Output:
// done
