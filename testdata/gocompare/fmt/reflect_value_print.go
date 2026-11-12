// fmt prints a reflect.Value given as a direct argument as the value it
// holds, and a reflect.Value in a struct, a slice, a map or behind a
// pointer with its String method ("<int Value>", "<invalid Value>"), like
// the cases of TestSprintf of package fmt.
package main

import (
	"fmt"
	"log"
	"os"
	"reflect"
)

type SI struct {
	I any
}

type SV struct {
	V reflect.Value
}

type named int

func (n named) String() string { return fmt.Sprintf("named(%d)", int(n)) }

func main() {
	rv := reflect.ValueOf(3)
	zero := reflect.Value{}
	str := reflect.ValueOf("s")
	nm := reflect.ValueOf(named(4))

	// direct arguments: the value held
	fmt.Println(rv, str, nm)
	fmt.Printf("%v %d %q %v\n", rv, rv, str, nm)
	fmt.Println(fmt.Sprintf("%v", zero))
	fmt.Println(fmt.Sprint(rv, zero))
	args := []any{rv, nm}
	fmt.Println(args...)

	// not direct: the String method of reflect.Value
	fmt.Println(fmt.Sprintf("%v", &zero))
	fmt.Println(fmt.Sprintf("%v", &rv))
	fmt.Println(fmt.Sprintf("%v", SI{zero}))
	fmt.Println(fmt.Sprintf("%v", SI{rv}))
	fmt.Println(fmt.Sprintf("%+v", SV{rv}))
	fmt.Println(fmt.Sprintf("%v", SV{nm}))
	fmt.Println([]any{rv, str}, []reflect.Value{rv, zero})
	fmt.Println(map[string]reflect.Value{"a": rv})
	fmt.Println([]any{args})

	// a reflect.Value in an interface that is a Stringer
	var s fmt.Stringer = rv
	fmt.Println(s.String(), s)

	l := log.New(os.Stdout, "log: ", 0)
	l.Println(rv, SI{rv})
	l.Printf("%v %v", nm, []reflect.Value{nm})
}
