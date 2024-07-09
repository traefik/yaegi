package main

func main() {
	foos := []func(){}

	for i := range 3 {
		a := i
		foos = append(foos, func() { println(i, a) })
	}
	foos[0]()
	foos[1]()
	foos[2]()
}

// Output:
// 0 0
// 1 1
// 2 2
