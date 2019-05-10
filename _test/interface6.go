package main

import "fmt"

type Myint int

func (i Myint) Double() { fmt.Println("Myint:", i, i) }

type Boo interface {
	Double()
}

func f(boo Boo) { boo.Double() }

func g(i int) Boo { return Myint(i) }

func main() {
	f(g(4))
}

// Output:
// Myint: 4 4
