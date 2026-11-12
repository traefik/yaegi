// Constraints: unions with ~ (named types allowed), unions without ~,
// constraints with methods, constraints combining methods and types,
// numeric generic code on many widths, and core type operations (len,
// range, index) on type parameters.
package main

import (
	"fmt"
	"strconv"
)

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type Float interface{ ~float32 | ~float64 }

type Number interface {
	Integer | Float
}

type Exact interface{ int | string } // no ~

type StringerInt interface {
	~int
	String() string
}

type Shower interface{ Show() string }

type Celsius float64
type ID int
type Code int8

func (i ID) String() string { return "ID-" + strconv.Itoa(int(i)) }

type Name string

func (n Name) Show() string { return "<" + string(n) + ">" }

func Sum[T Number](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

func Abs[T Integer | Float](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

func Max[T Number | ~string](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Twice[T Exact](v T) T { return v + v }

func Describe[T StringerInt](v T) string { return fmt.Sprint(v.String(), " = ", int(v)+1) }

func ShowAll[T Shower](xs []T) string {
	s := ""
	for _, x := range xs {
		s += x.Show()
	}
	return s
}

// Core type: a type set of slices of the same element type.
func First[S ~[]E, E any](s S) (E, bool) {
	if len(s) == 0 {
		var zero E
		return zero, false
	}
	return s[0], true
}

func Len[T ~string | ~[]byte](v T) int { return len(v) }

func Bytes[T ~string | ~[]byte](v T) []byte {
	var out []byte
	for i := 0; i < len(v); i++ {
		out = append(out, v[i])
	}
	return out
}

func MapLen[M ~map[K]V, K comparable, V any](m M) int { return len(m) }

func SendAll[C ~chan E, E any](c C, xs ...E) {
	for _, x := range xs {
		c <- x
	}
	close(c)
}

func Overflow[T Integer](x T) T { return x * 2 }

func Div[T Integer](a, b T) (q T, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	return a / b, nil
}

func Shift[T Integer](x T, n uint) (T, T) { return x << n, x >> n }

type Bits []byte
type MyMap map[string]int

func main() {
	fmt.Println(Sum(1, 2, 3), Sum(1.5, 2.25), Sum[int8](100, 27), Sum[uint8](200, 100))
	fmt.Println(Sum(Celsius(1.5), Celsius(2)), Sum(ID(1), ID(2)))
	fmt.Printf("%T %T\n", Sum(Celsius(0)), Sum(Code(1)))
	fmt.Println(Abs(-3), Abs(int8(-128)), Abs(-2.5), Abs(Celsius(-1)), Abs(uint(3)))
	fmt.Println(Max(3, 7), Max("abc", "abd"), Max(Name("x"), Name("y")), Max(-0.5, -1.5))
	fmt.Println(Twice(21), Twice("ab"))
	fmt.Println(Describe(ID(41)))
	fmt.Println(ShowAll([]Name{"a", "b"}))
	fmt.Println(First([]int{9, 8}))
	fmt.Println(First(Bits{}))
	f, ok := First([]string{"s"})
	fmt.Println(f, ok)
	fmt.Println(Len("héllo"), Len([]byte{1, 2}), Len(Name("abc")), Len(Bits{1}))
	fmt.Println(Bytes("hi"), Bytes(Bits{7}))
	fmt.Println(MapLen(MyMap{"a": 1}), MapLen(map[int]bool{}))
	c := make(chan int, 3)
	SendAll(c, 1, 2, 3)
	for v := range c {
		fmt.Print(v, " ")
	}
	fmt.Println()
	fmt.Println(Overflow(int8(100)), Overflow(uint8(200)), Overflow(int16(-20000)), Overflow(uint32(1<<31)), Overflow(int64(1)<<62))
	fmt.Println(Div(7, 2))
	fmt.Println(Div(-7, 2))
	fmt.Println(Div(int8(-128), -1))
	fmt.Println(Div(uint(1), 0))
	fmt.Println(Shift(int8(-64), 1))
	fmt.Println(Shift(uint16(1), 15))
	fmt.Println(Shift(int32(-1), 31))
}
