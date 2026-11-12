// Compound assignment operators (+= -= *= /= %= &= |= ^= <<= >>= &^=) and
// ++ / -- on variables, struct fields, pointers, array and slice elements,
// map entries and nested values, for integers of each width and floats,
// with wrapping, and with side effects evaluated once.
package main

import "fmt"

type S struct {
	A int8
	B uint16
	C float64
	D [3]int32
	P *int64
	M map[string]uint8
}

var calls int

func idx(i int) int {
	calls++
	return i
}

func main() {
	var x int64 = 100
	x += 5
	x -= 7
	x *= 3
	x /= 4
	x %= 17
	x &= 0xF
	x |= 0x30
	x ^= 0x0F
	x <<= 3
	x >>= 1
	x &^= 0x8
	fmt.Println(x)
	var i8 int8 = 120
	i8 += 10
	fmt.Println(i8)
	i8 -= 100
	i8 *= 3
	fmt.Println(i8)
	var u8 uint8 = 3
	u8 -= 5
	u8 <<= 1
	fmt.Println(u8)
	u8++
	u8++
	fmt.Println(u8)
	var u16 uint16 = 0
	u16--
	fmt.Println(u16)
	var i32 int32 = 2147483647
	i32++
	fmt.Println(i32)
	i32--
	fmt.Println(i32)
	var u64 uint64
	u64--
	u64 /= 3
	fmt.Println(u64)
	var f float64 = 1.5
	f++
	f -= 0.25
	f /= 2
	fmt.Println(f)
	f--
	f--
	fmt.Println(f)
	var c complex128 = 1i
	c++
	c *= 2
	fmt.Println(c)
	s := S{A: 127, B: 65535, C: 0.5, P: new(int64), M: map[string]uint8{}}
	s.A++
	s.B++
	s.C *= 4
	s.D[1] += 7
	s.D[2]--
	*s.P += 9
	*s.P <<= 2
	s.M["k"]--
	s.M["k"] -= 4
	s.M["new"]++
	s.M["new"] += 254
	fmt.Println(s.A, s.B, s.C, s.D, *s.P, s.M)
	ps := &s
	ps.A--
	ps.D[0] -= 2
	ps.B /= 5
	(*ps).C++
	fmt.Println(ps.A, ps.D, ps.B, ps.C)
	arr := [4]int16{32767, -32768, 1, 2}
	arr[0]++
	arr[1]--
	arr[idx(2)] += 10
	arr[idx(3)] *= -3
	fmt.Println(arr, calls)
	sl := []uint32{0, 1, 2}
	sl[idx(0)]--
	sl[len(sl)-1] <<= 31
	sl[1] ^= 0xFFFFFFFF
	fmt.Println(sl, calls)
	m := map[int]int64{}
	for i := 0; i < 10; i++ {
		m[i%3] += int64(i)
		m[i%2] *= 2
	}
	fmt.Println(m)
	mm := map[string][]int8{"a": {1, 2}}
	mm["a"][1] += 126
	mm["a"][1]++
	fmt.Println(mm)
	ms := map[string]*S{"s": {}}
	ms["s"].A--
	ms["s"].D[2] += 5
	fmt.Println(ms["s"].A, ms["s"].D)
	nested := [][2]map[string]int{{{"x": 1}, {"y": 2}}}
	nested[0][1]["y"] *= 21
	nested[0][0]["z"]--
	fmt.Println(nested)
	// The index expression is evaluated once.
	calls = 0
	ks := []int{0, 0, 0}
	ks[idx(1)]++
	ks[idx(2)] += idx(5)
	fmt.Println(ks, calls)
	// Shift with a variable count of another type.
	var cnt uint8 = 3
	var v int64 = -1
	v <<= cnt
	v >>= cnt - 1
	fmt.Println(v)
	// Assign-op on a string and on a pointer to a float field.
	str := "a"
	str += "b"
	str += string(rune('c' + 1))
	pf := &s.C
	*pf -= 0.5
	fmt.Println(str, s.C)
	// Swap and multiple assignment with wrapping.
	a, b := uint8(200), uint8(100)
	a, b = a+b, a-b
	fmt.Println(a, b)
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		zero := 0
		y := 5
		y /= zero
		fmt.Println(y)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var mz map[string]int
		mz["a"]++
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var np *S
		np.A++
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		arr := []int{1}
		i := 3
		arr[i] += 1
	}()
}
