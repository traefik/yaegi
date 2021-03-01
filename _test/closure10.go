package main

func main() {
	foos := []func(){}

	for i := 0; i < 3; i++ {
		a, b := i, i
		_ = b
		foos = append(foos, func() { println(i, a) })
	}
	foos[0]()
	foos[1]()
	foos[2]()
}

// Output:
// 3 0
// 3 1
// 3 2
