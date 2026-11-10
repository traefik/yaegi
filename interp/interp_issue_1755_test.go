package interp

import (
	"reflect"
	"testing"
)

// A blank assignment in the body of a method with a blank parameter must not
// overwrite the type of the parameter frame slot. Otherwise, calling the method
// from compiled code panics with "reflect.Set: value of type ... is not
// assignable to type ...".
func TestBlankParamWithBlankAssign(t *testing.T) {
	i := New(Options{})

	if _, err := i.Eval(`
package p

type T struct{}

func New() *T { return &T{} }

func (*T) Render(_ string, n int) {
	_ = bad()
}

func bad() error { return nil }
`); err != nil {
		t.Fatal(err)
	}

	f, err := i.Eval(`p.New().Render`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Type().String(), "func(string, int)"; got != want {
		t.Fatalf("got type %s, want %s", got, want)
	}

	// Must not panic.
	f.Call([]reflect.Value{reflect.ValueOf("s"), reflect.ValueOf(1)})
}
