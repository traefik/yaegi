// The elements of a slice of a byte type of the program with methods keep
// their methods for fmt: the slice is kept as Go bytes, and its elements
// were native uint8 values (TestSprintf of package fmt, byteFormatter).
package main

import (
	"fmt"
	"strings"
)

type byteFormatter byte

func (byteFormatter) Format(f fmt.State, _ rune) { fmt.Fprint(f, "X") }

type upper byte

func (u upper) String() string { return strings.ToUpper(string(rune(u))) }

type plain byte

func main() {
	b := []byteFormatter{'h', 'i'}
	fmt.Printf("%v %s %q %#v\n", b, b, b, b)
	fmt.Println(fmt.Sprint(b), len(fmt.Sprintf("%x", b)))

	u := []upper("abc")
	fmt.Printf("%v %s %d\n", u, u, u)
	fmt.Println(u[1], u)

	p := []plain("ok")
	fmt.Printf("%v %s %x %#v\n", p, p, p, p)
	arr := [2]upper{'x', 'y'}
	fmt.Println(arr, &arr)
}
