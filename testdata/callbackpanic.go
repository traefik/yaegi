package main

// A panic in interpreted code called back by native code (a String method
// called by fmt, a func given to sort.Slice or strings.Map): the native code
// recovers the value of the program, as in gc. fmt printed
// "%!v(PANIC=String method: panic: boom)" in default mode and in AOT mode:
// it recovered the VM's own panic value, whose Error method adds "panic: ".
// A panic that goes through the native code to the program is recovered
// with the same value and type. (From the reflect battery of the modes
// challenge, case fmt_stringer.)

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type S struct{ v any }

func (s S) String() string { panic(s.v) }

type E struct{ Code int }

func (e E) Error() string { return fmt.Sprint("code ", e.Code) }

type Name string

func main() {
	for _, v := range []any{"boom", 42, 2.5, true, io.EOF, errors.New("plain"), fmt.Errorf("wrapped: %w", io.EOF)} {
		fmt.Printf("%v|%s|%q\n", S{v}, S{v}, S{v})
		fmt.Println(S{v})
	}
	// values of the program's own types
	fmt.Println(S{E{3}}, S{Name("n")})

	// a panic through native code, recovered by the program
	for _, v := range []any{"cmp", 7, io.EOF, E{4}, Name("x"), &E{5}, []int{1}} {
		func() {
			defer func() {
				r := recover()
				if s, ok := v.([]int); ok {
					fmt.Printf("recovered %T %v\n", r, &r.([]int)[0] == &s[0])
					return
				}
				fmt.Printf("recovered %T %v %v\n", r, r, r == v)
			}()
			s := []int{3, 1, 2}
			sort.Slice(s, func(i, j int) bool { panic(v) })
		}()
	}
	func() {
		defer func() { fmt.Println("strings.Map:", recover()) }()
		strings.Map(func(r rune) rune { panic("in map") }, "abc")
	}()
	func() {
		defer func() {
			r := recover()
			err, ok := r.(error)
			fmt.Println("error:", ok, errors.Is(err, io.EOF))
		}()
		strings.Map(func(r rune) rune { panic(fmt.Errorf("x: %w", io.EOF)) }, "abc")
	}()
}
