package main

type Coord struct {
	x, y int
}

func (c Coord) dist() int { return c.x*c.x + c.y*c.y }

type Point struct {
	Coord
	z int
}

func main() {
	o := Point{Coord{3, 4}, 5}
	println(o.Coord.dist())
}

// Output:
// 25
