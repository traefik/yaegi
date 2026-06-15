package main

func f() any {
	return new(int)
}

func main() {
	a := f()
	println(*(a.(*int)))
}

// Output:
// 0
