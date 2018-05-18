package main

type Coord struct{ x, y int }

func f(c Coord) int { return c.x + c.y }

func main() {
	c := Coord{3, 4}
	println(f(c))
}

// Output:
// 7
