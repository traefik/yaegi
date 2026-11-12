package main

import "fmt"

type AST struct {
	Num      int
	Children []AST
}

func newAST(num int, root AST, children ...AST) AST {
	return AST{num, append([]AST{root}, children...)}
}

func main() {
	ast := newAST(1, AST{}, AST{})
	fmt.Println(ast)
}

// Output:
// {1 [{0 []} {0 []}]}
