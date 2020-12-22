package main

func f(a []int) interface{} {
	return cap(a)
}

func g(a []int) int {
	return cap(a)
}

func main() {
	a := []int{1, 2}
	println(g(a))
	println(f(a).(int))
}

// Output:
// 2
// 2
