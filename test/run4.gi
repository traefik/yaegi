package main

type fn func(int, int)

func f1(int) { println("f1", i) }

func test(f fn, v int) { f(v) }

func main() { test(f1, 21) }
