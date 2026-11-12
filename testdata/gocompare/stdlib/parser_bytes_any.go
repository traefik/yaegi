// A []byte given to native code in an any: go/parser takes a source of
// type string, []byte or io.Reader with a type switch. With -aot (and
// -opaque) the []byte was an opaque handle, and the parser said "invalid
// source".
package main

import (
	"fmt"
	"go/parser"
	"go/token"
)

func main() {
	var nilb []byte
	for _, src := range []any{"1 + 2", []byte("3 * 4"), []byte("x + y")[4:], nilb} {
		x, err := parser.ParseExprFrom(token.NewFileSet(), "e.go", src, 0)
		fmt.Printf("%T %T %v\n", src, x, err)
	}
	var src any = []byte("package p\nvar x = 1\n")
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	fmt.Println(f.Name.Name, len(f.Decls), err)
}
