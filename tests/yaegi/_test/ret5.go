package main

func r2() (int, int) { return 1, 2 }

var a, b int = r2()

func main() { println(a, b) }

// Output:
// 1 2
