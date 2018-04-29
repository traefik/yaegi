// +build ignore
package main

func main() {
	a, b := r2()
	println(a, b)
}

func r2() (int, int) {return 1, 2}

// Output:
// 1 2
