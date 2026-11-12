// A method value of a nil interface panics with a nil dereference; an
// assertion of a nil interface to its own type keeps the conversion error.
package main

import (
	"fmt"
	"io"
)

type I interface{ Val() int }

// J embeds I: go/ssa checks j.Val with j.(I), then converts j to I.
type J interface {
	I
	Other()
}

type T int

func (t T) Val() int { return int(t) }

func try(name string, f func()) {
	defer func() { fmt.Println(name, recover()) }()
	f()
}

func val[X I](x X) func() int { return x.Val }

func main() {
	var i I
	try("a", func() { f := i.Val; _ = f })
	try("b", func() { _ = i.(I) })
	try("b2", func() { _ = i.(I); f := i.Val; _ = f })
	try("c", func() { var r io.Reader; g := r.Read; _ = g })
	try("d", func() { f := val[I](nil); _ = f })
	i = T(4)
	try("e", func() { f := i.Val; fmt.Println("e", f()) })
	try("f", func() { fmt.Println("f", val[I](i)(), val(T(5))()) })
	try("g", func() {
		var j I
		defer func() { fmt.Println("g deferred") }()
		_ = j.Val
	})
	try("h", func() { var rc io.ReadCloser; g := rc.Read; _ = g })
	try("i", func() { var j J; f := j.Val; _ = f })
	try("j", func() { f := val[J](nil); _ = f })
	try("k", func() { var j J; _ = j.(I) })
}
