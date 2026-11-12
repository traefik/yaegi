// Package errasrace is a plugin whose handler wraps errors of its own types
// and looks for them with errors.As and errors.Is. In AOT mode the AOT copy
// of errors gets the target of As (a pointer to a variable of the request)
// and the errors as wrappers that share one payload per equal value
// (optPayload, vm/optwrap.go), and writes the target.
package errasrace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// Code is an error of a comparable type without pointers.
type Code int

func (c Code) Error() string { return "code " + strconv.Itoa(int(c)) }

// OpError is a pointer error with Unwrap.
type OpError struct {
	Op  string
	Err error
}

func (e *OpError) Error() string { return e.Op + ": " + e.Err.Error() }
func (e *OpError) Unwrap() error { return e.Err }

// Temp is a value error with a string field.
type Temp struct{ Reason string }

func (t Temp) Error() string { return "temp: " + t.Reason }

// Ref is a value error that holds a pointer.
type Ref struct{ P *int }

func (r Ref) Error() string { return "ref " + strconv.Itoa(*r.P) }

var errBase = errors.New("base")

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.URL.Path[1:])
		n := id
		c, t, ref := Code(id), Temp{Reason: strconv.Itoa(id)}, Ref{P: &n}
		err := fmt.Errorf("req: %w", &OpError{Op: "op" + strconv.Itoa(id), Err: fmt.Errorf("%w %w %w", c, t, ref)})
		var (
			op  *OpError
			cc  Code
			tt  Temp
			rr  Ref
			res []string
		)
		res = append(res, strconv.FormatBool(errors.As(err, &op)), op.Op)
		res = append(res, strconv.FormatBool(errors.As(err, &cc)), strconv.Itoa(int(cc)))
		res = append(res, strconv.FormatBool(errors.As(err, &tt)), tt.Reason)
		res = append(res, strconv.FormatBool(errors.As(err, &rr)), strconv.Itoa(*rr.P))
		res = append(res, strconv.FormatBool(errors.Is(err, c)), strconv.FormatBool(errors.Is(err, t)),
			strconv.FormatBool(errors.Is(err, ref)), strconv.FormatBool(errors.Is(err, Code(id+1))),
			strconv.FormatBool(errors.Is(err, errBase)))
		fmt.Fprint(w, res)
	}), nil
}
