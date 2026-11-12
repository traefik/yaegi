package main

import "fmt"

func safeDiv(a, b int) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered: %v", r)
		}
	}()
	result = a / b
	return
}

func order() {
	for i := 0; i < 3; i++ {
		defer fmt.Print(i, " ")
	}
}

func named() (x int) {
	defer func() { x *= 2 }()
	x = 21
	return
}

func main() {
	r, err := safeDiv(10, 2)
	fmt.Println(r, err)
	r, err = safeDiv(1, 0)
	fmt.Println(r, err)
	order()
	fmt.Println()
	fmt.Println(named())
}
