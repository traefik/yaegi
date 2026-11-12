package main

import "fmt"

type Myint int

func (i Myint) Double() { fmt.Println("Myint:", i, i) }

type Boo interface {
	Double()
}

func f(boo Boo) {
	boo.Double()
}

func main() {
	var i Myint = 3
	f(i)
}

// Output:
// Myint: 3 3
