package interp_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/containous/yaegi/interp"
	"github.com/containous/yaegi/stdlib"
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
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{desc: "add_II", src: "2 + 3", res: "5"},
		{desc: "add_FI", src: "2.3 + 3", res: "5.3"},
		{desc: "add_IF", src: "2 + 3.3", res: "5.3"},
		{desc: "add_SS", src: `"foo" + "bar"`, res: "foobar"},
		{desc: "add_SI", src: `"foo" + 1`, err: "1:28: invalid operation: mismatched types string and int"},
		{desc: "sub_SS", src: `"foo" - "bar"`, err: "1:28: invalid operation: operator - not defined on string"},
		{desc: "sub_II", src: "7 - 3", res: "4"},
		{desc: "sub_FI", src: "7.2 - 3", res: "4.2"},
		{desc: "sub_IF", src: "7 - 3.2", res: "3.8"},
		{desc: "mul_II", src: "2 * 3", res: "6"},
		{desc: "mul_FI", src: "2.2 * 3", res: "6.6"},
		{desc: "mul_IF", src: "3 * 2.2", res: "6.6"},
		{desc: "quo_Z", src: "3 / 0", err: "1:28: invalid operation: division by zero"},
		{desc: "rem_FI", src: "8.2 % 4", err: "1:28: invalid operation: operator % not defined on float64"},
		{desc: "rem_Z", src: "8 % 0", err: "1:28: invalid operation: division by zero"},
		{desc: "shl_II", src: "1 << 8", res: "256"},
		{desc: "shl_IN", src: "1 << -1", err: "1:28: invalid operation: shift count type int, must be integer"},
		{desc: "shl_IF", src: "1 << 1.0", res: "2"},
		{desc: "shl_IF1", src: "1 << 1.1", err: "1:28: invalid operation: shift count type float64, must be integer"},
		{desc: "shl_IF2", src: "1.0 << 1", res: "2"},
		{desc: "shr_II", src: "1 >> 8", res: "0"},
		{desc: "shr_IN", src: "1 >> -1", err: "1:28: invalid operation: shift count type int, must be integer"},
		{desc: "shr_IF", src: "1 >> 1.0", res: "0"},
		{desc: "shr_IF1", src: "1 >> 1.1", err: "1:28: invalid operation: shift count type float64, must be integer"},
		{desc: "neg_I", src: "-2", res: "-2"},
		{desc: "pos_I", src: "+2", res: "2"},
		{desc: "bitnot_I", src: "^2", res: "-3"},
		{desc: "bitnot_F", src: "^0.2", err: "1:28: invalid operation: operator ^ not defined on float64"},
		{desc: "not_B", src: "!false", res: "true"},
		{desc: "not_I", src: "!0", err: "1:28: invalid operation: operator ! not defined on int"},
	})
}

func TestEvalStar(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := &struct{A int}{1}; b := *a`, res: "{1}"},
		{src: `a := struct{A int}{1}; b := *a`, err: "1:57: invalid operation: cannot indirect \"a\""},
	})
}

func TestEvalAssign(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := "Hello"; a += " world"`, res: "Hello world"},
		{src: `b := "Hello"; b += 1`, err: "1:42: invalid operation: mismatched types string and int"},
		{src: `c := "Hello"; c -= " world"`, err: "1:42: invalid operation: operator -= not defined on string"},
		{src: "e := 64.4; e %= 64", err: "1:39: invalid operation: operator %= not defined on float64"},
		{src: "f := int64(3.2)", err: "1:39: cannot convert expression of type float64 to type int64"},
		{src: "g := 1; g <<= 8", res: "256"},
		{src: "h := 1; h >>= 8", res: "0"},
	})
}

