// A test program

// +build darwin,linux !arm
// +build go1.12 !go1.13

package main

func main() {
	println("hello world")
}

// Output:
// hello world
