package main

func f() interface{} {
	return new(int)
}

func main() {
	a := f()
	println(*(a.(*int)))
}

// Output:
// 0
