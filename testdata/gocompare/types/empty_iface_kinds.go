// The empty interface holding a value of every kind: %v, %T, %#v, round
// trips through assertions, copies on boxing, and slices and maps of any.
package main

import (
	"fmt"
	"sort"
)

type Celsius float64
type Rec struct {
	Name string
	Next *Rec
}

func main() {
	ch := make(chan string, 1)
	var arr [3]int8
	vals := []any{
		true, int(-1), int8(-8), int16(-16), int32(-32), int64(-64),
		uint(1), uint8(8), uint16(16), uint32(32), uint64(64), uintptr(7),
		float32(1.25), float64(2.5), complex64(1 + 2i), complex128(3 - 4i),
		"str", 'r', byte('b'), []byte("bs"), []string{"a"}, arr, map[string]bool{"t": true},
		struct{}{}, Rec{Name: "r"}, Celsius(36.6), [0]int{}, []any{1, "x", nil},
	}
	for i, v := range vals {
		fmt.Printf("%2d %-20T %v | %#v\n", i, v, v, v)
	}
	fmt.Printf("%T %v\n", ch, cap(ch))
	var fn any = func(a int) string { return fmt.Sprint(a) }
	fmt.Printf("%T %v\n", fn, fn.(func(int) string)(3))

	// Boxing copies the value.
	a := [2]int{1, 2}
	var box any = a
	a[0] = 100
	fmt.Println(box, a)
	r := Rec{Name: "x"}
	box = r
	r.Name = "y"
	fmt.Println(box.(Rec).Name, r.Name)
	box = &r
	r.Name = "z"
	fmt.Println(box.(*Rec).Name)

	// Round trips.
	var x any = uint16(65535)
	u := x.(uint16)
	u++
	fmt.Println(u, x)
	x = Celsius(10)
	c := x.(Celsius) * 2
	fmt.Printf("%v %T\n", c, c)

	// Maps of any with mixed keys and values.
	m := map[string]any{"i": 1, "s": "two", "f": 3.0, "l": []int{4}, "m": map[string]int{"five": 5}, "n": nil}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s=%v(%T) ", k, m[k], m[k])
	}
	fmt.Println()
	fmt.Println(m)

	// Nested any.
	var nested any = []any{map[string]any{"a": []any{1, 2}}, any(nil)}
	inner := nested.([]any)[0].(map[string]any)["a"].([]any)[1].(int)
	fmt.Println(inner, nested)

	// Variadic ...any.
	show := func(args ...any) {
		fmt.Println(len(args), args)
	}
	show()
	show(nil)
	show(1, "a", nil)
	sl := []any{1, 2}
	show(sl...)
	show(sl)
}