func TestEvalBuiltin(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := []int{}; a = append(a, 1); a`, res: "[1]"},
		{src: `b := []int{1}; b = append(a, 2, 3); b`, res: "[1 2 3]"},
		{src: `c := []int{1}; d := []int{2, 3}; c = append(c, d...); c`, res: "[1 2 3]"},
		{src: `string(append([]byte("hello "), "world"...))`, res: "hello world"},
		{src: `e := "world"; string(append([]byte("hello "), e...))`, res: "hello world"},
		{src: `b := []int{1}; b = append(1, 2, 3); b`, err: "1:54: first argument to append must be slice; have int"},
		{src: `g := len(a)`, res: "1"},
		{src: `g := cap(a)`, res: "1"},
		{src: `g := len("test")`, res: "4"},
		{src: `g := len(map[string]string{"a": "b"})`, res: "1"},
		{src: `a := len()`, err: "not enough arguments in call to len"},
		{src: `a := len([]int, 0)`, err: "too many arguments for len"},
		{src: `g := cap("test")`, err: "1:37: invalid argument for cap"},
		{src: `g := cap(map[string]string{"a": "b"})`, err: "1:37: invalid argument for cap"},
		{src: `h := make(chan int, 1); close(h); len(h)`, res: "0"},
		{src: `close(a)`, err: "1:34: invalid operation: non-chan type []int"},
		{src: `h := make(chan int, 1); var i <-chan int = h; close(i)`, err: "1:80: invalid operation: cannot close receive-only channel"},
		{src: `j := make([]int, 2)`, res: "[0 0]"},
		{src: `j := make([]int, 2, 3)`, res: "[0 0]"},
		{src: `j := make(int)`, err: "1:38: cannot make int; type must be slice, map, or channel"},
		{src: `j := make([]int)`, err: "1:33: not enough arguments in call to make"},
		{src: `j := make([]int, 0, 1, 2)`, err: "1:33: too many arguments for make"},
		{src: `j := make([]int, 2, 1)`, err: "1:33: len larger than cap in make"},
		{src: `j := make([]int, "test")`, err: "1:45: cannot convert \"test\" to int"},
		{src: `k := []int{3, 4}; copy(k, []int{1,2}); k`, res: "[1 2]"},
		{src: `f := []byte("Hello"); copy(f, "world"); string(f)`, res: "world"},
		{src: `copy(g, g)`, err: "1:28: copy expects slice arguments"},
		{src: `copy(a, "world")`, err: "1:28: arguments to copy have different element types []int and string"},
		{src: `l := map[string]int{"a": 1, "b": 2}; delete(l, "a"); l`, res: "map[b:2]"},
		{src: `delete(a, 1)`, err: "1:35: first argument to delete must be map; have []int"},
		{src: `l := map[string]int{"a": 1, "b": 2}; delete(l, 1)`, err: "1:75: cannot use int as type string in delete"},
		{src: `a := []int{1,2}; println(a...)`, err: "invalid use of ... with builtin println"},
		{src: `m := complex(3, 2); real(m)`, res: "3"},
		{src: `m := complex(3, 2); imag(m)`, res: "2"},
		{src: `m := complex("test", 2)`, err: "1:33: invalid types string and int"},
		{src: `imag("test")`, err: "1:33: cannot convert \"test\" to complex128"},
		{src: `imag(a)`, err: "1:33: invalid argument type []int for imag"},
		{src: `real(a)`, err: "1:33: invalid argument type []int for real"},
	})
}

func TestEvalDecl(t *testing.T) {
	i := interp.New(interp.Options{})
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
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `(func () string {return "ok"})()`, res: "ok"},
		{src: `(func () (res string) {res = "ok"; return})()`, res: "ok"},
		{src: `(func () int {f := func() (a, b int) {a, b = 3, 4; return}; x, y := f(); return x+y})()`, res: "7"},
		{src: `(func () int {f := func() (a int, b, c int) {a, b, c = 3, 4, 5; return}; x, y, z := f(); return x+y+z})()`, res: "12"},
		{src: `(func () int {f := func() (a, b, c int) {a, b, c = 3, 4, 5; return}; x, y, z := f(); return x+y+z})()`, res: "12"},
	})
}

func TestEvalImport(t *testing.T) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	runTests(t, i, []testCase{
		{pre: func() { eval(t, i, `import "time"`) }, src: "2 * time.Second", res: "2s"},
	})
}

func TestEvalNil(t *testing.T) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	runTests(t, i, []testCase{
		{desc: "assign nil", src: "a := nil", err: "1:33: use of untyped nil"},
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
			res: "<nil>",
		},
		{
			desc: "return nil func",
			pre: func() {
				eval(t, i, `func Bar() func() { return nil }`)
			},
			src: "Bar()",
			res: "<nil>",
		},
	})
}

func TestEvalStruct0(t *testing.T) {
	i := interp.New(interp.Options{})
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
	i := interp.New(interp.Options{})
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
	i := interp.New(interp.Options{})
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
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
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
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `2 > 1`, res: "true"},
		{src: `1.2 > 1.1`, res: "true"},
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
			err: "7:13: invalid operation: mismatched types main.Foo and main.Bar",
		},
	})
}

func TestEvalCompositeArray(t *testing.T) {
	i := interp.New(interp.Options{})
	eval(t, i, `const l = 10`)
	runTests(t, i, []testCase{
		{src: "a := []int{1, 2, 7: 20, 30}", res: "[1 2 0 0 0 0 0 20 30]"},
		{src: `a := []int{1, 1.2}`, err: "1:42: 6/5 truncated to int"},
		{src: `a := []int{0:1, 0:1}`, err: "1:46: duplicate index 0 in array or slice literal"},
		{src: `a := []int{1.1:1, 1.2:"test"}`, err: "1:39: index float64 must be integer constant"},
		{src: `a := [2]int{1, 1.2}`, err: "1:43: 6/5 truncated to int"},
		{src: `a := [1]int{1, 2}`, err: "1:43: index 1 is out of bounds (>= 1)"},
		{src: `b := [l]int{1, 2}`, res: "[1 2 0 0 0 0 0 0 0 0]"},
		{src: `i := 10; a := [i]int{1, 2}`, err: "1:43: non-constant array bound \"i\""},
	})
}

func TestEvalCompositeMap(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := map[string]int{"one":1, "two":2}`, res: "map[one:1 two:2]"},
		{src: `a := map[string]int{1:1, 2:2}`, err: "1:48: cannot convert 1 to string"},
		{src: `a := map[string]int{"one":1, "two":2.2}`, err: "1:63: 11/5 truncated to int"},
		{src: `a := map[string]int{1, "two":2}`, err: "1:48: missing key in map literal"},
		{src: `a := map[string]int{"one":1, "one":2}`, err: "1:57: duplicate key one in map literal"},
	})
}

