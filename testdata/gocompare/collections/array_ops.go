// More array operations: lengths from constants, len and cap as constants,
// methods on array types with value and pointer receivers, arrays of
// structs compared, arrays returned from functions and indexed directly,
// array of arrays of strings, and formatting verbs on arrays.
package main

import "fmt"

const N = 3

type Vec [N]float64

func (v Vec) Add(o Vec) Vec {
	for i := range v {
		v[i] += o[i]
	}
	return v
}

func (v *Vec) Scale(k float64) {
	for i := range v {
		v[i] *= k
	}
}

func (v Vec) Dot(o Vec) float64 {
	s := 0.0
	for i, x := range v {
		s += x * o[i]
	}
	return s
}

type Board [3][3]byte

func (b *Board) Set(r, c int, v byte) { b[r][c] = v }
func (b Board) String() string {
	s := ""
	for _, row := range b {
		for _, c := range row {
			if c == 0 {
				c = '.'
			}
			s += string(c)
		}
		s += "/"
	}
	return s
}

func fib() [10]int64 {
	var f [10]int64
	f[0], f[1] = 0, 1
	for i := 2; i < len(f); i++ {
		f[i] = f[i-1] + f[i-2]
	}
	return f
}

func main() {
	var a [N * 2]int
	const l = len(a)
	var b [l + 1]string
	fmt.Println(len(a), cap(a), len(b), l)

	v := Vec{1, 2, 3}
	w := v.Add(Vec{10, 20, 30})
	fmt.Println(v, w, v.Dot(w))
	v.Scale(2)
	fmt.Println(v)
	pv := &v
	fmt.Println(pv.Add(v), pv.Dot(v))

	var bd Board
	bd.Set(0, 0, 'X')
	bd.Set(1, 1, 'O')
	bd2 := bd
	bd2.Set(2, 2, 'X')
	fmt.Println(bd, bd2, bd == bd2)
	fmt.Printf("%v %s\n", bd, &bd2)

	fmt.Println(fib()[9], fib()[3], len(fib()))
	f := fib()
	s := f[:]
	s[0] = 100
	fmt.Println(f[0])

	type P struct {
		N string
		A [2]int
	}
	ps := [2]P{{"a", [2]int{1, 2}}, {"b", [2]int{3, 4}}}
	ps2 := ps
	fmt.Println(ps == ps2)
	ps2[1].A[1] = 0
	fmt.Println(ps == ps2, ps, ps2)

	words := [2][3]string{{"a", "b", "c"}, {"d", "e", "f"}}
	for _, row := range words {
		for j := range row {
			row[j] += "!"
		}
	}
	fmt.Println(words)
	for i := range words {
		for j := range words[i] {
			words[i][j] += "?"
		}
	}
	fmt.Println(words)

	bs := [4]byte{'g', 'o', 0xff, 0}
	fmt.Printf("%v %x %X %q %d\n", bs, bs, bs[:2], bs[:2], bs)
	fl := [3]float64{1, 0.5, -2.25}
	fmt.Printf("%v %.1f %g %#v\n", fl, fl, fl, fl)
	bools := [2]bool{true}
	fmt.Printf("%v %t %#v\n", bools, bools, bools)
	ptrArr := &[2]string{"x", "y"}
	fmt.Printf("%v %q\n", ptrArr, *ptrArr)

	// Array of arrays equality with mixed values.
	g1 := [2][2]int{{1, 2}, {3, 4}}
	g2 := [2][2]int{{1, 2}, {3, 4}}
	g2[1][0] = 3
	fmt.Println(g1 == g2, g1[1] == [2]int{3, 4})

	// Arrays as function values in maps and channels.
	ch := make(chan [3]int, 1)
	arr := [3]int{1, 2, 3}
	ch <- arr
	arr[0] = 0
	got := <-ch
	fmt.Println(got, arr)
	am := map[string][2]bool{"a": {true, false}}
	fmt.Println(am["a"][0], am["z"][1])

	// Array of empty structs and zero-size elements.
	var es [5]struct{}
	fmt.Println(len(es), es)
	// Compound assignment on array elements.
	c := [3]int{1, 2, 3}
	c[0] += 10
	c[1] <<= 3
	c[2] %= 2
	idx := 1
	c[idx]--
	fmt.Println(c)
}
