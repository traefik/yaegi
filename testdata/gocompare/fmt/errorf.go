// fmt.Errorf with %w (one, several, nil, a non-error operand), the
// Unwrap chain seen by errors.Is, errors.As and errors.Unwrap, errors.Join,
// custom error types with Is/As/Unwrap methods, and how they print.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
)

type CodeErr struct{ Code int }

func (e *CodeErr) Error() string { return "code " + strconv.Itoa(e.Code) }

type ValErr struct{ Msg string }

func (e ValErr) Error() string { return "val: " + e.Msg }

type Wrap struct {
	Op  string
	Err error
}

func (w *Wrap) Error() string { return w.Op + ": " + w.Err.Error() }
func (w *Wrap) Unwrap() error { return w.Err }

type Always struct{}

func (Always) Error() string        { return "always" }
func (Always) Is(target error) bool { return target == io.EOF }

type Multi []error

func (m Multi) Error() string   { return fmt.Sprintf("multi(%d)", len(m)) }
func (m Multi) Unwrap() []error { return m }

var ErrBase = errors.New("base")

func main() {
	e1 := fmt.Errorf("level1: %w", ErrBase)
	e2 := fmt.Errorf("level2 [%v]: %w", 7, e1)
	e3 := &Wrap{"op", e2}
	fmt.Println(e1, "|", e2, "|", e3)
	fmt.Println(errors.Is(e3, ErrBase), errors.Is(e3, io.EOF), errors.Unwrap(e3) == e2, errors.Unwrap(e2) == e1, errors.Unwrap(ErrBase) == nil)
	fmt.Printf("%T %T %T\n", e1, e3, errors.Unwrap(e1))
	two := fmt.Errorf("a %w and b %w", ErrBase, io.EOF)
	fmt.Println(two, errors.Is(two, ErrBase), errors.Is(two, io.EOF), errors.Unwrap(two) == nil)
	if u, ok := two.(interface{ Unwrap() []error }); ok {
		fmt.Println(len(u.Unwrap()), u.Unwrap())
	}
	fmt.Println(fmt.Errorf("nil: %w", nil))
	fmt.Println(errors.Unwrap(fmt.Errorf("nil: %w", nil)) == nil)
	fmt.Println(fmt.Errorf("int: %w", 5))
	fmt.Println(fmt.Errorf("no wrap %v", ErrBase), errors.Is(fmt.Errorf("no wrap %v", ErrBase), ErrBase))
	fmt.Println(fmt.Errorf("plain"), fmt.Errorf("%d%%", 5))
	fmt.Println(fmt.Errorf("%[2]w then %[1]v", "x", ErrBase), errors.Is(fmt.Errorf("%[2]w then %[1]v", "x", ErrBase), ErrBase))
	ce := &CodeErr{404}
	wrapped := fmt.Errorf("get: %w", &Wrap{"read", ce})
	var target *CodeErr
	fmt.Println(errors.As(wrapped, &target), target.Code, target == ce)
	var ve ValErr
	fmt.Println(errors.As(wrapped, &ve), errors.As(fmt.Errorf("x: %w", ValErr{"m"}), &ve), ve.Msg)
	var w *Wrap
	fmt.Println(errors.As(wrapped, &w), w.Op)
	var pe *fs.PathError
	fmt.Println(errors.As(wrapped, &pe), pe == nil)
	fmt.Println(errors.Is(fmt.Errorf("w: %w", Always{}), io.EOF), errors.Is(Always{}, ErrBase))
	j := errors.Join(ErrBase, nil, ce, io.EOF)
	fmt.Printf("%q %v %v\n", j.Error(), errors.Is(j, io.EOF), errors.As(j, &target))
	fmt.Println(errors.Join() == nil, errors.Join(nil, nil) == nil)
	m := Multi{ValErr{"a"}, fmt.Errorf("b: %w", ErrBase)}
	fmt.Println(m, errors.Is(m, ErrBase), errors.As(m, &ve), ve.Msg)
	deep := error(ErrBase)
	for i := 0; i < 50; i++ {
		deep = fmt.Errorf("l%d: %w", i, deep)
	}
	fmt.Println(len(deep.Error()), errors.Is(deep, ErrBase))
	fmt.Printf("%v|%s|%q|%+v|%x\n", e1, e1, e1, e1, e1)
	fmt.Println(errors.Is(nil, nil), errors.Is(nil, ErrBase), errors.Is(ErrBase, nil))
	func() {
		defer func() { fmt.Println("as panic:", recover()) }()
		errors.As(ErrBase, nil)
	}()
	func() {
		defer func() { fmt.Println("as panic:", recover()) }()
		var s string
		errors.As(ErrBase, &s)
	}()
	fmt.Println(errors.ErrUnsupported, io.ErrUnexpectedEOF, fs.ErrNotExist)
}
