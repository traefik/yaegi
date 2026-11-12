package main

func main() {
	foos := []func(){}

	for i := range 3 {
		i := i
		foos = append(foos, func() { println(i) })
	}
	foos[0]()
	foos[1]()
	foos[2]()
}

// Output:
// 0
// 1
// 2
