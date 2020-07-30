package interp_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

func TestEvalAssign(t *testing.T) {
	i := interp.New(interp.Options{})
	runTests(t, i, []testCase{
		{src: `a := "Hello"; a += " world"`, res: "Hello world"},
		{src: `b := "Hello"; b += 1`, err: "1:42: invalid operation: mismatched types string and int"},
		{src: `c := "Hello"; c -= " world"`, err: "1:42: invalid operation: operator -= not defined on string"},
		{src: "e := 64.4; e %= 64", err: "1:39: invalid operation: operator %= not defined on float64"},
		{src: "f := int64(3.2)", err: "1:33: truncated to integer"},
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
		{src: `f := []byte("Hello"); copy(f, "world"); string(f)`, res: "world"},
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
	runTests(t, i, []testCase{
		{src: "a := []int{1, 2, 7: 20, 30}", res: "[1 2 0 0 0 0 0 20 30]"},
		{src: `a := []int{1, 1.2}`, err: "1:42: 6/5 truncated to int"},
		{src: `a := []int{0:1, 0:1}`, err: "1:46: duplicate index 0 in array or slice literal"},
		{src: `a := []int{1.1:1, 1.2:"test"}`, err: "1:39: index float64 must be integer constant"},
		{src: `a := [2]int{1, 1.2}`, err: "1:43: 6/5 truncated to int"},
		{src: `a := [1]int{1, 2}`, err: "1:43: index 1 is out of bounds (>= 1)"},
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
