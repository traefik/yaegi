// errors: Is, As, Join, Unwrap with interpreted error types (value and
// pointer receivers, custom Is and As methods, Unwrap() []error).
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

type NotFound struct{ Key string }

func (e NotFound) Error() string { return "not found: " + e.Key }

type Temporary struct {
	Op  string
	Err error
}

func (e *Temporary) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *Temporary) Unwrap() error { return e.Err }

// Code compares by code with Is.
type Code int

func (c Code) Error() string { return fmt.Sprintf("code %d", int(c)) }
func (c Code) Is(target error) bool {
	t, ok := target.(Code)
	return ok && t/100 == c/100
}

// Multi has several causes.
type Multi struct{ Errs []error }

func (m Multi) Error() string   { return fmt.Sprintf("%d errors", len(m.Errs)) }
func (m Multi) Unwrap() []error { return m.Errs }

// Asser converts itself with As.
type Asser struct{}

func (Asser) Error() string { return "asser" }
func (Asser) As(target any) bool {
	if p, ok := target.(*NotFound); ok {
		*p = NotFound{"from As"}
		return true
	}
	return false
}

var ErrSentinel = errors.New("sentinel")

func main() {
	e1 := fmt.Errorf("layer2: %w", &Temporary{"read", NotFound{"k1"}})
	var nf NotFound
	fmt.Println(errors.As(e1, &nf), nf.Key)
	var tmp *Temporary
	fmt.Println(errors.As(e1, &tmp), tmp.Op)
	fmt.Println(errors.Is(e1, NotFound{"k1"}), errors.Is(e1, NotFound{"k2"}))

	e2 := fmt.Errorf("x: %w", Code(404))
	fmt.Println(errors.Is(e2, Code(400)), errors.Is(e2, Code(500)))

	j := errors.Join(ErrSentinel, &Temporary{"w", io.EOF}, nil, Code(201))
	fmt.Println(j)
	fmt.Println(errors.Is(j, io.EOF), errors.Is(j, ErrSentinel), errors.Is(j, Code(299)))
	fmt.Println(errors.As(j, &tmp), tmp.Op)

	m := Multi{[]error{NotFound{"m"}, fs.ErrNotExist}}
	fmt.Println(errors.Is(m, fs.ErrNotExist), errors.As(m, &nf), nf.Key)
	wrapped := fmt.Errorf("two: %w and %w", Code(500), NotFound{"w2"})
	fmt.Println(wrapped, errors.Is(wrapped, Code(503)), errors.As(wrapped, &nf), nf.Key)

	fmt.Println(errors.As(fmt.Errorf("a: %w", Asser{}), &nf), nf.Key)

	_, err := os.Open("/does/not/exist")
	var pe *fs.PathError
	fmt.Println(errors.Is(err, fs.ErrNotExist), errors.As(err, &pe), pe.Op)
	fmt.Println(errors.Is(&Temporary{"t", err}, os.ErrNotExist))

	var target interface{ Unwrap() error }
	fmt.Println(errors.As(e1, &target), target != nil)
	fmt.Printf("%T %T\n", errors.Unwrap(e1), errors.Unwrap(errors.Unwrap(e1)))
}
