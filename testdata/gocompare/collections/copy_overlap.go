// copy: overlapping ranges in both directions, copy count, copy from a
// string, to and from nil and empty slices, and copy of arrays of structs.
package main

import "fmt"

func main() {
	a := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	n := copy(a[2:], a)
	fmt.Println(n, a)
	a = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	n = copy(a, a[3:])
	fmt.Println(n, a)
	a = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	n = copy(a[1:4], a[5:])
	fmt.Println(n, a)
	n = copy(a, a)
	fmt.Println(n, a)

	b := make([]byte, 3)
	n = copy(b, "hello")
	fmt.Println(n, string(b))
	b = make([]byte, 10)
	n = copy(b, "héllo")
	fmt.Println(n, b[:n], string(b[:n]))

	var nilS []int
	fmt.Println(copy(nilS, []int{1, 2}), copy([]int{1}, nilS), copy(nilS, nilS))
	fmt.Println(copy([]int{}, []int{1}))

	type S struct {
		A [2]int
		B string
	}
	src := []S{{[2]int{1, 2}, "x"}, {[2]int{3, 4}, "y"}}
	dst := make([]S, 3)
	fmt.Println(copy(dst, src), dst)
	src[0].A[0] = 100
	fmt.Println(dst[0].A, src[0].A)

	// copy into an array through a slice.
	var arr [5]int
	fmt.Println(copy(arr[:], []int{9, 8, 7}), arr)
	fmt.Println(copy(arr[3:], arr[:]), arr)

	// Shifting left and right in place.
	s := []string{"a", "b", "c", "d", "e"}
	copy(s[1:], s[:4])
	s[0] = "z"
	fmt.Println(s)
	copy(s, s[2:])
	fmt.Println(s)

	// copy of runes and float slices.
	r := make([]rune, 2)
	fmt.Println(copy(r, []rune("日本語")), string(r))
	fl := []float64{1.5, 2.5, 3.5}
	copy(fl[1:], fl)
	fmt.Println(fl)
}
