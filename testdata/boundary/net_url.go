// Native boundary of net/url: program types given to net/url (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
	"net/url"
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

// tmoErr has Timeout and Temporary (value receivers).
type tmoErr struct{ s string }

func (e tmoErr) Error() string   { return "tmo:" + e.s }
func (e tmoErr) Timeout() bool   { return true }
func (e tmoErr) Temporary() bool { return false }

// tmpErr has Timeout and Temporary (pointer receivers).
type tmpErr struct{ s string }

func (e *tmpErr) Error() string   { return "tmp:" + e.s }
func (e *tmpErr) Timeout() bool   { return false }
func (e *tmpErr) Temporary() bool { return true }

// onlyTimeout has only Timeout.
type onlyTimeout int

func (e onlyTimeout) Error() string { return fmt.Sprintf("only-timeout:%d", int(e)) }
func (e onlyTimeout) Timeout() bool { return e > 0 }

// gErr is a generic error with Timeout and Temporary.
type gErr[T any] struct{ v T }

func (e gErr[T]) Error() string   { return fmt.Sprintf("g:%v", e.v) }
func (e gErr[T]) Timeout() bool   { return true }
func (e gErr[T]) Temporary() bool { return true }

// plainErr has no Timeout and no Temporary.
type plainErr string

func (e plainErr) Error() string { return string(e) }

// wrapErr wraps a timeout error: url.Error does not unwrap it.
type wrapErr struct{ inner error }

func (e *wrapErr) Error() string { return "wrap(" + e.inner.Error() + ")" }
func (e *wrapErr) Unwrap() error { return e.inner }

func main() {
	errs := []error{
		tmoErr{"a"}, &tmpErr{"b"}, onlyTimeout(1), onlyTimeout(0),
		gErr[string]{"c"}, plainErr("d"), &wrapErr{tmoErr{"e"}},
	}
	// row: net/url (*Error).Timeout assert x.(interface{Timeout() bool})
	probe("Error.Timeout", func() {
		for i, e := range errs {
			ue := &url.Error{Op: "Get", URL: "http://x/", Err: e}
			out("%d: %v %q", i, ue.Timeout(), ue.Error())
		}
		var ie error = &url.Error{Op: "Get", URL: "u", Err: tmoErr{"iface"}}
		var t interface{ Timeout() bool }
		out("As: %v %v", errors.As(ie, &t), t != nil && t.Timeout())
	})
	// row: net/url (*Error).Temporary assert x.(interface{Temporary() bool})
	probe("Error.Temporary", func() {
		for i, e := range errs {
			ue := &url.Error{Op: "Post", URL: "http://x/", Err: e}
			out("%d: %v", i, ue.Temporary())
		}
		var target *tmpErr
		ue := &url.Error{Op: "Get", URL: "u", Err: &tmpErr{"as"}}
		out("As: %v %v", errors.As(ue, &target), target != nil && target.s == "as")
	})
}
