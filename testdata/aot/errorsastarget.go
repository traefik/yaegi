// AOT test program: an As method of the program sets the target of
// errors.As. In AOT mode the copy of errors gives the target (a pointer to
// a cell of the program) back to the method in an any: the write must reach
// the program's variable, and a pointer of a native type that the method
// stores must keep its identity (the TestAs test of package errors).
//
//aot:lib errors
package main

import (
	"errors"
	"fmt"
	"io/fs"
)

type myErr struct{ s string }

func (e *myErr) Error() string { return e.s }

type poser struct{}

var (
	poserPathErr = &fs.PathError{Op: "poser", Path: "/p", Err: errors.New("inner")}
	mine         = &myErr{"mine"}
)

func (p *poser) Error() string { return "poser" }

func (p *poser) As(target any) bool {
	switch x := target.(type) {
	case **fs.PathError:
		*x = poserPathErr
	case **myErr:
		*x = mine
	case *string:
		*x = "set by As"
	default:
		return false
	}
	return true
}

type wrapper struct{ err error }

func (w wrapper) Error() string { return "wrap: " + w.err.Error() }
func (w wrapper) Unwrap() error { return w.err }

func main() {
	var pe *fs.PathError
	fmt.Println(errors.As(&poser{}, &pe), pe == poserPathErr, pe)

	var me *myErr
	fmt.Println(errors.As(&poser{}, &me), me == mine, me)

	// through Unwrap, and twice on the same target
	pe = nil
	fmt.Println(errors.As(wrapper{&poser{}}, &pe), pe == poserPathErr)
	fmt.Println(errors.As(wrapper{wrapper{&poser{}}}, &pe), pe == poserPathErr)

	// a target in a struct field
	var box struct{ pe *fs.PathError }
	fmt.Println(errors.As(&poser{}, &box.pe), box.pe == poserPathErr)

	// a pointer of the program to a native struct, given to the copy and
	// set into the target with reflect
	p := &fs.PathError{Op: "own"}
	var target *fs.PathError
	fmt.Println(errors.As(error(p), &target), target == p)
}
