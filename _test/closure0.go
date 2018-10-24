package main

type adder func(int, int) int

func genAdd(k int) adder {
	println("k:", k)
	return func(i, j int) int {
		println("#1 k:", k)
		return i + j + k
	}
}

func main() {
	f := genAdd(5)
	println(f(3, 4))
}

// Output:
// 12
