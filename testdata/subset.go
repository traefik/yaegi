package main

type Point struct {
	X, Y int
}

func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }
func (p *Point) Scale(k int)      { p.X *= k; p.Y *= k }

type Shape interface {
	Area() int
	Name() string
}

type Rect struct{ W, H int }
type Square struct{ S int }

func (r Rect) Area() int       { return r.W * r.H }
func (r Rect) Name() string    { return "rect" }
func (s *Square) Area() int    { return s.S * s.S }
func (s *Square) Name() string { return "square" }

type Celsius int

func (c Celsius) Double() Celsius { return c * 2 }

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func swap(a, b int) (int, int) { return b, a }

func main() {
	p := Point{1, 2}
	q := p // copy
	q.X = 100
	p.Scale(3)
	r := p.Add(Point{10, 20})
	println(p.X, p.Y, q.X, q.Y, r.X, r.Y)

	xs := []int{}
	for i := 0; i < 10; i++ {
		xs = append(xs, i*i)
	}
	ys := xs[2:5]
	ys[0] = -1 // aliasing
	println(len(xs), cap(ys) > 0, xs[2], sum(xs))

	shapes := []Shape{Rect{2, 3}, &Square{4}}
	total := 0
	for _, s := range shapes {
		total += s.Area()
		if sq, ok := s.(*Square); ok {
			println("found", sq.Name(), sq.S)
		}
		switch v := s.(type) {
		case Rect:
			println("rect w", v.W)
		case *Square:
			println("square s", v.S)
		}
	}
	println("total", total)

	var c Celsius = 21
	println(c.Double())
	a, b := swap(1, 2)
	println(a, b)

	var u8 uint8 = 250
	u8 += 10
	println(u8)

	m := map[string]int{}
	m["a"] = 1
	m["b"] += 2
	v, ok := m["zz"]
	println(m["a"], m["b"], v, ok, len(m))

	f := func(k int) func(int) int {
		return func(x int) int { return x*k + len(xs) }
	}(7)
	println(f(3))

	arr := [3]Point{}
	arr[1].X = 5
	brr := arr
	brr[1].X = 9
	println(arr[1].X, brr[1].X)
	s := "héllo"
	println(len(s), s[1], s[0:2] == "h\xc3", s+"!")
}
