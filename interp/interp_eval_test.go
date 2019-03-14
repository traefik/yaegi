package interp_test

import (
	"fmt"
	"log"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/containous/dyngo/interp"
	"github.com/containous/dyngo/stdlib"
)

func init() { log.SetFlags(log.Lshortfile) }

// testCase represents an interpreter test case.
// Care must be taken when defining multiple test cases within the same interpreter
// context, as all declarations occur in the global scope and are therefore
// shared between multiple test cases.
// Hint: use different variables or package names in testcases to keep them uncoupled.
type testCase struct {
	desc, src, res, err string
	skip                string // if not empty, skip this test case (used in case of known error)
	pre                 func() // functions to execute prior eval src, or nil
}

func TestEvalArithmetic(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{desc: "add_II", src: "2 + 3", res: "5"},
		{desc: "add_FI", src: "2.3 + 3", res: "5.3"},
		{desc: "add_IF", src: "2 + 3.3", res: "5.3"},
		{desc: "add_SS", src: `"foo" + "bar"`, res: "foobar"},
		{desc: "add_SI", src: `"foo" + 1`, err: "1:22: illegal operand types for '+' operator"},
		{desc: "sub_SS", src: `"foo" - "bar"`, err: "1:22: illegal operand types for '-' operator"},
		{desc: "sub_II", src: "7 - 3", res: "4"},
		{desc: "sub_FI", src: "7.2 - 3", res: "4.2"},
		{desc: "sub_IF", src: "7 - 3.2", res: "3.8"},
		{desc: "mul_II", src: "2 * 3", res: "6"},
		{desc: "mul_FI", src: "2.2 * 3", res: "6.6000000000000005"},
		{desc: "mul_IF", src: "3 * 2.2", res: "6.6000000000000005"},
		{desc: "rem_FI", src: "8.0 % 4", err: "1:22: illegal operand types for '%' operator"},
	})
}

func TestEvalAssign(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{src: `a := "Hello"; a += " world"`, res: "Hello world"},
		{src: `b := "Hello"; b += 1`, err: "1:36: illegal operand types for '+=' operator"},
		{src: `c := "Hello"; c -= " world"`, err: "1:36: illegal operand types for '-=' operator"},
		{src: "e := 64.0; e %= 64", err: "1:33: illegal operand types for '%=' operator"},
	})
}

func TestEvalDecl(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{pre: func() { eval(t, i, "var i int = 2") }, src: "i", res: "2"},
		{pre: func() { eval(t, i, "var j, k int = 2, 3") }, src: "j", res: "2"},
		{pre: func() { eval(t, i, "var l, m int = 2, 3") }, src: "k", res: "3"},
		{pre: func() { eval(t, i, "func f() int {return 4}") }, src: "f()", res: "4"},
		{pre: func() { eval(t, i, `package foo; var I = 2`) }, src: "foo.I", res: "2"},
		{pre: func() { eval(t, i, `package foo; func F() int {return 5}`) }, src: "foo.F()", res: "5"},
	})
}

func TestEvalFunc(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{src: `(func () string {return "ok"})()`, res: "ok"},
		{src: `(func () (res string) {res = "ok"; return})()`, res: "ok"},
		{src: `(func () int {f := func() (a, b int) {a, b = 3, 4; return}; x, y := f(); return x+y})()`, res: "7"},
		{src: `(func () int {f := func() (a int, b, c int) {a, b, c = 3, 4, 5; return}; x, y, z := f(); return x+y+z})()`, res: "12"},
		{src: `(func () int {f := func() (a, b, c int) {a, b, c = 3, 4, 5; return}; x, y, z := f(); return x+y+z})()`, res: "12"},
	})
}

func TestEvalImport(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value)
	runTests(t, i, []testCase{
		{pre: func() { eval(t, i, `import "time"`) }, src: "2 * time.Second", res: "2s"},
	})
}

func TestEvalNil(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value)
	runTests(t, i, []testCase{
		{desc: "assign nil", src: "a := nil", err: "1:22: use of untyped nil"},
		{desc: "return nil", pre: func() { eval(t, i, "func getNil() error {return nil}") }, src: "getNil()", res: "<nil>"},
		{
			desc: "return func which return error",
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
					t.Fatalf("got %v, want nil", res)
				}
			},
		},
		{
			desc: "return nil pointer",
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
			desc: "func field in struct",
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
			desc: "literal func field in struct",
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
		t.Fatalf("got %v, want test", v)
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
		t.Fatalf("got %v, want word", v)
	}
}

func TestEvalCompositeBin0(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value)
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
		t.Fatalf("got %v, want 2s", http.DefaultClient.Timeout)
	}
}

func TestEvalComparison(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{src: `"hhh" > "ggg"`, res: "true"},
		{
			desc: "mismatched types",
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
		{src: "a := []int{1, 2, 7: 20, 30}", res: "[1 2 0 0 0 0 0 20 30]"},
	})
}

func TestEvalUnary(t *testing.T) {
	i := interp.New(interp.Opt{})
	runTests(t, i, []testCase{
		{src: "a := -1", res: "-1"},
		{src: "b := +1", res: "1", skip: "BUG"},
		{src: "c := !false", res: "true"},
	})
}

func TestEvalMethod(t *testing.T) {
	i := interp.New(interp.Opt{})
	eval(t, i, `
		type Root struct {
			Name string
		}

		type One struct {
			Root
		}

		type Hi interface {
			Hello() string
		}

		func (r *Root) Hello() string { return "Hello " + r.Name }

		var r = Root{"R"}
		var o = One{r}
		var root interface{} = &Root{Name: "test1"}
		var one interface{} = &One{Root{Name: "test2"}}
	`)
	runTests(t, i, []testCase{
		{src: "r.Hello()", res: "Hello R"},
		{src: "(&r).Hello()", res: "Hello R"},
		{src: "o.Hello()", res: "Hello R"},
		{src: "(&o).Hello()", res: "Hello R"},
		{src: "root.(Hi).Hello()", res: "Hello test1"},
		{src: "one.(Hi).Hello()", res: "Hello test2"},
	})
}

func runTests(t *testing.T, i *interp.Interpreter, tests []testCase) {
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.skip != "" {
				t.Skip(test.skip)
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
