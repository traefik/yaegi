package vm

import (
	"strings"
	"testing"
)

// TestUnexportedIfaceError checks that a value of the program given to
// native code as an interface with unexported methods (ast.Expr) stops with
// a clear error, not with the reflect.Set panic of the wrapper.
func TestUnexportedIfaceError(t *testing.T) {
	m, spkg := buildSrc(t, `package p

import (
	"fmt"
	"go/ast"
)

type myExpr struct{ *ast.Ident }

func F() (msg string) {
	defer func() { msg = fmt.Sprint(recover()) }()
	ast.Inspect(&ast.ParenExpr{X: myExpr{ast.NewIdent("x")}}, func(n ast.Node) bool { return true })
	return "no panic"
}
`)
	got, _ := m.Call(m.funcOf(spkg.Func("F")), nil, nil).R.(string)
	want := "unsupported: a value of type p.myExpr cannot be given to native code as ast.Expr: the interface has the unexported method exprNode"
	if !strings.HasPrefix(got, want) {
		t.Errorf("got %q\nwant %q...", got, want)
	}
}
