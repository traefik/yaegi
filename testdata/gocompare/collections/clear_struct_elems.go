// clear(s) on a slice of structs or arrays gives each element its own
// zero value: a write to one element after the clear changes only that
// element, and the zero values made later stay zero.
package main

import "fmt"

type P struct {
	A, B int
}

type Q struct {
	P   P
	Arr [2]int
	S   string
}

func main() {
	s := []P{{1, 2}, {3, 4}, {5, 6}}
	clear(s)
	s[0].A = 7
	s[2].B = 9
	fmt.Println(s)
	var fresh P
	fmt.Println(fresh, []P{{}, {}}, make([]P, 2))

	a := [][2]int{{1, 2}, {3, 4}}
	clear(a)
	a[1][0] = 8
	fmt.Println(a)
	var za [2]int
	fmt.Println(za)

	q := make([]Q, 3)
	q[0] = Q{P{1, 1}, [2]int{1, 1}, "x"}
	q[1] = Q{P{2, 2}, [2]int{2, 2}, "y"}
	clear(q)
	q[0].P.A = 5
	q[1].Arr[1] = 6
	q[2].S = "z"
	fmt.Println(q)
	fmt.Println(Q{}, make([]Q, 1))

	// a clear of a part of a slice, and of a slice that shares its array
	t := []P{{1, 1}, {2, 2}, {3, 3}, {4, 4}}
	u := t[1:3]
	clear(u)
	u[0].A = 10
	fmt.Println(t, u)

	// pointers taken before the clear still point to the elements
	v := []P{{1, 1}, {2, 2}}
	p0, p1 := &v[0], &v[1]
	clear(v)
	p0.A = 3
	fmt.Println(v, *p0, *p1)
}
