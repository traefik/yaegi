// A type assertion of a native value to an interface checks the signature
// of each method, not only its name: err.(interface{ Unwrap() []error }) on
// the *fmt.wrapError of fmt.Errorf("%w", e), whose Unwrap returns error, is
// false (TestErrorf of package fmt). It was true, and the call of Unwrap
// then panicked.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type myWriter interface {
	Write([]byte) (int, error)
}

func check(name string, ok bool) { fmt.Println(name, ok) }

func main() {
	err := fmt.Errorf("%w", errors.New("inner"))
	_, ok := err.(interface{ Unwrap() []error })
	check("Unwrap() []error", ok)
	_, ok = err.(interface{ Unwrap() error })
	check("Unwrap() error", ok)

	multi := fmt.Errorf("%w %w", errors.New("a"), errors.New("b"))
	_, ok = multi.(interface{ Unwrap() []error })
	check("multi Unwrap() []error", ok)
	_, ok = multi.(interface{ Unwrap() error })
	check("multi Unwrap() error", ok)

	var x any = err
	switch x.(type) {
	case interface{ Unwrap() []error }:
		fmt.Println("switch: multi")
	case interface{ Unwrap() error }:
		fmt.Println("switch: single")
	}

	var r any = strings.NewReader("abc")
	_, ok = r.(interface{ Len() int })
	check("Len() int", ok)
	_, ok = r.(interface{ Len() int64 })
	check("Len() int64", ok)
	_, ok = r.(interface{ Read([]byte) (int, error) })
	check("Read([]byte) (int, error)", ok)
	_, ok = r.(interface{ Read([]byte) int })
	check("Read([]byte) int", ok)
	_, ok = r.(interface {
		WriteTo(io.Writer) (int64, error)
	})
	check("WriteTo(io.Writer)", ok)
	_, ok = r.(interface {
		WriteTo(myWriter) (int64, error)
	})
	check("WriteTo(myWriter)", ok)
	_, ok = r.(interface {
		ReadAt([]byte, int64) (int, error)
	})
	check("ReadAt", ok)
	_, ok = r.(interface {
		Seek(int64, int) (int64, error)
	})
	check("Seek", ok)
	_, ok = r.(interface{ Seek(int, int) (int64, error) })
	check("Seek(int, int)", ok)

	// Unnamed types in the signature (<-chan struct{} of Done) are compared
	// by their structure.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c any = ctx
	_, ok = c.(context.Context)
	check("context.Context", ok)
	_, ok = c.(interface{ Done() <-chan struct{} })
	check("Done() <-chan struct{}", ok)
	_, ok = c.(interface{ Done() chan struct{} })
	check("Done() chan struct{}", ok)
	_, ok = c.(interface{ Done() <-chan struct{ X int } })
	check("Done() <-chan struct{ X int }", ok)
	_, ok = c.(interface{ Value(any) any })
	check("Value(any) any", ok)
	var h any = strings.NewReplacer("a", "b")
	_, ok = h.(interface{ Replace(string) string })
	check("Replace(string) string", ok)
	_, ok = h.(interface {
		WriteString(io.Writer, string) (int, error)
	})
	check("WriteString(io.Writer, string)", ok)
}
