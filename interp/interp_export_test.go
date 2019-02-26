package interp_test

import (
	"reflect"
	"testing"

	"github.com/containous/dyngo/interp"
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
// the interfaces and methods  definitions)
//
type Wrap struct {
	DoHello func() // implements Hello()
	// Other interface method wrappers...
}

func (w Wrap) Hello() { w.DoHello() }

func TestInterface(t *testing.T) {
	i := interp.New(interp.Opt{})
	// export the Wrap type to the interpeter under virtual "wrap" package
	i.Use(nil, interp.LibTypeMap{
		"wrap": {
			"Wrap": reflect.TypeOf((*Wrap)(nil)).Elem(),
		},
	})

	evalCheck(t, i, `
import "wrap"

type MyInt int

func (m MyInt) Hello() { println("hello from Myint", m) }

func NewMyInt(i int) wrap.Wrap {
	m := MyInt(i)
	return wrap.Wrap{DoHello: m.Hello}
}
`)
	NewMyInt := evalCheck(t, i, "NewMyInt").Interface().(func(int) Wrap)
	w := NewMyInt(4)
	Hi(w)
}