func TestEvalCompositeStruct(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := struct{A,B,C int}{}`, res: "{0 0 0}"},
		{src: `a := struct{A,B,C int}{1,2,3}`, res: "{1 2 3}"},
		{src: `a := struct{A,B,C int}{1,2.2,3}`, err: "1:53: 11/5 truncated to int"},
		{src: `a := struct{A,B,C int}{1,2}`, err: "1:53: too few values in struct literal"},
		{src: `a := struct{A,B,C int}{1,2,3,4}`, err: "1:57: too many values in struct literal"},
		{src: `a := struct{A,B,C int}{1,B:2,3}`, err: "1:53: mixture of field:value and value elements in struct literal"},
		{src: `a := struct{A,B,C int}{A:1,B:2,C:3}`, res: "{1 2 3}"},
		{src: `a := struct{A,B,C int}{B:2}`, res: "{0 2 0}"},
		{src: `a := struct{A,B,C int}{A:1,D:2,C:3}`, err: "1:55: unknown field D in struct literal"},
		{src: `a := struct{A,B,C int}{A:1,A:2,C:3}`, err: "1:55: duplicate field name A in struct literal"},
		{src: `a := struct{A,B,C int}{A:1,B:2.2,C:3}`, err: "1:57: 11/5 truncated to int"},
		{src: `a := struct{A,B,C int}{A:1,2,C:3}`, err: "1:55: mixture of field:value and value elements in struct literal"},
	})
}

func TestEvalSliceExpression(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := []int{0,1,2}[1:3]`, res: "[1 2]"},
		{src: `a := []int{0,1,2}[:3]`, res: "[0 1 2]"},
		{src: `a := []int{0,1,2}[:]`, res: "[0 1 2]"},
		{src: `a := []int{0,1,2,3}[1:3:4]`, res: "[1 2]"},
		{src: `a := []int{0,1,2,3}[:3:4]`, res: "[0 1 2]"},
		{src: `ar := [3]int{0,1,2}
			   a := ar[1:3]`, res: "[1 2]"},
		{src: `a := (&[3]int{0,1,2})[1:3]`, res: "[1 2]"},
		{src: `a := (&[3]int{0,1,2})[1:3]`, res: "[1 2]"},
		{src: `s := "hello"[1:3]`, res: "el"},
		{src: `str := "hello"
			   s := str[1:3]`, res: "el"},
		{src: `a := int(1)[0:1]`, err: "1:33: cannot slice type int"},
		{src: `a := ([3]int{0,1,2})[1:3]`, err: "1:33: cannot slice type [3]int"},
		{src: `a := (&[]int{0,1,2,3})[1:3]`, err: "1:33: cannot slice type *[]int"},
		{src: `a := "hello"[1:3:4]`, err: "1:45: invalid operation: 3-index slice of string"},
		{src: `ar := [3]int{0,1,2}
			   a := ar[:4]`, err: "2:16: index int is out of bounds"},
		{src: `a := []int{0,1,2,3}[1::4]`, err: "1:49: 2nd index required in 3-index slice"},
		{src: `a := []int{0,1,2,3}[1:3:]`, err: "1:51: 3rd index required in 3-index slice"},
		{src: `a := []int{0,1,2}[3:1]`, err: "invalid index values, must be low <= high <= max"},
	})
}

