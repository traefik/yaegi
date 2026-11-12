// Native boundary of errors: program types given to errors (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// valErr is an error with a value receiver.
type valErr struct{ msg string }

func (e valErr) Error() string { return "valErr:" + e.msg }

// ptrErr is an error with a pointer receiver.
type ptrErr struct{ code int }

func (e *ptrErr) Error() string { return fmt.Sprintf("ptrErr:%d", e.code) }
func (e *ptrErr) Code() int     { return e.code }

// genErr is a generic error.
type genErr[T any] struct{ v T }

func (e genErr[T]) Error() string { return fmt.Sprintf("genErr:%v", e.v) }

// wrapErr wraps one error with Unwrap() error.
type wrapErr struct{ inner error }

func (e wrapErr) Error() string { return "wrap(" + e.inner.Error() + ")" }
func (e wrapErr) Unwrap() error { return e.inner }

// pwrapErr wraps one error with a pointer receiver.
type pwrapErr struct{ inner error }

func (e *pwrapErr) Error() string { return "pwrap(" + e.inner.Error() + ")" }
func (e *pwrapErr) Unwrap() error { return e.inner }

// multiErr wraps many errors with Unwrap() []error; it is not comparable.
type multiErr []error

func (m multiErr) Error() string   { return fmt.Sprintf("multi%d", len(m)) }
func (m multiErr) Unwrap() []error { return m }

// isErr matches every error whose text starts with its prefix.
type isErr struct{ prefix string }

func (e isErr) Error() string { return "isErr:" + e.prefix }
func (e isErr) Is(target error) bool {
	s := target.Error()
	return len(s) >= len(e.prefix) && s[:len(e.prefix)] == e.prefix
}

// asErr fills a *string target in its As method.
type asErr struct{ s string }

func (e *asErr) Error() string { return "asErr:" + e.s }
func (e *asErr) As(target any) bool {
	if p, ok := target.(*valErr); ok {
		*p = valErr{"from As " + e.s}
		return true
	}
	return false
}

// boxErr is comparable, but its field may hold an uncomparable value.
type boxErr struct{ v any }

func (e boxErr) Error() string { return fmt.Sprintf("boxErr:%v", e.v) }

// coder is an interface that some errors implement.
type coder interface{ Code() int }

// errIface is an interface type like error.
type errIface interface{ Error() string }

// notErr is not an error.
type notErr struct{}

func main() {
	// row: errors Unwrap assert x.(interface{Unwrap() error})
	probe("Unwrap", func() {
		for i, e := range []error{wrapErr{valErr{"a"}}, &pwrapErr{&ptrErr{1}}, multiErr{valErr{"m"}}, valErr{"x"}, genErr[int]{3}, fmt.Errorf("w: %w", wrapErr{valErr{"b"}})} {
			out("%d: %v -> %v", i, e, errors.Unwrap(e))
		}
	})
	// row: errors Is assert x.(interface{Is(error) bool})
	// row: errors Is assert x.(interface{Unwrap() error})
	// row: errors Is assert x.(interface{Unwrap() []error})
	// row: errors Is reflect comparability
	// row: errors Is reflect walk
	probe("Is", func() {
		pe := &ptrErr{7}
		chains := []error{
			valErr{"a"},
			wrapErr{valErr{"a"}},
			&pwrapErr{wrapErr{pe}},
			multiErr{valErr{"z"}, wrapErr{pe}},
			isErr{"valErr"},
			wrapErr{isErr{"genErr"}},
			genErr[string]{"g"},
			fmt.Errorf("x: %w, %w", valErr{"q"}, genErr[string]{"g"}),
		}
		targets := []error{valErr{"a"}, pe, &ptrErr{7}, genErr[string]{"g"}, multiErr{valErr{"z"}}, genErr[int]{0}}
		for i, e := range chains {
			for j, t := range targets {
				out("%d,%d: %v", i, j, errors.Is(e, t))
			}
		}
		var iface error = multiErr{}
		out("uncomparable target: %v", errors.Is(iface, iface))
	})
	// row: errors Is panic == of two interfaces: comparing uncomparable type
	probe("Is/uncomparable", func() {
		out("diff: %v", errors.Is(boxErr{1}, boxErr{2}))
		out("same: %v", errors.Is(boxErr{1}, boxErr{1}))
		out("slice: %v", errors.Is(boxErr{[]int{1}}, boxErr{[]int{1}}))
	})
	// row: errors As assert x.(interface{As(any) bool})
	// row: errors As assert x.(interface{Unwrap() error})
	// row: errors As assert x.(interface{Unwrap() []error})
	// row: errors As reflect kind
	// row: errors As reflect method set
	// row: errors As reflect walk
	probe("As", func() {
		chains := []error{
			valErr{"a"},
			wrapErr{&ptrErr{2}},
			&pwrapErr{multiErr{genErr[int]{5}, valErr{"m"}}},
			&asErr{"s"},
			wrapErr{&asErr{"t"}},
			fmt.Errorf("f: %w", &ptrErr{9}),
		}
		for i, e := range chains {
			var v valErr
			var p *ptrErr
			var g genErr[int]
			var c coder
			var ae errIface
			out("%d: valErr %v %v", i, errors.As(e, &v), v)
			out("%d: *ptrErr %v %v", i, errors.As(e, &p), p)
			out("%d: genErr %v %v", i, errors.As(e, &g), g)
			ok := errors.As(e, &c)
			code := -1
			if ok {
				code = c.Code()
			}
			out("%d: coder %v %d", i, ok, code)
			out("%d: error %v %v", i, errors.As(e, &ae), ae)
		}
	})
	// row: errors As reflect kind
	probe("As/panic", func() {
		try := func(name string, target any) {
			defer func() {
				if r := recover(); r != nil {
					out("%s: panic %T: %v", name, r, r)
				}
			}()
			out("%s: %v", name, errors.As(valErr{"x"}, target))
		}
		try("nil", nil)
		var v valErr
		try("value", v)
		var np *valErr
		try("nil pointer", np)
		var n notErr
		try("not error", &n)
		var pv ptrErr
		try("ptrErr value", &pv)
		var i int
		try("int", &i)
	})
}
