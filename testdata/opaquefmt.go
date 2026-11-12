// Native printers (fmt, log) with values given as `any`, with opaque
// handles (-opaque): %T and the other verbs must print the type and the
// value of the interpreted value, not of the handle.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
)

type T []int

type S struct {
	A int
	B string
}

type Str struct{ N int }

func (s Str) String() string { return fmt.Sprintf("Str(%d)", s.N) }

type E struct{ msg string }

func (e *E) Error() string { return "E: " + e.msg }

type F int

func (f F) Format(st fmt.State, verb rune) { fmt.Fprintf(st, "F<%c,%d>", verb, int(f)) }

func main() {
	// The case of the issue.
	fmt.Printf("%T %v\n", []int{1}, []int{1})
	var x any = []int{1}
	fmt.Println(fmt.Sprintf("%T", x))

	n := 3
	vals := []any{
		[]int{1, 2},
		map[string]int{"a": 1, "b": 2},
		[2]string{"x", "y"},
		&n,
		[]string{"p", "q"},
		map[int][]string{1: {"z"}},
		[][]int{{1}, {2, 3}},
		[]byte("hi"),
		struct{ A int }{4},
		T{5, 6},
		S{7, "s"},
		&S{8, "t"},
		[]S{{9, "u"}},
		map[string]S{"k": {10, "v"}},
		Str{11},
		[]Str{{12}},
		&E{"boom"},
		F(13),
		[]any{1, "a", []int{2}},
		42,
		"str",
	}
	for _, v := range vals {
		if p, ok := v.(*int); ok {
			fmt.Printf("%T %v\n", v, *p) // the address changes between runs
			continue
		}
		fmt.Printf("%T|%v|%+v|%#v|%d|%s|%x\n", v, v, v, v, v, v, v)
		fmt.Println(fmt.Sprintf("%T", v), fmt.Sprint(v), fmt.Sprintln(v) == fmt.Sprint(v)+"\n")
	}

	// Fprintf, Fprint, Append, to a native writer.
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%T %v;", []int{1}, map[string]bool{"t": true})
	fmt.Fprint(&buf, []int{2}, " ")
	fmt.Fprintln(&buf, []string{"w"})
	b := fmt.Appendf(nil, "%T", []float64{1.5})
	b = fmt.Append(b, " ", []int{3})
	fmt.Print(buf.String(), string(b), "\n")

	// Errorf, also with %w.
	err := fmt.Errorf("%T %v", []int{1}, []int{1})
	fmt.Println(err)
	base := errors.New("base")
	err = fmt.Errorf("%T: %w", map[string]int{}, base)
	fmt.Println(err, errors.Is(err, base))
	err = fmt.Errorf("%v %T", &E{"in"}, []struct{ A int }{{1}})
	fmt.Println(err)

	// log, to stdout, without the time.
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	log.Printf("%T %v", []int{1}, []int{1})
	log.Println([]string{"l"}, map[int]int{1: 2})
	log.Print(T{1})
	l := log.New(os.Stdout, "pfx ", 0)
	l.Printf("%T %#v", []int{1}, []int{1})
	var p interface{ Printf(string, ...any) } = l // a native method through an interface
	p.Printf("%T %v", map[string]int{"i": 1}, []int{1})

	// Printers called as func values.
	fs := []func(string, ...any) string{fmt.Sprintf}
	fmt.Println(fs[0]("%T %v", []int{1}, map[int]bool{1: true}))
	pf := l.Printf
	pf("%T", [][]string{{"m"}})

	// Other native code that formats a value.
	var sb strings.Builder
	fmt.Fprintf(&sb, "%T", []uint8{1})
	fmt.Println(sb.String())
}
