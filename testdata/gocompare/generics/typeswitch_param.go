// Type switches and assertions on type parameters through any(x): the
// dynamic type is the type argument, named types are distinct from their
// underlying type, and generic code chooses behaviour per type.
package main

import (
	"fmt"
	"strconv"
)

type MyInt int
type MyStr string

func (m MyInt) String() string { return "MyInt(" + strconv.Itoa(int(m)) + ")" }

func Kind[T any](x T) string {
	switch v := any(x).(type) {
	case nil:
		return "nil"
	case int:
		return "int " + strconv.Itoa(v)
	case string:
		return "string " + v
	case fmt.Stringer:
		return "stringer " + v.String()
	case []T:
		return fmt.Sprint("slice of T ", len(v))
	case error:
		return "error " + v.Error()
	default:
		return fmt.Sprintf("other %T", v)
	}
}

func Stringify[T any](xs []T) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		switch v := any(x).(type) {
		case int:
			out[i] = strconv.Itoa(v)
		case float64:
			out[i] = strconv.FormatFloat(v, 'f', 2, 64)
		case bool:
			out[i] = strconv.FormatBool(v)
		case string:
			out[i] = strconv.Quote(v)
		default:
			out[i] = fmt.Sprint(v)
		}
	}
	return out
}

func Is[T, U any](x T) bool {
	_, ok := any(x).(U)
	return ok
}

func Cast[U, T any](x T) (u U, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	return any(x).(U), nil
}

func Same[T, U any]() bool {
	var t T
	_, ok := any(&t).(*U)
	return ok
}

type Number interface{ ~int | ~float64 }

func Double[T Number](x T) T {
	switch any(x).(type) {
	case MyInt:
		return x * 10 // a special case for MyInt
	}
	return x * 2
}

func main() {
	fmt.Println(Kind(1))
	fmt.Println(Kind("s"))
	fmt.Println(Kind(MyInt(3)))
	fmt.Println(Kind(MyStr("m")))
	fmt.Println(Kind[any](nil))
	fmt.Println(Kind[error](nil))
	fmt.Println(Kind(fmt.Errorf("e")))
	fmt.Println(Kind[any]([]any{1, 2}))
	fmt.Println(Kind(2.5))
	fmt.Println(Kind([]int{1}))
	fmt.Println(Stringify([]int{1, 2}), Stringify([]float64{1.5}), Stringify([]any{true, "x", nil, 3}))
	fmt.Println(Is[int, int](1), Is[MyInt, int](1), Is[int, any](1), Is[MyInt, fmt.Stringer](1), Is[int, fmt.Stringer](1))
	fmt.Println(Cast[int](5))
	fmt.Println(Cast[string](5))
	fmt.Println(Cast[fmt.Stringer](MyInt(1)))
	fmt.Println(Cast[int](MyInt(1)))
	fmt.Println(Same[int, int](), Same[int, MyInt](), Same[[]byte, []uint8](), Same[rune, int32]())
	fmt.Println(Double(3), Double(MyInt(3)), Double(1.25))
}
