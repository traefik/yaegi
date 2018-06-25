package main

func f(i *int) {
	*i = *i + 3
}

func main() {
	var a int = 2
	f(&a)
	println(a)
}

// Output:
// 5
