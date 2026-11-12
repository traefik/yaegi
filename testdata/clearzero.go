package main

import "fmt"

// clear on a slice of structs or arrays gives each element the zero value
// of its type, which fmt (and reflect) can walk.

type T struct {
	a, b int
	s    string
}

func main() {
	ts := []T{{1, 2, "x"}, {3, 4, "y"}}
	clear(ts[1:])
	fmt.Println(ts, ts[1].a, ts[1].s == "")
	fmt.Printf("%+v %T\n", ts, ts)

	as := [][2]int{{1, 2}, {3, 4}}
	clear(as)
	fmt.Println(as, as[0][1])

	func() {
		defer clear(ts)
	}()
	fmt.Printf("%#v\n", ts)
}
