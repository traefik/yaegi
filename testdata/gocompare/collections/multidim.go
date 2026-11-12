// Multi-dimensional slices: jagged rows, rows sharing one backing slice,
// transposition, and appends to a row that do or do not affect others.
package main

import (
	"fmt"
	"strings"
)

func main() {
	rows, cols := 4, 3
	backing := make([]int, rows*cols)
	m := make([][]int, rows)
	for i := range m {
		m[i] = backing[i*cols : (i+1)*cols : (i+1)*cols]
	}
	for i := range m {
		for j := range m[i] {
			m[i][j] = i*cols + j
		}
	}
	fmt.Println(m, backing)
	m[1] = append(m[1], 99)
	m[1][0] = -1
	fmt.Println(m, backing)

	// Shared without the third index: append overwrites the next row.
	backing2 := make([]int, 6)
	r0 := backing2[0:3]
	r1 := backing2[3:6]
	r0 = append(r0, 42)
	fmt.Println(r0, r1)

	// Jagged.
	tri := make([][]string, 5)
	for i := range tri {
		for j := 0; j <= i; j++ {
			tri[i] = append(tri[i], strings.Repeat("*", j+1))
		}
	}
	for _, r := range tri {
		fmt.Println(len(r), r)
	}

	// Transpose.
	a := [][]int{{1, 2, 3}, {4, 5, 6}}
	t := make([][]int, len(a[0]))
	for j := range t {
		t[j] = make([]int, len(a))
		for i := range a {
			t[j][i] = a[i][j]
		}
	}
	fmt.Println(t)

	// Three dimensions.
	cube := make([][][]byte, 2)
	for i := range cube {
		cube[i] = make([][]byte, 2)
		for j := range cube[i] {
			cube[i][j] = []byte{byte('a' + i), byte('0' + j)}
		}
	}
	fmt.Printf("%q %s\n", cube, cube[1][0])

	// Slice of arrays and array of slices.
	sa := [][2]int{{1, 2}, {3, 4}}
	sa[0][1] = 20
	v := sa[1]
	v[0] = 0
	fmt.Println(sa, v)
	as := [2][]int{{1}, {2, 3}}
	as2 := as
	as2[1][0] = 200
	as2[0] = []int{9}
	fmt.Println(as, as2)

	// Matrix multiply.
	x := [][]float64{{1, 2}, {3, 4}}
	y := [][]float64{{5, 6}, {7, 8}}
	z := make([][]float64, 2)
	for i := range z {
		z[i] = make([]float64, 2)
		for j := range z[i] {
			for k := range y {
				z[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	fmt.Println(z)
	fmt.Println(len(m[1:]), len(m[0][:0]), cap(m[3]))
}
