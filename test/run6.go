package main

type fn func(int)

func test(f fn, v int) { f(v) }

func main() {
	test(func(i int) { println("f1", i) }, 21)
}

// Output:
// f1 21
