// The spare capacity of a slice of structs holds zero values, which
// s[:cap(s)] shows: protobuf's internal/filedesc allocates its descriptors
// from make([]T, 0, n) and takes &s[len(s)] after growing the length.
package main

import "fmt"

type desc struct {
	name  string
	index int
	sub   *desc
	arr   [2]int
}

func main() {
	s := make([]desc, 0, 4)
	s = s[:3]
	s[2].name = "c"
	fmt.Println(s[2].name, s[:cap(s)][3].name == "", s[:4][3].sub == nil, s[:4][3].arr)
	p := &s[:4][3]
	p.index = 7
	fmt.Println(s[:4][3].index)

	// the new array of append has zero values after the length
	t := []desc{{name: "a"}}
	zero := true
	for i := 0; i < 20; i++ {
		old := cap(t)
		t = append(t, desc{name: "b", sub: &desc{}})
		full := t[:cap(t)]
		for j := len(t); j < len(full); j++ {
			if cap(t) != old && (full[j].name != "" || full[j].sub != nil || full[j].arr[1] != 0) {
				zero = false // a new array: zero values after the length
			}
			full[j].arr[1] = j // then overwritten by the next appends
		}
	}
	fmt.Println(len(t), zero, t[0].name, t[20].name, t[20].arr)

	// append into a slice made with spare capacity
	u := make([]desc, 1, 3)
	u = append(u, desc{name: "x"})
	fmt.Println(len(u), u[1].name, u[:3][2].sub == nil)
}
