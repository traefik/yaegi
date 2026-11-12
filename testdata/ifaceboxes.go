// Values in interfaces: the VM shares one box per type for small integers,
// bools and the empty string (vm/boxes.go). Equality, type switches,
// printing, map keys and method calls must not see it.
package main

import (
	"errors"
	"fmt"
	"strconv"
)

type Kind int

func (k Kind) String() string { return "K" + strconv.Itoa(int(k)) }

type Code uint8

type Err int

func (e Err) Error() string { return fmt.Sprint("err ", int(e)) }

var errThree error = Err(3)

func kind(x any) string {
	switch v := x.(type) {
	case Kind:
		return "Kind " + v.String()
	case int:
		return "int " + strconv.Itoa(v)
	case Code:
		return fmt.Sprint("Code ", uint8(v))
	case bool:
		return fmt.Sprint("bool ", v)
	case string:
		return fmt.Sprintf("string %q", v)
	case *int:
		return fmt.Sprint("*int nil ", v == nil)
	case []byte:
		return fmt.Sprint("[]byte nil ", v == nil)
	}
	return "other"
}

func main() {
	var xs []any
	for i := -2; i < 260; i += 37 {
		xs = append(xs, i, Kind(i), Code(i))
	}
	xs = append(xs, true, false, "", "x", (*int)(nil), []byte(nil), Kind(0), 0)
	for _, x := range xs {
		fmt.Print(kind(x), "; ")
	}
	fmt.Println()
	fmt.Println(any(1) == any(1), any(1) == any(Kind(1)), any("") == any(""), any(Kind(2)) == any(Kind(3)))
	m := map[any]int{}
	for i := 0; i < 10; i++ {
		m[i%3]++
		m[Kind(i%2)]++
	}
	fmt.Println(len(m), m[0], m[1], m[2], m[Kind(0)], m[Kind(1)])
	var err error = Err(3)
	fmt.Println(errors.Is(err, errThree), err == errThree, err)
	var s fmt.Stringer = Kind(7)
	fmt.Println(s, s.String())
	fmt.Printf("%v %d %T %v\n", any(5), any(Kind(5)), any(Code(5)), any(""))
}
