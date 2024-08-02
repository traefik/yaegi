package main

type T struct {
	F func()
}

func main() {
	foos := []T{}

	for i := range 3 {
		a := i
		foos = append(foos, T{func() { println(i, a) }})
	}
	foos[0].F()
	foos[1].F()
	foos[2].F()
}

// Output:
// 0 0
// 1 1
// 2 2
