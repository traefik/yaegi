package main

func main() {
	a := 3
	f := func(i int) { println("f1", i, a) }
	f(21)
}

// Output:
// f1 21 3
