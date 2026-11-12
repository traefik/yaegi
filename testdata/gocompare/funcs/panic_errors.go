// Panics with custom error types: errors.Is and errors.As on recovered
// values through wrapping chains, value vs pointer receivers of Error,
// panics converted to returned errors by a deferred recover, recover in each
// iteration of a loop, and the message of a panic raised inside a deferred
// call while another panic is in flight.
package main

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
)

var ErrNotFound = errors.New("not found")

type valErr struct{ code int }

func (v valErr) Error() string { return "valErr " + strconv.Itoa(v.code) }

type ptrErr struct{ msg string }

func (p *ptrErr) Error() string { return "ptrErr " + p.msg }
func (p *ptrErr) Unwrap() error { return ErrNotFound }

type multi struct{ errs []error }

func (m multi) Error() string   { return fmt.Sprint(len(m.errs), " errors") }
func (m multi) Unwrap() []error { return m.errs }

func safe(f func()) (err error) {
	defer func() {
		r := recover()
		switch x := r.(type) {
		case nil:
		case runtime.Error:
			err = fmt.Errorf("runtime: %w", x)
		case error:
			err = fmt.Errorf("error: %w", x)
		default:
			err = fmt.Errorf("value: %v", x)
		}
	}()
	f()
	return nil
}

func describe(err error) string {
	var ve valErr
	var pe *ptrErr
	var re runtime.Error
	return fmt.Sprintf("%v | is NotFound=%v as valErr=%v as ptrErr=%v as runtime=%v",
		err, errors.Is(err, ErrNotFound), errors.As(err, &ve), errors.As(err, &pe), errors.As(err, &re))
}

func main() {
	cases := []func(){
		func() {},
		func() { panic(ErrNotFound) },
		func() { panic(valErr{3}) },
		func() { panic(&ptrErr{"x"}) },
		func() { panic(fmt.Errorf("wrap: %w", &ptrErr{"y"})) },
		func() { panic(multi{[]error{valErr{1}, ErrNotFound}}) },
		func() { panic(errors.Join(valErr{2}, &ptrErr{"j"})) },
		func() { panic("text") },
		func() { panic(42) },
		func() {
			var m map[int]int
			m[1] = 1
		},
		func() {
			a := []int{}
			_ = a[len(a)]
		},
	}
	for i, c := range cases {
		err := safe(c)
		if err == nil {
			fmt.Println(i, "no error")
			continue
		}
		fmt.Println(i, describe(err))
	}
	// Recover in each iteration of a loop.
	var results []string
	for i := 0; i < 5; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					results = append(results, fmt.Sprint("r", r))
				}
			}()
			if i%2 == 1 {
				panic(i)
			}
			results = append(results, fmt.Sprint("ok", i))
		}()
	}
	fmt.Println(results)
	// A deferred call panics while another panic is in flight.
	err := safe(func() {
		defer func() {
			panic(valErr{99})
		}()
		panic(&ptrErr{"first"})
	})
	fmt.Println(describe(err))
	// The deferred function recovers the first panic and then panics.
	err = safe(func() {
		defer func() {
			r := recover()
			panic(fmt.Errorf("after %v: %w", r, ErrNotFound))
		}()
		panic("first")
	})
	fmt.Println(describe(err))
	// Comparing recovered errors.
	err = safe(func() { panic(valErr{5}) })
	fmt.Println(errors.Unwrap(err) == valErr{5}, errors.Is(err, valErr{5}), errors.Is(err, valErr{6}))
	fmt.Printf("%v\n%+v\n%q\n", err, err, err)
}
