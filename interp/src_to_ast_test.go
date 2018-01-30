package interp

import (
	"fmt"
	"testing"
)

func TestSrcToAst(t *testing.T) {
	src := `
package main

func main() {
	println(1)
}
`
	root := SrcToAst(src)
	fmt.Println(root.index)
}
