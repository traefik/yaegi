// Type switches: single and multiple types per case, nil case, the bound
// variable's type in each case, interfaces as cases, default, fallthrough-free
// order of case matching, and type switches on errors and on named types.
package main

import (
	"errors"
	"fmt"
	"io"
)

type myInt int
type stringer interface{ String() string }
type point struct{ x, y int }

func (p point) String() string { return fmt.Sprintf("(%d,%d)", p.x, p.y) }

type ptrStringer struct{ n int }

func (p *ptrStringer) String() string { return fmt.Sprint("ps", p.n) }

type myErr struct{ code int }

func (e *myErr) Error() string {
	if e == nil {
		return "nil myErr"
	}
	return fmt.Sprint("code ", e.code)
}

func describe(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return fmt.Sprintf("nil %T", x)
	case int, int64:
		return fmt.Sprintf("int-ish %T %v", x, x)
	case myInt:
		return fmt.Sprintf("myInt %d", x+1)
	case string:
		return "string len " + fmt.Sprint(len(x))
	case error:
		return "error " + x.Error()
	case stringer:
		return "stringer " + x.String()
	case []int:
		return fmt.Sprint("slice ", len(x), x == nil)
	case func() int:
		return fmt.Sprint("func ", x())
	case map[string]int:
		return fmt.Sprint("map ", len(x))
	case chan int:
		return fmt.Sprint("chan ", cap(x))
	case *int:
		if x == nil {
			return "nil *int"
		}
		return fmt.Sprint("*int ", *x)
	default:
		return fmt.Sprintf("other %T", x)
	}
}

func main() {
	n := 42
	var nilp *int
	var nilErr *myErr
	vals := []interface{}{
		nil, 1, int64(2), myInt(3), "hey", errors.New("boom"), &myErr{7},
		point{1, 2}, &point{3, 4}, &ptrStringer{5}, ptrStringer{6}, []int(nil), []int{1, 2},
		func() int { return 9 }, map[string]int{"a": 1}, make(chan int, 3), &n, nilp,
		3.5, true, struct{}{}, nilErr, io.EOF, [2]int{},
	}
	for i, v := range vals {
		fmt.Println(i, describe(v))
	}
	var e error = &myErr{3}
	switch e.(type) {
	case interface{ Unwrap() error }:
		fmt.Println("unwrapper")
	case *myErr:
		fmt.Println("myErr")
	}
	switch v := e.(type) {
	case *myErr:
		v.code = 99
	}
	fmt.Println(e)
	var s stringer = point{}
	switch s.(type) {
	case fmt.Stringer:
		fmt.Println("fmt.Stringer first")
	case point:
		fmt.Println("point")
	}
	switch t := interface{}(myInt(5)).(type) {
	case int:
		fmt.Println("int", t)
	default:
		fmt.Printf("default %T %v\n", t, t)
	}
	for _, v := range []interface{}{1, "a", 2.5} {
		switch v.(type) {
		case int:
			fmt.Println("int case")
			continue
		case string:
			fmt.Println("string case, break")
			break
		}
		fmt.Println("after switch for", v)
	}
}
