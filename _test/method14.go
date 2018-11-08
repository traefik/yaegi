package main

func main() {
	o := Coord{3, 4}
	println(o.dist())
}

func (c *Coord) dist() int { return c.x*c.x + c.y*c.y }

type Coord struct {
	x, y int
}

// Output:
// 25
