package main

// errors.Is, errors.As, errors.AsType and errors.Unwrap with interpreted
// error types: their Is, As and Unwrap methods, equality of value errors,
// and interpreted errors wrapped by native ones (fmt.Errorf %w,
// errors.Join).

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

type wrapErr struct{ err error }

func (e *wrapErr) Error() string { return "w" }
func (e *wrapErr) Unwrap() error { return e.err }

type isErr struct{}

func (isErr) Error() string        { return "is" }
func (isErr) Is(target error) bool { return target == io.EOF }

type intErr struct{ n int }

func (e intErr) Error() string { return "e" }

type multiErr []error

func (m multiErr) Error() string   { return "multi" }
func (m multiErr) Unwrap() []error { return m }

type asErr struct{}

func (asErr) Error() string { return "as" }
func (asErr) As(target any) bool {
	if p, ok := target.(*intErr); ok {
		*p = intErr{42}
		return true
	}
	return false
}

type timeout interface{ Timeout() bool }

type timeoutError interface {
	error
	Timeout() bool
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Unwrap() []error { return nil }

var sentinel = &wrapErr{}

func main() {
	fmt.Println(errors.Is(&wrapErr{io.EOF}, io.EOF))
	fmt.Println(errors.Is(isErr{}, io.EOF))
	fmt.Println(errors.Is(fmt.Errorf("x: %w", sentinel), sentinel))
	fmt.Println(errors.Is(intErr{1}, intErr{1}))
	var e error = intErr{1}
	fmt.Println(errors.Is(e, e))

	// negative cases and nil
	fmt.Println(errors.Is(intErr{1}, intErr{2}), errors.Is(&wrapErr{}, &wrapErr{}))
	fmt.Println(errors.Is(nil, nil), errors.Is(nil, io.EOF), errors.Is(io.EOF, nil))

	// non comparable target: only its Is method can match
	fmt.Println(errors.Is(multiErr{io.EOF}, multiErr{io.EOF}))
	fmt.Println(errors.Is(multiErr{intErr{3}, isErr{}}, io.EOF))
	fmt.Println(errors.Is(errors.Join(io.ErrUnexpectedEOF, &wrapErr{intErr{7}}), intErr{7}))

	// errors.As
	var ie intErr
	fmt.Println(errors.As(&wrapErr{fmt.Errorf("y: %w", intErr{5})}, &ie), ie.n)
	fmt.Println(errors.As(asErr{}, &ie), ie.n)
	var we *wrapErr
	fmt.Println(errors.As(multiErr{io.EOF, sentinel}, &we), we == sentinel)
	var to timeout
	fmt.Println(errors.As(fmt.Errorf("z: %w", timeoutErr{}), &to), to != nil && to.Timeout())
	_, err := os.Open("/does/not/exist")
	var pe *fs.PathError
	fmt.Println(errors.As(&wrapErr{err}, &pe), pe.Op, errors.Is(&wrapErr{err}, fs.ErrNotExist))
	fmt.Println(errors.As(io.EOF, &we), errors.As(nil, &we))

	// errors.AsType (generic: no native instance)
	ie2, ok := errors.AsType[intErr](&wrapErr{fmt.Errorf("x: %w", intErr{4})})
	fmt.Println(ie2.n, ok)
	ie2, ok = errors.AsType[intErr](asErr{})
	fmt.Println(ie2.n, ok)
	we2, ok := errors.AsType[*wrapErr](errors.Join(io.EOF, sentinel))
	fmt.Println(we2 == sentinel, ok)
	to2, ok := errors.AsType[timeoutError](multiErr{io.EOF, timeoutErr{}})
	fmt.Println(to2 != nil && to2.Timeout(), ok)
	pe2, ok := errors.AsType[*fs.PathError](&wrapErr{err})
	fmt.Println(pe2.Op, ok)
	_, ok = errors.AsType[intErr](io.EOF)
	_, ok2 := errors.AsType[intErr](nil)
	fmt.Println(ok, ok2)

	// errors.Unwrap
	fmt.Println(errors.Unwrap(&wrapErr{io.EOF}) == io.EOF, errors.Unwrap(multiErr{io.EOF}) == nil)
	fmt.Println(errors.Unwrap(fmt.Errorf("x: %w", sentinel)) == sentinel)

	defer func() { fmt.Println("recovered:", recover()) }()
	errors.As(io.EOF, ie)
}
