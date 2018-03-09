package main

type Coord struct {
	x, y int
}

type Point struct {
	Coord
	z int
}

func (c Coord) dist() int { return c.x * c.x + c.y * c.y }

func main() {
	o := Point{ Coord{3, 4}, 5}
	println(o.dist())
}
