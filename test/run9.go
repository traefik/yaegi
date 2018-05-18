package main

func main() {
	a := 3
	f := func(i int) int { println("f1", i, a); return i + 1 }
	b := f(21)
	println(b)
}

// Output:
// f1 21 3
// 22
