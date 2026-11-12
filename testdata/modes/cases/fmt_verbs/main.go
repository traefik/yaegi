// fmt: %v %+v %#v %T %s %d %q %x on named types, structs, slices, maps,
// arrays, funcs and channels of interpreted types (no methods).
package main

import "fmt"

type ID int
type Name string
type Bytes []byte
type Flags map[string]bool
type Pair struct {
	K Name
	V ID
}
type Nested struct {
	P     Pair
	PP    *Pair
	List  []Pair
	M     map[Name]ID
	Arr   [2]ID
	Iface any
	B     Bytes
	f     float64
}
type Fn func(int) int
type Ch chan Pair
type Gen[T any] struct{ V T }

func main() {
	n := Nested{P: Pair{"a", 1}, List: []Pair{{"b", 2}}, M: map[Name]ID{"z": 26, "y": 25}, Arr: [2]ID{3, 4}, Iface: Pair{"i", 9}, B: Bytes("hi"), f: 1.5}
	for _, verb := range []string{"%v", "%+v", "%#v", "%T", "%s", "%d", "%q", "%x"} {
		fmt.Printf(verb+"\n", n)
	}
	vals := []any{ID(5), Name("nm"), Bytes{1, 2}, Flags{"x": true}, Pair{"k", 7}, &Pair{"p", 8}, []Pair{{"s", 1}}, [1]Name{"arr"}, Gen[ID]{4}, Gen[Pair]{}, Ch(nil), Fn(nil), (*Pair)(nil)}
	for _, v := range vals {
		fmt.Printf("%v|%+v|%#v|%T|%s|%d|%q\n", v, v, v, v, v, v, v)
	}
	fmt.Println(fmt.Sprint(ID(1), Name("x"), 3, "y"), fmt.Sprintln(Pair{"l", 1}))
	fmt.Printf("%5d|%-5v|%05d|%x|%X|%o|%b|%e|%8.3f|%t\n", ID(42), Name("ab"), ID(7), ID(255), Name("hi"), ID(8), ID(5), 1234.5, 3.14159, true)
	fmt.Printf("%[2]v %[1]v %v\n", ID(1), Name("two"), Pair{})
	fmt.Printf("%!|%z|%d\n", Name("bad"), "s")
}
