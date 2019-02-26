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

func TestEval0(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `var I int = 2`)

	t1 := evalCheck(t, i, `I`)
	if t1.Interface().(int) != 2 {
		t.Fatalf("expected 2, got %v", t1)
	}
}

func TestEval1(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `func Hello() string { return "hello" }`)

	v := evalCheck(t, i, `Hello`)

	f, ok := v.Interface().(func() string)
	if !ok {
		t.Fatal("conversion failed")
	}

	if s := f(); s != "hello" {
		t.Fatalf("expected hello, got %v", s)
	}
}

func TestEval2(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `package foo; var I int = 2`)

	t1 := evalCheck(t, i, `foo.I`)
	if t1.Interface().(int) != 2 {
		t.Fatalf("expected 2, got %v", t1)
	}
}

func TestEval3(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `package foo; func Hello() string { return "hello" }`)

	v := evalCheck(t, i, `foo.Hello`)
	f, ok := v.Interface().(func() string)
	if !ok {
		t.Fatal("conversion failed")
	}
	if s := f(); s != "hello" {
		t.Fatalf("expected hello, got %v", s)
	}
}

func TestEvalNil0(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `func getNil() error { return nil }`)

	v := evalCheck(t, i, `getNil()`)
	if !v.IsNil() {
		t.Fatalf("expected nil, got %v", v)
	}
}

func TestEvalNil1(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `
package bar

func New() func(string) error {
	return func(v string) error {
		return nil
	}
}
`)

	v := evalCheck(t, i, `bar.New()`)
	fn, ok := v.Interface().(func(string) error)
	if !ok {
		t.Fatal("conversion failed")
	}

	if res := fn("hello"); res != nil {
		t.Fatalf("expected nil, got %v", res)
	}
}

func TestEvalNil2(t *testing.T) {
	i := interp.New(interp.Opt{})
	_, err := i.Eval(`a := nil`)
	if err.Error() != "1:27: use of untyped nil" {
		t.Fatal("should have failed")
	}
}

func TestEvalNil3(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type)
	evalCheck(t, i, `
import "fmt"

type Foo struct{}

func Hello() *Foo {
	fmt.Println("Hello")
	return nil
}
`)

	evalCheck(t, i, `Hello()`)
}

func TestEvalStruct0(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `
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

	v := evalCheck(t, i, `f()`)
	if v.Interface().(string) != "test" {
		t.Fatalf("expected test, got %v", v)
	}
}

func TestEvalStruct1(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `
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

	v := evalCheck(t, i, `f()`)
	if v.Interface().(string) != "test" {
		t.Fatalf("expected test, got %v", v)
	}
}

func TestEvalComposite0(t *testing.T) {
	i := interp.New(interp.Opt{})
	evalCheck(t, i, `
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
	v := evalCheck(t, i, `a.p[1]`)
	if v.Interface().(string) != "world" {
		t.Fatalf("expected world, got %v", v)
	}
}

func TestEvalCompositeBin0(t *testing.T) {
	i := interp.New(interp.Opt{})
	i.Use(stdlib.Value, stdlib.Type)
	evalCheck(t, i, `
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
	evalCheck(t, i, `Foo()`)
	if http.DefaultClient.Timeout != 2*time.Second {
		t.Fatalf("expected 2s, got %v", http.DefaultClient.Timeout)
	}
}

func TestEvalCompositeArray0(t *testing.T) {
	i := interp.New(interp.Opt{})
	v := evalCheck(t, i, `a := []int{1, 2, 7: 20, 30}`)
	expected := "[1 2 0 0 0 0 0 20 30]"
	if fmt.Sprintf("%v", v) != expected {
		t.Fatalf("expected: %s, got %v", expected, v)
	}
}

func TestEvalUnary0(t *testing.T) {
	i := interp.New(interp.Opt{})
	v := evalCheck(t, i, `a := -1`)
	if expected := "-1"; fmt.Sprintf("%v", v) != expected {
		t.Fatalf("Expected %v, got %v", expected, v)
	}
}

func TestEvalComparison(t *testing.T) {
	i := interp.New(interp.Opt{})
	v := evalCheck(t, i, `"hhh" > "ggg"`)
	if v.Bool() != true {
		t.Fatalf("expected true, got %v", v)
	}
}

func evalCheck(t *testing.T, i *interp.Interpreter, src string) reflect.Value {
	t.Helper()

	res, err := i.Eval(src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
