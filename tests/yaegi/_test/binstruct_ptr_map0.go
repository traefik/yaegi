package main

import (
	"fmt"
	"image"
)

func main() {
	v := map[string]*image.Point{
		"foo": {X: 3, Y: 2},
		"bar": {X: 4, Y: 5},
	}
	fmt.Println(v["foo"], v["bar"])
}

// Output:
// (3,2) (4,5)
