package main

func main() {
	a := 1
	if a := 2; a > 0 {
		println(a)
	}
	{
		a := 3
		println(a)
	}
	println(a)
}

// Output:
// 2
// 3
// 1
