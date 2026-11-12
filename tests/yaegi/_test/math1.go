package main

import "math"

func main() {
	var f float32
	if f < math.MaxFloat32 {
		println("ok")
	}
}

// Output:
// ok
