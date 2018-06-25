package main

type fn func(int)

func test(f fn, v int) { f(v) }

func main() {
	a := 3
	test(func(i int) { println("f1", i, a) }, 21)
}

// Output:
// f1 21 3
