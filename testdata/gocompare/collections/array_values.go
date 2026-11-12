// Arrays are values: assignment, function arguments, range copies, arrays in
// structs and arrays of arrays copy, == compares element by element, and
// arrays work as map keys.
package main

import "fmt"

type Pt struct {
	X, Y int
}

type Box struct {
	Name  string
	Cells [3]int
	Pts   [2]Pt
}

func modify(a [3]int) [3]int {
	a[0] = 100
	return a
}

func modifyPtr(a *[3]int) {
	a[0] = 100
}

func main() {
	a := [3]int{1, 2, 3}
	b := a
	b[0] = 9
	fmt.Println(a, b, a == b, a != b)
	c := modify(a)
	fmt.Println(a, c)
	modifyPtr(&a)
	fmt.Println(a, a == c)

	// Range over an array copies it first.
	arr := [3]int{1, 2, 3}
	for i, v := range arr {
		arr[2] = 30
		fmt.Print(i, v, " ")
	}
	fmt.Println(arr)
	arr = [3]int{1, 2, 3}
	for i, v := range &arr {
		arr[2] = 30
		fmt.Print(i, v, " ")
	}
	fmt.Println()

	// Arrays of arrays.
	var grid [3][4]int
	for i := range grid {
		for j := range grid[i] {
			grid[i][j] = i*10 + j
		}
	}
	row := grid[1]
	row[0] = -1
	fmt.Println(grid, row, len(grid), len(grid[0]))
	g2 := grid
	g2[2][3] = 0
	fmt.Println(grid[2], g2[2], grid == g2)

	// Arrays in structs.
	x := Box{Name: "x", Cells: [3]int{1, 2, 3}, Pts: [2]Pt{{1, 2}, {3, 4}}}
	y := x
	y.Cells[1] = 0
	y.Pts[0].X = 7
	fmt.Println(x, y, x == y)
	y = x
	fmt.Println(x == y)

	// Arrays as map keys.
	m := map[[2]int]string{}
	m[[2]int{1, 2}] = "a"
	m[[2]int{2, 1}] = "b"
	k := [2]int{1, 2}
	m[k] = "c"
	fmt.Println(len(m), m[[2]int{1, 2}], m[[2]int{0, 0}] == "", m)
	sk := map[[2]string]int{{"a", "b"}: 1, {"b", "a"}: 2}
	fmt.Println(sk[[2]string{"a", "b"}], sk)

	// Comparing arrays of floats and strings, zero-length arrays.
	f1 := [2]float64{1, 2}
	f2 := [2]float64{1, 2}
	fmt.Println(f1 == f2, [0]int{} == [0]int{}, [2]string{"a"} == [2]string{"a", ""})
	var e [0]int
	fmt.Println(len(e), e)

	// Array literals with indices and ellipsis.
	z := [...]string{2: "c", 0: "a", 5: "f"}
	fmt.Println(len(z), z, fmt.Sprintf("%q", z))
	w := [...]int{4: 1, 2, 1: 7}
	fmt.Println(len(w), w)

	// Pointer to array: indexing and len.
	pa := &[4]int{1, 2, 3, 4}
	pa[1] = 20
	fmt.Println(len(pa), cap(pa), *pa, pa[1:3])

	// Array of interfaces compared.
	i1 := [2]any{1, "x"}
	i2 := [2]any{1, "x"}
	fmt.Println(i1 == i2, i1 == [2]any{1, "y"})
	fmt.Printf("%v %d %x %T\n", a, a, [2]byte{10, 255}, grid)
}
