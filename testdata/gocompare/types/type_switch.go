// Type switches: single and multiple types per case (the variable keeps the
// interface type), the nil case, default, interfaces in cases, named and
// composite types, fallthrough-free ordering, and a switch with no variable.
package main

import (
	"errors"
	"fmt"
)

type MyInt int
type MyStr string
type Pair struct{ A, B int }

func (m MyInt) String() string { return fmt.Sprint("MyInt:", int(m)) }

func classify(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case int:
		return fmt.Sprintf("int %d", x+1)
	case int8, int16, int32:
		return fmt.Sprintf("small int %T %v", x, x)
	case uint, uint64:
		return fmt.Sprintf("unsigned %v", x)
	case float64:
		return fmt.Sprintf("float %.2f", x*2)
	case string:
		return "string " + x + "!"
	case MyStr:
		return "MyStr " + string(x)
	case fmt.Stringer:
		return "Stringer " + x.String()
	case error:
		return "error " + x.Error()
	case []int:
		return fmt.Sprint("[]int len ", len(x))
	case map[string]int:
		return fmt.Sprint("map len ", len(x))
	case Pair:
		return fmt.Sprint("Pair sum ", x.A+x.B)
	case *Pair:
		if x == nil {
			return "nil *Pair"
		}
		return fmt.Sprint("*Pair ", x.A)
	case func() int:
		return fmt.Sprint("func ", x())
	case chan int:
		return fmt.Sprint("chan cap ", cap(x))
	case bool, complex128:
		return fmt.Sprintf("bool or complex %v", x)
	case [2]string:
		return "array " + x[1]
	case struct{ N int }:
		return fmt.Sprint("anon ", x.N)
	default:
		return fmt.Sprintf("other %T", x)
	}
}

func firstMatch(v any) string {
	// Order matters: the first matching case wins.
	switch v.(type) {
	case fmt.Stringer:
		return "Stringer first"
	case MyInt:
		return "MyInt"
	}
	return "none"
}

func main() {
	vals := []any{
		nil, 1, int8(2), int16(3), int32(4), uint(5), uint64(6), 1.5, "s",
		MyStr("m"), MyInt(3), errors.New("boom"), []int{1, 2}, map[string]int{"a": 1},
		Pair{1, 2}, &Pair{A: 9}, (*Pair)(nil), func() int { return 7 }, make(chan int, 3),
		true, complex(1, 2), [2]string{"a", "b"}, struct{ N int }{4}, float32(1), []string{},
		MyInt(0), uintptr(1), int64(8),
	}
	for _, v := range vals {
		fmt.Println(classify(v))
	}
	fmt.Println(firstMatch(MyInt(1)), firstMatch(1))

	// A type switch with no variable, and a variable used in only some cases.
	for _, v := range []any{1, "a", nil} {
		switch v.(type) {
		case int, string:
			fmt.Print("int or string; ")
		case nil:
			fmt.Print("nil; ")
		}
	}
	fmt.Println()

	// A switch over an error value.
	var err error = fmt.Errorf("wrapped: %w", errors.New("inner"))
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		fmt.Println("unwrappable:", e.Unwrap())
	default:
		fmt.Println("plain")
	}

	// Initializer statement in a type switch.
	switch v := any(MyInt(5)); x := v.(type) {
	case MyInt:
		fmt.Println("init", x*2)
	}
}
