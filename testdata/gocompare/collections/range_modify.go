// Changing a slice during its range: the length is evaluated once, so
// appends are not visited, element writes ahead are visited, reslicing the
// variable does not change the loop, and the value is a copy.
package main

import "fmt"

type item struct {
	N    int
	Tags []string
}

func main() {
	s := []int{1, 2, 3}
	for i, v := range s {
		s = append(s, v*10)
		fmt.Print(i, v, " ")
	}
	fmt.Println(s)

	s = []int{1, 2, 3, 4}
	for i, v := range s {
		if i+1 < len(s) {
			s[i+1] *= 100
		}
		fmt.Print(v, " ")
	}
	fmt.Println(s)

	s = []int{1, 2, 3, 4, 5}
	for i, v := range s {
		s = s[:2]
		fmt.Print(i, v, " ")
	}
	fmt.Println(s)

	// The range uses the original slice header, but shares the array.
	s = make([]int, 3, 10)
	for i := range s {
		s = append(s[:i], i+7)
		s = s[:3]
	}
	fmt.Println(s)

	// The value variable is a copy of the element.
	items := []item{{1, nil}, {2, nil}}
	for _, it := range items {
		it.N *= 10
		it.Tags = append(it.Tags, "x")
	}
	fmt.Println(items)
	for i := range items {
		items[i].N *= 10
		items[i].Tags = append(items[i].Tags, "y")
	}
	fmt.Println(items)
	ptrs := []*item{{1, nil}, {2, nil}}
	for _, p := range ptrs {
		p.N++
	}
	fmt.Println(*ptrs[0], *ptrs[1])

	// Removing in place while filtering, with an index loop.
	f := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	out := f[:0]
	for _, v := range f {
		if v%3 != 0 {
			out = append(out, v)
		}
	}
	fmt.Println(out, f)

	g := []string{"a", "b", "c", "d"}
	for i := 0; i < len(g); i++ {
		if g[i] == "b" || g[i] == "c" {
			g = append(g[:i], g[i+1:]...)
			i--
		}
	}
	fmt.Println(g, len(g))

	// Changing the array during a range over the array copies nothing for
	// the pointer form, everything for the value form.
	arr := [3]int{1, 2, 3}
	var seen []int
	for _, v := range arr {
		arr[2] = 99
		seen = append(seen, v)
	}
	fmt.Println(seen, arr)
	arr = [3]int{1, 2, 3}
	seen = seen[:0]
	for _, v := range arr[:] {
		arr[2] = 99
		seen = append(seen, v)
	}
	fmt.Println(seen)

	// Swapping elements in a range.
	r := []int{1, 2, 3, 4, 5, 6}
	for i := range len(r) / 2 {
		r[i], r[len(r)-1-i] = r[len(r)-1-i], r[i]
	}
	fmt.Println(r)

	// Range over a string that changes.
	str := "abc"
	for i, c := range str {
		str += string(c)
		fmt.Print(i)
	}
	fmt.Println(str)
	// Break and continue with labels in nested ranges.
	grid := [][]int{{1, 2, 3}, {4, -1, 6}, {7, 8, 9}}
	total := 0
rows:
	for _, row := range grid {
		for _, v := range row {
			if v < 0 {
				continue rows
			}
			if v > 8 {
				break rows
			}
			total += v
		}
	}
	fmt.Println(total)
}
