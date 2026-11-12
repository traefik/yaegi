package main

import "fmt"

func f(x int) {
	switch x {
	case 1:
	case 2, 3:
	default:
	case 4:
		fmt.Println("four")
	}
	fmt.Println("after", x)
}

func g(x int) {
	switch {
	case x < 0:
	default:
	case x > 10:
		fmt.Println("big")
	}
	fmt.Println("after g", x)
}

func main() {
	for _, x := range []int{1, 2, 3, 4, 5} {
		f(x)
	}
	g(-1)
	g(5)
	g(20)
}

// Output:
// after 1
// after 2
// after 3
// four
// after 4
// after 5
// after g -1
// after g 5
// big
// after g 20
