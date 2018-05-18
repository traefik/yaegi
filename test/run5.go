package main

type fn func(int)

func test(f fn, v int) { f(v) }

func main() {
	f1 := func(i int) { println("f1", i) }
	test(f1, 21)
}

// Output:
// f1 21
