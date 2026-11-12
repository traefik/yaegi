package main

import (
	"errors"
	"sync/atomic"
)

type wrappedError struct {
	wrapped error
}

func (e *wrappedError) Error() string {
	return "some outer error"
}

func (e *wrappedError) Unwrap() error {
	return e.wrapped
}

var err atomic.Value

func getWrapped() *wrappedError {
	if v := err.Load(); v != nil {
		err := v.(*wrappedError)
		if err.wrapped != nil {
			return err
		}
	}
	return nil
}

func main() {
	err.Store(&wrappedError{wrapped: errors.New("test")})

	e := getWrapped()
	if e != nil {
		println(e.Error())
		println(e.wrapped.Error())
	}
}

// Output:
// some outer error
// test
