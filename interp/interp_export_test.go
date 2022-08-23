package interp_test

import (
	"reflect"
	"testing"

	"github.com/traefik/yaegi/interp"
)

type Helloer interface {
	Hello()
}

func Hi(h Helloer) {
	println("In Hi:")
	h.Hello()
}

// A Wrap represents the wrapper which allows to use objects created by
// the interpreter as Go interfaces (despite limitations in reflect which
// forbid dynamic method creation).
//
// All the struct fields are functions, where the fied name corresponds to
// the method name prefixed by "Do". The function signature must be the
// same as the interface one.
//
// A corresponding Wrap method Xyz which satisfies the interface must exist and
// must invoke the DoXyz function.
//
// To be usable, the interpreter should return a Wrap instance with the relevant
// function fields filled. The application can then invoke methods on it.
// The method calls will be forwarded to the interpreter.
//
// Only the Wrap type definition needs to be exported to the interpreter (not
// the interfaces and methods definitions).
type Wrap struct {
	DoHello func() // related to the Hello() method.
	// Other interface method wrappers...
}

func (w Wrap) Hello() { w.DoHello() }

func TestExportsSemantics(t *testing.T) {
	Foo := &struct{}{}

	t.Run("Correct", func(t *testing.T) {
		t.Skip()
		i := interp.New(interp.Options{})

		err := i.Use(interp.Exports{
			"foo/foo": {"Foo": reflect.ValueOf(Foo)},
		})
		if err != nil {
			t.Fatal(err)
		}
		i.ImportUsed()

		res, err := i.Eval("foo.Foo")
		if err != nil {
			t.Fatal(err)
		}
		if res.Interface() != Foo {
			t.Fatalf("expected foo.Foo to equal local Foo")
		}
	})

	t.Run("Incorrect", func(t *testing.T) {
		i := interp.New(interp.Options{})

		err := i.Use(interp.Exports{
			"foo": {"Foo": reflect.ValueOf(Foo)},
		})
		if err == nil {
			t.Fatal("expected error for incorrect Use semantics")
		}
	})
}

func TestInterface(t *testing.T) {
	i := interp.New(interp.Options{})
	// export the Wrap type to the interpreter under virtual "wrap" package
	err := i.Use(interp.Exports{
		"wrap/wrap": {
			"Wrap": reflect.ValueOf((*Wrap)(nil)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	eval(t, i, `
import "wrap"

type MyInt int

func (m MyInt) Hello() { println("hello from Myint", m) }

func NewMyInt(i int) wrap.Wrap {
	m := MyInt(i)
	return wrap.Wrap{DoHello: m.Hello}
}
`)
	NewMyInt := eval(t, i, "NewMyInt").Interface().(func(int) Wrap)
	w := NewMyInt(4)
	Hi(w)
}

type T struct{}

func (t T) Bar(s ...string) {}

func TestCallBinVariadicMethod(t *testing.T) {
	i := interp.New(interp.Options{})
	err := i.Use(interp.Exports{
		"mypkg/mypkg": {
			"T": reflect.ValueOf((*T)(nil)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	eval(t, i, `
package p

import "mypkg"

func Foo(x mypkg.T) { x.Bar("s") }
`)
	v := eval(t, i, "p.Foo")
	bar := v.Interface().(func(t T))
	bar(T{})
}
