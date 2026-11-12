// Three-index slicing: len and cap set by the max index, append past the
// limit copies, slicing again within cap, slicing of arrays and pointers to
// arrays with three indices, and the omitted low index.
package main

import "fmt"

func main() {
	a := []int{0, 1, 2, 3, 4, 5, 6, 7}
	for _, c := range [][3]int{{0, 0, 0}, {0, 2, 4}, {1, 3, 3}, {2, 5, 8}, {8, 8, 8}, {3, 3, 6}} {
		s := a[c[0]:c[1]:c[2]]
		fmt.Println(c, s, len(s), cap(s))
	}
	s := a[2:4:6]
	fmt.Println(s[:cap(s)], len(s[:cap(s)]))
	t := s[1:4]
	fmt.Println(t, len(t), cap(t))
	s = append(s, 40)
	fmt.Println(a, s)
	s = append(s, 50)
	fmt.Println(a, s)
	s = append(s, 60)
	s[0] = -1
	fmt.Println(a, s)

	arr := [6]string{"a", "b", "c", "d", "e", "f"}
	x := arr[1:2:3]
	fmt.Println(x, len(x), cap(x))
	p := &arr
	y := p[:3:4]
	fmt.Println(y, len(y), cap(y))
	y[0] = "Z"
	fmt.Println(arr)
	z := arr[:0:0]
	fmt.Println(z, len(z), cap(z), z == nil)
	z = append(z, "new")
	fmt.Println(arr[0], z)

	// Make with len and cap then three-index slice.
	m := make([]int, 5, 20)
	m2 := m[1:3:10]
	fmt.Println(len(m2), cap(m2), cap(m[5:]), cap(m[:0]), cap(m[19:20]), len(m[:20]))

	// Strings cannot use 3 indices; byte slices can.
	bs := []byte("abcdef")
	b2 := bs[1:3:4]
	b2 = append(b2, 'X')
	fmt.Println(string(bs), string(b2))
	b2 = append(b2, 'Y')
	b2[0] = 'Q'
	fmt.Println(string(bs), string(b2))
}
