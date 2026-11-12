package main

type adder func(int, int) int

func genAdd(k int) adder {
	return func(i, j int) int {
		return i + j + k
	}
}

func main() {
	f := genAdd(5)
	g := genAdd(8)
	println(f(3, 4))
	println(g(3, 4))
}

// Output:
// 12
// 15
