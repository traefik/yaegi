package main

func f(i *int) {
	*i++
}

func main() {
	var a int = 2
	f(&a)
	println(a)
}

// Output:
// 3
