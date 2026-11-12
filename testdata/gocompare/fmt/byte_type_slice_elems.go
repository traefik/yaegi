// The elements of a slice of a byte type of the program keep the name of
// the type when fmt walks them: the VM holds []U8 as Go bytes, and the
// twin reflect of the AOT copy of fmt gave each element as a native uint8.
package main

import "fmt"

type U8 uint8

type Bs8 []U8

func main() {
	v := Bs8{104, 105}
	for _, verb := range []string{"%v", "%d", "%t", "%g", "%.2f", "%x", "%s", "%q"} {
		fmt.Printf(verb+"\n", v)
	}
	fmt.Printf("%v %t\n", []U8{1, 2}, []U8{3})
	fmt.Printf("%#v\n", v)
}
