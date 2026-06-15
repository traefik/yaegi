package main

func f() any {
	return make([]int, 2)
}

func main() {
	a := f()
	println(len(a.([]int)))
}

// Output:
// 2
