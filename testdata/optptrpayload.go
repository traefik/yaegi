// Pointers to different variables, to two elements of one array and to
// two fields of one struct, given to native code as an io.Writer. The VM
// boxes each value once per type and value (optPayload and
// optSameShallow in vm/optwrap.go): two pointers are the same value only
// when they point to the same variable, and each writer must write into
// its own variable.
package main

import (
	"bufio"
	"fmt"
	"io"
)

type C int

func (c *C) Write(p []byte) (int, error) {
	*c += C(len(p))
	return len(p), nil
}

type pair struct{ x, y C }

func main() {
	var x, y C
	io.WriteString(&x, "abc")
	io.WriteString(&y, "de")
	io.WriteString(&x, "f")
	fmt.Println(x, y)

	var a [3]C
	io.WriteString(&a[0], "ab")
	io.WriteString(&a[1], "xyz")
	io.WriteString(&a[2], "q")
	fmt.Println(a)

	// both writers alive at the same time
	w0 := bufio.NewWriter(&a[0])
	w1 := bufio.NewWriter(&a[1])
	w1.WriteString("1234")
	w0.WriteString("5")
	w1.Flush()
	w0.Flush()
	fmt.Println(a)

	var p pair
	fmt.Fprint(&p.y, "abcd")
	fmt.Fprint(&p.x, "e")
	fmt.Println(p.x, p.y)

	// the same pointer twice: the same element
	io.WriteString(&a[2], "zz")
	io.WriteString(&a[2], "z")
	fmt.Println(a)
}
