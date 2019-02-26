package interp_test

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

// testCase represents an interpreter test case.
// Care must be taken when defining multiple test cases within the same interpreter
// context, as all declarations occur in the global scope and are therefore
// shared between multiple test cases.
// Hint: use different variables or package names in testcases to keep them uncoupled.
type testCase struct {
	id, src, res, err string // only id is mandatory
	skip              bool   // if true, skip this test case (used in case of known error)
	pre               func() // functions to execute prior eval src, or nil
}

func TestEvalArithmetic(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{id: "add0", src: "2 + 3", res: "5"},
		{id: "add1", src: "2.3 + 3", res: "5.3"},
		{id: "add2", src: "2 + 3.3", res: "5.3"},
		{id: "mul0", src: "2 * 3", res: "6"},
		{id: "mul1", src: "2.2 * 3", res: "6.6000000000000005"},
		{id: "mul2", src: "3 * 2.2", res: "6.6000000000000005"},
	})
}

func TestEvalDecl(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{id: "0", pre: func() { eval(t, i, "var i int = 2") }, src: "i", res: "2"},
		{id: "1", pre: func() { eval(t, i, "var j, k int = 2, 3") }, src: "j", res: "2"},
		{id: "2", pre: func() { eval(t, i, "var l, m int = 2, 3") }, src: "k", res: "3"},
		{id: "3", pre: func() { eval(t, i, "func f() int {return 4}") }, src: "f()", res: "4"},
		{id: "4", pre: func() { eval(t, i, `package foo; var I = 2`) }, src: "foo.I", res: "2"},
		{id: "5", pre: func() { eval(t, i, `package foo; func F() int {return 5}`) }, src: "foo.F()", res: "5"},
	})
}

func TestEvalImport(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type)
	runTests(t, i, []testCase{
		{id: "0", pre: func() { eval(t, i, `import "time"`) }, src: "2 * time.Second", res: "2s"},
	})
}

func TestEvalNil(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type)
	runTests(t, i, []testCase{
		{id: "0", src: "a := nil", err: "1:27: use of untyped nil"},
		{id: "1", pre: func() { eval(t, i, "func getNil() error {return nil}") }, src: "getNil()", res: "<nil>"},
		{
			id: "2",
			pre: func() {
				eval(t, i, `
					package bar

					func New() func(string) error {
						return func(v string) error {
							return nil
						}
					}
				`)
				v := eval(t, i, `bar.New()`)
				fn, ok := v.Interface().(func(string) error)
				if !ok {
					t.Fatal("conversion failed")
				}
				if res := fn("hello"); res != nil {
					t.Fatalf("expected nil, got %v", res)
				}
			},
		},
		{
			id: "3",
			pre: func() {
				eval(t, i, `
					import "fmt"

					type Foo struct{}

					func Hello() *Foo {
						fmt.Println("Hello")
						return nil
					}
				`)
			},
			src: "Hello()",
			res: "<invalid reflect.Value>",
		},
	})
}

func TestEvalStruct0(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{
			id: "0",
			pre: func() {
				eval(t, i, `
					type Fromage struct {
						Name string
						Call func(string) string
					}

					func f() string {
						a := Fromage{}
						a.Name = "test"
						a.Call = func(s string) string { return s }

						return a.Call(a.Name)
					}
				`)
			},
			src: "f()",
			res: "test",
		},
		{
			id: "1",
			pre: func() {
				eval(t, i, `
					type Fromage2 struct {
						Name string
						Call func(string) string
					}

					func f2() string {
						a := Fromage2{
							"test",
							func(s string) string { return s },
						}
						return a.Call(a.Name)
					}
				`)
			},
			src: "f2()",
			res: "test",
		},
	})
}

func TestEvalStruct1(t *testing.T) {
	i := interp.New(interp.Opt{})
	eval(t, i, `
type Fromage struct {
	Name string
	Call func(string) string
}

func f() string {
	a := Fromage{
		"test",
		func(s string) string { return s },
	}

	return a.Call(a.Name)
}
`)

	v := eval(t, i, `f()`)
	if v.Interface().(string) != "test" {
		t.Fatalf("expected test, got %v", v)
	}
}

func TestEvalComposite0(t *testing.T) {
	i := interp.New(interp.Opt{})
	eval(t, i, `
type T struct {
	a, b, c, d, e, f, g, h, i, j, k, l, m, n string
	o map[string]int
	p []string
}

var a = T{
	o: map[string]int{"truc": 1, "machin": 2},
	p: []string{"hello", "world"},
}
`)
	v := eval(t, i, `a.p[1]`)
	if v.Interface().(string) != "world" {
		t.Fatalf("expected world, got %v", v)
	}
}

func TestEvalCompositeBin0(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type)
	eval(t, i, `
import (
	"fmt"
	"net/http"
	"time"
)

func Foo() {
	http.DefaultClient = &http.Client{Timeout: 2 * time.Second}
}
`)
	http.DefaultClient = &http.Client{}
	eval(t, i, `Foo()`)
	if http.DefaultClient.Timeout != 2*time.Second {
		t.Fatalf("expected 2s, got %v", http.DefaultClient.Timeout)
	}
}

func TestEvalComparison(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{id: "0", src: `"hhh" > "ggg"`, res: "true"},
		{
			id: "1",
			src: `
				type Foo string
				type Bar string

				var a = Foo("test")
				var b = Bar("test")
				var c = a == b
			`,
			err: "7:13: mismatched types _.Foo and _.Bar",
		},
	})
}

func TestEvalCompositeArray(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{id: "0", src: "a := []int{1, 2, 7: 20, 30}", res: "[1 2 0 0 0 0 0 20 30]"},
	})
}

func TestEvalUnary(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{id: "0", src: "a := -1", res: "-1"},
		{id: "1", src: "b := +1", res: "1", skip: true},
		{id: "2", src: "c := !false", res: "true"},
	})
}

func runTests(t *testing.T, i *interp.Interpreter, tests []testCase) {
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			if test.skip {
				t.SkipNow()
			}
			if test.pre != nil {
				test.pre()
			}
			if test.src != "" {
				assertEval(t, i, test.src, test.err, test.res)
			}
		})
	}
}

func eval(t *testing.T, i *interp.Interpreter, src string) reflect.Value {
	t.Helper()
	res, err := i.Eval(src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func assertEval(t *testing.T, i *interp.Interpreter, src, expectedError, expectedRes string) {
	res, err := i.Eval(src)

	if expectedError != "" {
		if err == nil || err.Error() != expectedError {
			t.Fatalf("got %v, want %s", err, expectedError)
		}
		return
	}

	if err != nil {
		t.Fatalf("got an error %v", err)
	}

	if fmt.Sprintf("%v", res) != expectedRes {
		t.Fatalf("got %v, want %s", res, expectedRes)
	}
}
