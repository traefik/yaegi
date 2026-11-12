package main

import (
	"fmt"
	"go/ast"
)

func NewBadExpr() ast.Expr {
	return &ast.BadExpr{}
}

func main() {
	fmt.Printf("%T\n", NewBadExpr().(*ast.BadExpr))
}

// Output:
// *ast.BadExpr
