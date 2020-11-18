package main

type F func() (int, error)

func f1() (int, error) { return 3, nil }

func f2(a string, f F) {
	c, _ := f()
	println(a, c)
}

func main() {
	f2("hello", F(f1))
}

// Output:
// hello 3
