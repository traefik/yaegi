package main

func f(a []int) interface{} {
	return cap(a)
}

func main() {
	a := []int{1, 2}
	println(f(a).(int))
}

// Output:
// 2
