package interp_test

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

type resultTestCase struct {
	desc, src string
	res       interface{}
}

func TestEvalFileResult(t *testing.T) {
	type Results = []interp.FileStatementResult
	type Import = interp.PackageImportResult
	type Func = interp.FunctionDeclarationResult
	type Type = interp.TypeDeclarationResult

	i := interp.New(interp.Options{})
	_ = i.Use(stdlib.Symbols)
	runResultTests(t, i, []resultTestCase{
		{desc: "bare import", src: `import "time"`, res: &Import{Path: "time"}},
		{desc: "named import", src: `import x "time"`, res: &Import{Name: "x", Path: "time"}},
		{desc: "multiple imports", src: "import (\ny \"time\"\nz \"fmt\"\n)", res: Results{&Import{Name: "y", Path: "time"}, &Import{Name: "z", Path: "fmt"}}},

		{desc: "func", src: `func foo() { }`, res: &Func{Name: "foo"}},

		{desc: "struct type", src: `type Foo struct {}`, res: &Type{Name: "Foo"}},
	})
}

func runResultTests(t *testing.T, i *interp.Interpreter, tests []resultTestCase) {
	t.Helper()

	for _, test := range tests {
		expected := test.res
		if stmtResult, ok := expected.(interp.FileStatementResult); ok {
			expected = []interp.FileStatementResult{stmtResult}
		}
		if stmtResults, ok := expected.([]interp.FileStatementResult); ok {
			expected = &interp.FileResult{Statements: stmtResults}
		}

		t.Run(test.desc, func(t *testing.T) {
			res, err := i.Eval(test.src)
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsValid() {
				t.Fatal("Result is not valid")
			}
			if !reflect.DeepEqual(expected, res.Interface()) {
				t.Fatalf("Got %v, expected %v", res, expected)
			}
		})
	}
}
