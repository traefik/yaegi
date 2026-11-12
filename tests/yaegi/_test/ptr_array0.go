package main

type T [2]int

func F0(t *T) int { return t[0] }

func main() {
	t := &T{1, 2}
	println(F0(t))
}

// Output:
// 1
