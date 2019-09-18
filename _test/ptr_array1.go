package main

type T [3]int

func F0(t *T) {
	for i, v := range t {
		println(i, v)
	}
}

func main() {
	t := &T{1, 2, 3}
	F0(t)
}

// Output:
// 0 1
// 1 2
// 2 3
