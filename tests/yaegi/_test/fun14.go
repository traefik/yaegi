package main

func f() (bool, int) { return true, 2 }

func g() (bool, int) { return f() }

func main() {
	b, i := g()
	println(b, i)
}

// Output:
// true 2
