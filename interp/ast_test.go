package interp

import (
	"fmt"
	"testing"
)

func TestAst(t *testing.T) {
	src := `
package main

func main() {
	println(1)
}
`
	root := Ast(src)
	fmt.Println(root.index)
}
