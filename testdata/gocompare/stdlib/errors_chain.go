// errors with program error types: Is and As through Unwrap chains,
// custom Is and As methods, errors.Join, fmt.Errorf with several %w,
// errors.As into interfaces, and value versus pointer receivers.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
)

var ErrNotFound = errors.New("not found")

type NotFound struct{ Name string }

func (e NotFound) Error() string { return "not found: " + e.Name }

type QueryError struct {
	Query string
	Err   error
}

func (e *QueryError) Error() string { return "query " + e.Query + ": " + e.Err.Error() }
func (e *QueryError) Unwrap() error { return e.Err }

type CodeErr int

func (c CodeErr) Error() string { return "code " + strconv.Itoa(int(c)) }
func (c CodeErr) Is(target error) bool {
	t, ok := target.(CodeErr)
	return ok && (t == 0 || t/100 == c/100)
}

type Temporary interface{ Temporary() bool }

type TempErr struct{ msg string }

func (t *TempErr) Error() string   { return t.msg }
func (t *TempErr) Temporary() bool { return true }

type Multi struct{ errs []error }

func (m Multi) Error() string   { return fmt.Sprint(len(m.errs), " errors") }
func (m Multi) Unwrap() []error { return m.errs }

type Converter struct{ inner error }

func (c Converter) Error() string { return "converter" }
func (c Converter) As(target any) bool {
	if p, ok := target.(*CodeErr); ok {
		*p = 999
		return true
	}
	return false
}

func describe(err error) {
	var nf NotFound
	var nfp *NotFound
	var qe *QueryError
	var ce CodeErr
	var tmp Temporary
	var pe *fs.PathError
	fmt.Printf("%v\n  is notfound=%v eof=%v code404=%v code0=%v code500=%v errnf=%v\n", err,
		errors.Is(err, ErrNotFound), errors.Is(err, io.EOF), errors.Is(err, CodeErr(404)), errors.Is(err, CodeErr(0)), errors.Is(err, CodeErr(500)), errors.Is(err, NotFound{"x"}))
	fmt.Printf("  as nf=%v(%v) nfp=%v qe=%v ce=%v(%v) tmp=%v pe=%v\n", errors.As(err, &nf), nf.Name, errors.As(err, &nfp), errors.As(err, &qe), errors.As(err, &ce), int(ce), errors.As(err, &tmp), errors.As(err, &pe))
	if qe != nil {
		fmt.Println("  query:", qe.Query)
	}
	if tmp != nil {
		fmt.Println("  temporary:", tmp.Temporary())
	}
	fmt.Printf("  unwrap=%v\n", errors.Unwrap(err))
}

func main() {
	errs := []error{
		ErrNotFound,
		NotFound{"x"},
		NotFound{"y"},
		&NotFound{"ptr"},
		fmt.Errorf("wrap: %w", NotFound{"x"}),
		&QueryError{"select", ErrNotFound},
		fmt.Errorf("outer: %w", &QueryError{"q2", fmt.Errorf("mid: %w", io.EOF)}),
		CodeErr(404),
		fmt.Errorf("c: %w", CodeErr(403)),
		&TempErr{"temp"},
		fmt.Errorf("t: %w", &TempErr{"deep temp"}),
		errors.Join(NotFound{"j"}, nil, CodeErr(500), &TempErr{"j3"}),
		errors.Join(nil, nil),
		fmt.Errorf("two: %w and %w", io.EOF, &QueryError{"q3", CodeErr(401)}),
		Multi{[]error{io.ErrUnexpectedEOF, NotFound{"m"}}},
		Converter{},
		&fs.PathError{Op: "open", Path: "a/b", Err: fs.ErrNotExist},
		fmt.Errorf("no wrap: %v", ErrNotFound),
		nil,
	}
	for i, e := range errs {
		fmt.Print(i, ": ")
		describe(e)
	}
	j := errors.Join(errors.New("a"), errors.New("b"))
	fmt.Printf("%q %v\n", j.Error(), errors.Join() == nil)
	if u, ok := j.(interface{ Unwrap() []error }); ok {
		fmt.Println(len(u.Unwrap()), u.Unwrap())
	}
	fmt.Println(errors.Is(fs.ErrNotExist, fs.ErrNotExist), errors.Is(nil, nil), errors.Is(nil, io.EOF))
	// errors.As panics on a bad target.
	for _, target := range []any{nil, 5, new(int), (*error)(nil)} {
		func() {
			defer func() { fmt.Println("as panic:", recover()) }()
			errors.As(io.EOF, target)
		}()
	}
	var target *QueryError
	err := error(&QueryError{"q", io.EOF})
	fmt.Println(errors.As(err, &target), target == err)
	fmt.Println(errors.AsType[*QueryError](fmt.Errorf("w: %w", err)))
	var _ = strconv.Itoa
	numErr := func() error { _, err := strconv.Atoi("12a"); return err }()
	var ne *strconv.NumError
	fmt.Println(errors.As(numErr, &ne), ne.Func, ne.Num, ne.Err, errors.Is(numErr, strconv.ErrSyntax))
}
