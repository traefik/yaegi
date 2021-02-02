package main

import "fmt"

func main() {
	defer func() {
		r := recover()
		if r != nil {
			fmt.Println(r)
		}
	}()

	panic("Ho Ho Ho!")
}

// Output:
// Ho Ho Ho!
