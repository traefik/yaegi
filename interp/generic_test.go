package interp

import (
	"reflect"
	"testing"
)

func TestGenericFuncDeclare(t *testing.T) {
	i := New(Options{})
	_, err := i.Eval("func Hello[T comparable](v T) *T {\n\treturn &v\n}")
	if err != nil {
		t.Error(err)
	}
	res, err := i.Eval("Hello(3)")
	if err != nil {
		t.Error(err)
	}
	if res.Elem().Interface() != 3 {
		t.Error("expected &(3), got", res)
	}
}

func TestGenericFuncBasic(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Hello": reflect.ValueOf(GenericFunc("func Hello[T comparable](v T) *T {\n\treturn &v\n}")),
		},
	})
	if err != nil {
		t.Error(err)
	}
	res, err := i.Eval("generic.Hello(3)")
	if err != nil {
		t.Error(err)
	}
	if res.Elem().Interface() != 3 {
		t.Error("expected &(3), got", res)
	}
}

func TestGenericFuncNoDotImport(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Hello": reflect.ValueOf(GenericFunc("func Hello[T any](v T) { println(v) }")),
		},
	})
	if err != nil {
		t.Error(err)
	}
	_, err = i.Eval(`
import "guthib.com/generic"
func main() { generic.Hello(3) }
`)
	if err != nil {
		t.Error(err)
	}
}

func TestGenericFuncDotImport(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Hello": reflect.ValueOf(GenericFunc("func Hello[T any](v T) { println(v) }")),
		},
	})
	if err != nil {
		t.Error(err)
	}
	_, err = i.Eval(`
import . "guthib.com/generic"
func main() { Hello(3) }
`)
	if err != nil {
		t.Error(err)
	}
}
func TestGenericFuncComplex(t *testing.T) {
	i := New(Options{})
	done := false
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Do":    reflect.ValueOf(func() { done = true }),
			"Hello": reflect.ValueOf(GenericFunc("func Hello[T comparable, F any](v T, f func(a T) F) *T {\n\tDo(); return &v\n}")),
		},
	})
	i.ImportUsed()
	if err != nil {
		t.Error(err)
	}
	res, err := i.Eval("generic.Hello[int, bool](3, func(a int) bool { return true })")
	if err != nil {
		t.Error(err)
	}
	if res.Elem().Interface() != 3 {
		t.Error("expected &(3), got", res)
	}
	if !done {
		t.Error("!done")
	}
}

func TestGenericFuncTwice(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Do":    reflect.ValueOf(GenericFunc("func Do[T any](v T) { println(v) }")),
			"Hello": reflect.ValueOf(GenericFunc("func Hello[T any](v T) { Do(v) }")),
		},
	})
	i.ImportUsed()
	if err != nil {
		t.Error(err)
	}
	_, err = i.Eval(`
func main() { generic.Hello[int](3) }
`)
	if err != nil {
		t.Error(err)
	}
}

func TestGenericFuncInfer(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"New":   reflect.ValueOf(GenericFunc("func New[T any]() *T { return new(T) }")),
			"AddAt": reflect.ValueOf(GenericFunc("func AddAt[T any](init func(n *T)) { v := New[T](); init(any(v).(*T)); println(*v) }")),
		},
	})
	i.ImportUsed()
	if err != nil {
		t.Error(err)
	}
	_, err = i.Eval(`
func main() {
	generic.AddAt(func(w *int) { *w = 3 })
}
`)
	if err != nil {
		t.Error(err)
	}
}

type Plan struct{}

// this one failed with valueT included in inferTypes
func TestGenericFuncInferSecondArg(t *testing.T) {
	i := New(Options{})
	err := i.Use(Exports{
		"guthib.com/generic/generic": map[string]reflect.Value{
			"Plan": reflect.ValueOf((*Plan)(nil)),
		},
	})
	i.ImportUsed()
	if err != nil {
		t.Error(err)
	}
	_, err = i.Eval(`
func Add[T any](p generic.Plan, v T) { }
func main() {
	Add(generic.Plan{}, []int{})
}
`)
	if err != nil {
		t.Error(err)
	}
}

// this one worked fine with valueT
func TestGenericFuncInferSecondArgLocal(t *testing.T) {
	i := New(Options{})
	_, err := i.Eval(`
type Plan struct{}
func Add[T any](p Plan, v T) { }
func main() {
	Add(Plan{}, []int{})
}
`)
	if err != nil {
		t.Error(err)
	}
}

// this one failed without more robust arg type matching in generic.go:300
func TestGenericFuncIgnoreError(t *testing.T) {
	i := New(Options{})
	_, err := i.Eval(`
func Ignore[T any](v T, err error) T { return v }
func Make() (int, error) { return 3, nil }
func main() {
	a := Ignore(Make())
}
`)
	if err != nil {
		t.Error(err)
	}
}
