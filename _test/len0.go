package main

func f(a []int) any {
	return len(a)
}

func main() {
	a := []int{1, 2}
	println(f(a).(int))
}

// Output:
// 2
