package main

type foo func(b int)

func boo(b int) { println("boo", b) }

func main() {
	var f foo

	f = boo
	f(4)
}

// Output:
// boo 4
