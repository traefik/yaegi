package main

import (
	"fmt"
	"image"
)

func main() {
	v := []*image.Point{
		{X: 3, Y: 2},
		{X: 4, Y: 5},
	}
	fmt.Println(v)
}

// Output:
// [(3,2) (4,5)]
