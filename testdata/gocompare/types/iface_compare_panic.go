// Comparison of interface values: equal dynamic types and values, different
// dynamic types, and the runtime panics for uncomparable dynamic types
// (slices, maps, funcs, structs and arrays holding them) recovered with
// their exact messages, also through switch and map keys.
package main

import "fmt"

type Named []int
type Wrap struct{ S []string }
type Arr [1]map[int]int

func eq(a, b any) (r string) {
	defer func() {
		if e := recover(); e != nil {
			r = fmt.Sprint("panic: ", e)
			if re, ok := e.(interface{ RuntimeError() }); ok {
				_ = re
				r += " (runtime error)"
			}
			if er, ok := e.(error); ok {
				r += " [" + er.Error() + "]"
			}
		}
	}()
	return fmt.Sprint(a == b)
}

func main() {
	fmt.Println(eq(1, 1), eq(1, 2), eq(1, "1"), eq(int32(1), int64(1)))
	fmt.Println(eq("a", "a"), eq(nil, nil), eq(nil, 0), eq(1.0, 1.0))
	fmt.Println(eq([2]int{1, 2}, [2]int{1, 2}), eq(struct{ A int }{1}, struct{ A int }{1}))
	fmt.Println(eq([]int{1}, []int{1}))
	fmt.Println(eq(map[int]int{}, map[int]int{}))
	fmt.Println(eq(func() {}, nil))
	fmt.Println(eq(Named{1}, Named{1}))
	fmt.Println(eq(Wrap{}, Wrap{}))
	fmt.Println(eq(Arr{}, Arr{}))
	fmt.Println(eq([1][]int{}, [1][]int{}))
	fmt.Println(eq(struct{ F func() }{}, struct{ F func() }{}))
	// Different dynamic types never panic: the types are compared first.
	fmt.Println(eq([]int{1}, 1), eq(map[int]int{}, "x"), eq(Named{}, []int{}))

	// Switch on an interface with an uncomparable dynamic type.
	sw := func(v any) (r string) {
		defer func() {
			if e := recover(); e != nil {
				r = fmt.Sprint("panic: ", e)
			}
		}()
		switch v {
		case 1:
			return "one"
		case "s":
			return "s"
		}
		return "none"
	}
	fmt.Println(sw(1), sw("s"), sw(2.5), sw([]int{}))

	// Map with interface keys.
	m := map[any]string{}
	put := func(k any) (r string) {
		defer func() {
			if e := recover(); e != nil {
				r = fmt.Sprint("panic: ", e)
			}
		}()
		m[k] = "v"
		return "ok"
	}
	fmt.Println(put(1), put("a"), put([2]int{}), put([]byte("x")), put(map[string]int{}), put(Wrap{}))
	fmt.Println(len(m))

	// Interface compared with a concrete value.
	var i any = 5
	fmt.Println(i == 5, i == int8(5), i != "5")
	var s fmt.Stringer
	fmt.Println(s == nil)
}
