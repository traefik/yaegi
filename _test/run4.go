package main

type fn func(int)

func f1(i int) { println("f1", i) }

func test(f fn, v int) { f(v) }

func main() { test(f1, 21) }

// Output:
// f1 21