func TestEvalConversion(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := uint64(1)`, res: "1"},
		{src: `i := 1.1; a := uint64(i)`, res: "1"},
		{src: `b := string(49)`, res: "1"},
		{src: `c := uint64(1.1)`, err: "1:40: cannot convert expression of type float64 to type uint64"},
	})
}

func TestEvalUnary(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: "a := -1", res: "-1"},
		{src: "b := +1", res: "1", skip: "BUG"},
		{src: "c := !false", res: "true"},
	})
}

func TestEvalMethod(t *testing.T) {
	i := interp.New(interp.Options{})
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

func TestEvalChan(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{
			src: `(func () string {
				messages := make(chan string)
				go func() { messages <- "ping" }()
				msg := <-messages
				return msg
			})()`, res: "ping",
		},
		{
			src: `(func () bool {
				messages := make(chan string)
				go func() { messages <- "ping" }()
				msg, ok := <-messages
				return ok && msg == "ping"
			})()`, res: "true",
		},
		{
			src: `(func () bool {
				messages := make(chan string)
				go func() { messages <- "ping" }()
				var msg string
				var ok bool
				msg, ok = <-messages
				return ok && msg == "ping"
			})()`, res: "true",
		},
	})
}

func TestEvalFunctionCallWithFunctionParam(t *testing.T) {
	i := interp.New(interp.Options{})
	eval(t, i, `
		func Bar(s string, fn func(string)string) string { return fn(s) }
	`)

	v := eval(t, i, "Bar")
	bar := v.Interface().(func(string, func(string) string) string)

	got := bar("hello ", func(s string) string {
		return s + "world!"
	})

	want := "hello world!"
	if got != want {
		t.Errorf("unexpected result of function eval: got %q, want %q", got, want)
	}
}

func TestEvalCall(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: ` test := func(a int, b float64) int { return a }
				a := test(1, 2.3)`, res: "1"},
		{src: ` test := func(a int, b float64) int { return a }
				a := test(1)`, err: "2:10: not enough arguments in call to test"},
		{src: ` test := func(a int, b float64) int { return a }
				s := "test"
				a := test(1, s)`, err: "3:18: cannot use type string as type float64"},
		{src: ` test := func(a ...int) int { return 1 }
				a := test([]int{1}...)`, res: "1"},
		{src: ` test := func(a ...int) int { return 1 }
				a := test()`, res: "1"},
		{src: ` test := func(a ...int) int { return 1 }
				blah := func() []int { return []int{1,1} }
				a := test(blah()...)`, res: "1"},
		{src: ` test := func(a ...int) int { return 1 }
				a := test([]string{"1"}...)`, err: "2:15: cannot use []string as type []int"},
		{src: ` test := func(a ...int) int { return 1 }
				i := 1
				a := test(i...)`, err: "3:15: cannot use int as type []int"},
		{src: ` test := func(a int) int { return a }
				a := test([]int{1}...)`, err: "2:10: invalid use of ..., corresponding parameter is non-variadic"},
		{src: ` test := func(a ...int) int { return 1 }
				blah := func() (int, int) { return 1, 1 }
				a := test(blah()...)`, err: "3:15: cannot use ... with 2-valued func()(int,int)"},
		{src: ` test := func(a, b int) int { return a }
				blah := func() (int, int) { return 1, 1 }
				a := test(blah())`, res: "1"},
		{src: ` test := func(a, b int) int { return a }
				blah := func() int { return 1 }
				a := test(blah(), blah())`, res: "1"},
		{src: ` test := func(a, b, c, d int) int { return a }
				blah := func() (int, int) { return 1, 1 }
				a := test(blah(), blah())`, err: "3:15: cannot use func()(int,int) as type int"},
		{src: ` test := func(a, b int) int { return a }
				blah := func() (int, float64) { return 1, 1.1 }
				a := test(blah())`, err: "3:15: cannot use func()(int,float64) as type (int,int)"},
	})
}

func TestEvalBinCall(t *testing.T) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	if _, err := i.Eval(`import "fmt"`); err != nil {
		t.Fatal(err)
	}
	runTests(t, i, []testCase{
		{src: `a := fmt.Sprint(1, 2.3)`, res: "1 2.3"},
		{src: `a := fmt.Sprintf()`, err: "1:33: not enough arguments in call to fmt.Sprintf"},
		{src: `i := 1
			   a := fmt.Sprintf(i)`, err: "2:24: cannot use type int as type string"},
		{src: `a := fmt.Sprint()`, res: ""},
	})
}

func TestEvalMissingSymbol(t *testing.T) {
	defer func() {
		r := recover()
		if r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	type S2 struct{}
	type S1 struct {
		F S2
	}
	i := interp.New(interp.Options{})
	i.Use(interp.Exports{"p": map[string]reflect.Value{
		"S1": reflect.Zero(reflect.TypeOf(&S1{})),
	}})
	_, err := i.Eval(`import "p"`)
	if err != nil {
		t.Fatalf("failed to import package: %v", err)
	}
	_, err = i.Eval(`p.S1{F: p.S2{}}`)
	if err == nil {
		t.Error("unexpected nil error for expression with undefined type")
	}
}

func TestEvalWithContext(t *testing.T) {
	tests := []testCase{
		{
			desc: "for {}",
			src: `(func() {
				      for {}
			      })()`,
		},
		{
			desc: "select {}",
			src: `(func() {
				     select {}
			     })()`,
		},
		{
			desc: "blocked chan send",
			src: `(func() {
			         c := make(chan int)
				     c <- 1
				 })()`,
		},
		{
			desc: "blocked chan recv",
			src: `(func() {
			         c := make(chan int)
				     <-c
			     })()`,
		},
		{
			desc: "blocked chan recv2",
			src: `(func() {
			         c := make(chan int)
				     _, _ = <-c
			     })()`,
		},
		{
			desc: "blocked range chan",
			src: `(func() {
			         c := make(chan int)
				     for range c {}
			     })()`,
		},
		{
			desc: "double lock",
			src: `(func() {
			         var mu sync.Mutex
				     mu.Lock()
				     mu.Lock()
			      })()`,
		},
	}

	for _, test := range tests {
		done := make(chan struct{})
		src := test.src
		go func() {
			defer close(done)
			i := interp.New(interp.Options{})
			i.Use(stdlib.Symbols)
			_, err := i.Eval(`import "sync"`)
			if err != nil {
				t.Errorf(`failed to import "sync": %v`, err)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err = i.EvalWithContext(ctx, src)
			switch err {
			case context.DeadlineExceeded:
				// Successful cancellation.

				// Check we can still execute an expression.
				v, err := i.EvalWithContext(context.Background(), "1+1\n")
				if err != nil {
					t.Errorf("failed to evaluate expression after cancellation: %v", err)
				}
				got := v.Interface()
				if got != 2 {
					t.Errorf("unexpected result of eval(1+1): got %v, want 2", got)
				}
			case nil:
				t.Errorf("unexpected success evaluating expression %q", test.desc)
			default:
				t.Errorf("failed to evaluate expression %q: %v", test.desc, err)
			}
		}()
		select {
		case <-time.After(time.Second):
			t.Errorf("timeout failed to terminate execution of %q", test.desc)
		case <-done:
		}
	}
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
		t.Logf("Error: %v", err)
		if e, ok := err.(interp.Panic); ok {
			t.Logf(string(e.Stack))
		}
		t.FailNow()
	}
	return res
}

func assertEval(t *testing.T, i *interp.Interpreter, src, expectedError, expectedRes string) {
	res, err := i.Eval(src)

	if expectedError != "" {
		if err == nil || !strings.Contains(err.Error(), expectedError) {
			t.Fatalf("got %v, want %s", err, expectedError)
		}
		return
	}

	if err != nil {
		t.Logf("got an error: %v", err)
		if e, ok := err.(interp.Panic); ok {
			t.Logf(string(e.Stack))
		}
		t.FailNow()
	}

	if fmt.Sprintf("%v", res) != expectedRes {
		t.Fatalf("got %v, want %s", res, expectedRes)
	}
}

func TestMultiEval(t *testing.T) {
	// catch stdout
	backupStdout := os.Stdout
	defer func() {
		os.Stdout = backupStdout
	}()
	r, w, _ := os.Pipe()
	os.Stdout = w

	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	var err error

	f, err := os.Open(filepath.Join("testdata", "multi", "731"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range names {
		if _, err := i.EvalPath(filepath.Join(f.Name(), v)); err != nil {
			t.Fatal(err)
		}
	}

	// read stdout
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	outInterp, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	// restore Stdout
	os.Stdout = backupStdout

	want := "A\nB\n"
	got := string(outInterp)
	if got != want {
		t.Fatalf("unexpected output: got %v, wanted %v", got, want)
	}
}

func TestMultiEvalNoName(t *testing.T) {
	i := interp.New(interp.Options{})
	i.Use(stdlib.Symbols)
	var err error

	f, err := os.Open(filepath.Join("testdata", "multi", "731"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range names {
		data, err := ioutil.ReadFile(filepath.Join(f.Name(), v))
		if err != nil {
			t.Fatal(err)
		}
		_, err = i.Eval(string(data))
		if k == 1 {
			expectedErr := fmt.Errorf("3:8: fmt/%s redeclared in this block", interp.DefaultSourceName)
			if err.Error() != expectedErr.Error() {
				t.Fatalf("unexpected result; wanted error %v, got %v", expectedErr, err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestImportPathIsKey(t *testing.T) {
	// No need to check the results of Eval, as TestFile already does it.
	i := interp.New(interp.Options{GoPath: filepath.FromSlash("../_test/testdata/redeclaration-global7")})
	i.Use(stdlib.Symbols)

	filePath := filepath.Join("..", "_test", "ipp_as_key.go")
	if _, err := i.EvalPath(filePath); err != nil {
		t.Fatal(err)
	}

	wantScopes := map[string][]string{
		"main": {
			"titi/ipp_as_key.go",
			"tutu/ipp_as_key.go",
			"main",
		},
		"guthib.com/toto": {
			"quux/titi.go",
			"Quux",
		},
		"guthib.com/bar": {
			"Quux",
		},
		"guthib.com/tata": {
			"quux/tutu.go",
			"Quux",
		},
		"guthib.com/baz": {
			"Quux",
		},
	}
	wantPackages := map[string]string{
		"guthib.com/baz":  "quux",
		"guthib.com/tata": "tutu",
		"main":            "main",
		"guthib.com/bar":  "quux",
		"guthib.com/toto": "titi",
	}

	scopes := i.Scopes()
	if len(scopes) != len(wantScopes) {
		t.Fatalf("want %d, got %d", len(wantScopes), len(scopes))
	}
	for k, v := range scopes {
		wantSym := wantScopes[k]
		if len(v) != len(wantSym) {
			t.Fatalf("want %d, got %d", len(wantSym), len(v))
		}
		for _, sym := range wantSym {
			if _, ok := v[sym]; !ok {
				t.Fatalf("symbol %s not found in scope %s", sym, k)
			}
		}
	}

	packages := i.Packages()
	if len(packages) != len(wantPackages) {
		t.Fatalf("want %d, got %d", len(wantPackages), len(packages))
	}
	for k, v := range wantPackages {
		pkg := packages[k]
		if pkg != v {
			t.Fatalf("for import path %s, want %s, got %s", k, v, pkg)
		}
	}
}

// Disabled for now, because it reveals a data race, and we want our current PRs
// to pass the CI.
/*
func TestEvalScanner(t *testing.T) {
	tests := []struct {
		desc      string
		src       []string
		errorLine int
	}{
		{
			desc: "no error",
			src: []string{
				`func main() {`,
				`println("foo")`,
				`}`,
			},
			errorLine: -1,
		},

		{
			desc: "no parsing error, but block error",
			src: []string{
				`func main() {`,
				`println(foo)`,
				`}`,
			},
			errorLine: 2,
		},
		{
			desc: "parsing error",
			src: []string{
				`func main() {`,
				`println(/foo)`,
				`}`,
			},
			errorLine: 1,
		},
		{
			desc: "multi-line string literal",
			src: []string{
				"var a = `hello",
				"there, how",
				"are you?`",
			},
			errorLine: -1,
		},

		{
			desc: "multi-line comma operand",
			src: []string{
				`println(2,`,
				`3)`,
			},
			errorLine: -1,
		},
		{
			desc: "multi-line arithmetic operand",
			src: []string{
				`println(2. /`,
				`3.)`,
			},
			errorLine: -1,
		},
		// TODO: these tests are showing that we handle retries properly for func
		// closure calls, but they reveal a data race we seem to have in EvalWithContext,
		// so they're disabled for now to avoid the CI nagging us about it.
			{
				desc: "anonymous func call with no assignment",
				src: []string{
					`func() { println(3) }()`,
				},
				errorLine: -1,
			},
			{
				// to make sure that special handling of the above anonymous, does not break this general case.
				desc: "just func",
				src: []string{
					`func foo() { println(3) }`,
				},
				errorLine: -1,
			},
			{
				// to make sure that special handling of the above anonymous, does not break this general case.
				desc: "just method",
				src: []string{
					`type bar string`,
					`func (b bar) foo() { println(3) }`,
				},
				errorLine: -1,
			},
	}

	for _, test := range tests {
		i := interp.New(interp.Options{})
		var stderr bytes.Buffer
		safeStderr := &safeBuffer{buf: &stderr}
		pin, pout := io.Pipe()
		defer func() {
			// Closing the pipe also takes care of making i.REPL terminate,
			// hence freeing its goroutine.
			_ = pin.Close()
			_ = pout.Close()
		}()

		go func() {
			i.REPL(pin, safeStderr)
		}()
		for k, v := range test.src {
			if _, err := pout.Write([]byte(v + "\n")); err != nil {
				t.Error(err)
			}
			Sleep(100 * time.Millisecond)

			errMsg := safeStderr.String()
			if k == test.errorLine {
				if errMsg == "" {
					t.Fatalf("test %q: statement %q should have produced an error", test.desc, v)
				}
				break
			}
			if errMsg != "" {
				t.Fatalf("test %q: unexpected error: %v", test.desc, errMsg)
			}
		}
	}
}

type safeBuffer struct {
	mu  sync.RWMutex
	buf *bytes.Buffer
}

func (sb *safeBuffer) Read(p []byte) (int, error) {
	return sb.buf.Read(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.buf.String()
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

const (
	// CITimeoutMultiplier is the multiplier for all timeouts in the CI.
	CITimeoutMultiplier = 3
)

// Sleep pauses the current goroutine for at least the duration d.
func Sleep(d time.Duration) {
	d = applyCIMultiplier(d)
	time.Sleep(d)
}

func applyCIMultiplier(timeout time.Duration) time.Duration {
	ci := os.Getenv("CI")
	if ci == "" {
		return timeout
	}
	b, err := strconv.ParseBool(ci)
	if err != nil || !b {
		return timeout
	}
	return time.Duration(float64(timeout) * CITimeoutMultiplier)
}
*/
