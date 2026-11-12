// AOT test program: errors on interpreted error types.
//
//aot:lib errors
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"time"
)

var ErrNotFound = errors.New("not found")

// sentinel value of an interpreted comparable type
type Code int

func (c Code) Error() string { return "code " + strconv.Itoa(int(c)) }

const ErrBusy Code = 503

// pointer error with Unwrap
type OpError struct {
	Op  string
	Err error
}

func (e *OpError) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *OpError) Unwrap() error { return e.Err }

// value error with a custom Is
type Temporary struct{ Reason string }

func (t Temporary) Error() string { return "temporary: " + t.Reason }
func (t Temporary) Is(target error) bool {
	_, ok := target.(Temporary)
	return ok
}

// error with As into another type
type Wrapped struct{ code Code }

func (w Wrapped) Error() string { return fmt.Sprint("wrapped ", int(w.code)) }
func (w Wrapped) As(target any) bool {
	if p, ok := target.(*Code); ok {
		*p = w.code
		return true
	}
	return false
}

// multi-error with Unwrap() []error
type Multi []error

func (m Multi) Error() string   { return fmt.Sprint(len(m), " errors") }
func (m Multi) Unwrap() []error { return m }

type Timeouter interface{ Timeout() bool }

type netErr struct{}

func (netErr) Error() string { return "net" }
func (netErr) Timeout() bool { return true }

func main() {
	if benchMain(benchOp) {
		return
	}
	e1 := &OpError{Op: "read", Err: ErrNotFound}
	fmt.Println(e1, errors.Is(e1, ErrNotFound), errors.Unwrap(e1) == ErrNotFound)
	e2 := fmt.Errorf("layer: %w", &OpError{"write", ErrBusy})
	fmt.Println(e2, errors.Is(e2, ErrBusy), errors.Is(e2, Code(500)), errors.Is(e2, ErrNotFound))
	var op *OpError
	fmt.Println(errors.As(e2, &op), op.Op, op.Err)
	var code Code
	fmt.Println(errors.As(e2, &code), code)
	e3 := fmt.Errorf("retry: %w", Temporary{"disk"})
	fmt.Println(errors.Is(e3, Temporary{}), errors.Is(e3, Temporary{"other"}), errors.Is(e3, ErrBusy))
	var c2 Code
	fmt.Println(errors.As(fmt.Errorf("x: %w", Wrapped{42}), &c2), c2)
	m := Multi{ErrNotFound, &OpError{"stat", fs.ErrNotExist}, ErrBusy}
	fmt.Println(errors.Is(m, fs.ErrNotExist), errors.Is(m, ErrBusy), errors.Is(m, io.EOF))
	var op2 *OpError
	fmt.Println(errors.As(m, &op2), op2.Op)
	j := errors.Join(ErrNotFound, nil, &OpError{"j", io.EOF})
	fmt.Printf("%q %v %v\n", j.Error(), errors.Is(j, io.EOF), errors.Is(j, ErrBusy))
	var to Timeouter
	fmt.Println(errors.As(fmt.Errorf("w: %w", netErr{}), &to), to != nil && to.Timeout())
	var ifaceErr error
	fmt.Println(errors.As(e1, &ifaceErr), ifaceErr)
	_, err := os.Open("/nonexistent/file")
	var pe *fs.PathError
	fmt.Println(errors.Is(err, fs.ErrNotExist), errors.As(err, &pe), pe.Op)
	fmt.Println(errors.Is(nil, ErrNotFound), errors.Is(ErrNotFound, nil), errors.Unwrap(ErrNotFound) == nil)
	defer func() {
		fmt.Println("recovered:", recover())
	}()
	errors.As(e1, nil)
}

var benchErr = fmt.Errorf("l1: %w", fmt.Errorf("l2: %w", &OpError{"deep", ErrBusy}))

// benchOp: Is and As through a chain of wrapped interpreted errors.
func benchOp() {
	var op *OpError
	if !errors.Is(benchErr, ErrBusy) || !errors.As(benchErr, &op) {
		panic("bench")
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
