package main

type Coord struct{ x, y int }

func f(i, j int, c Coord) int { return i*c.x + j*c.y }

func main() {
	c := Coord{3, 4}
	println(f(2, 3, c))
}

// Output:
// 18
