// fmt verbs on interpreted types: %v, %+v, %#v, %T, %s, %q, %x and %d on
// named structs, nested pointers, maps (sorted keys), slices of named types,
// Stringer, error and GoStringer methods (value and pointer receivers),
// Formatter, width and flags, Sprint spacing rules, Errorf with %w, and the
// Sscan family.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Color int

const (
	Red Color = iota
	Green
	Blue
)

func (c Color) String() string { return [...]string{"Red", "Green", "Blue"}[c] }

type Point struct {
	X, Y int
	tag  string
}

type PtrStringer struct{ N int }

func (p *PtrStringer) String() string { return fmt.Sprintf("PS(%d)", p.N) }

type GoStr struct{ A int }

func (g GoStr) GoString() string { return "GoStr!" }

type Money struct {
	Cents int64
	Cur   string
}

func (m Money) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v', 's':
		if f.Flag('+') {
			fmt.Fprintf(f, "%s %d.%02d", m.Cur, m.Cents/100, m.Cents%100)
			return
		}
		fmt.Fprintf(f, "%d.%02d", m.Cents/100, m.Cents%100)
	default:
		fmt.Fprintf(f, "%%!%c(Money)", verb)
	}
}

type ErrNotFound struct{ Key string }

func (e ErrNotFound) Error() string { return "not found: " + e.Key }

type Both struct{}

func (Both) String() string { return "string" }
func (Both) Error() string  { return "error wins" }

type Tree struct {
	Val   int
	Left  *Tree
	Kids  []*Tree
	Attrs map[string]Color
}

type Bytes []byte
type Names []string

func main() {
	p := Point{1, 2, "t"}
	pp := &p
	fmt.Printf("%v|%+v|%#v|%T\n", p, p, p, p)
	fmt.Printf("%v|%+v|%T\n", pp, pp, pp)
	ppp := &pp
	fmt.Printf("%T %v\n", ppp, **ppp)

	fmt.Printf("%v %d %s %q %x %08b %T\n", Blue, Blue, Green, Red, Blue, Blue, Blue)
	fmt.Printf("%v %+v\n", []Color{Red, Blue}, map[Color]int{Blue: 2, Red: 0})
	fmt.Printf("%#v\n", []Color{Red})

	ps := PtrStringer{3}
	fmt.Printf("%v %v %s\n", ps, &ps, []*PtrStringer{&ps})

	fmt.Printf("%#v %v %+v\n", GoStr{1}, GoStr{1}, GoStr{1})

	m := Money{12345, "EUR"}
	fmt.Printf("%v|%+v|%s|%d|%10v|\n", m, m, m, m, []Money{m})
	fmt.Println(m, fmt.Sprint(m))

	var err error = ErrNotFound{"k"}
	fmt.Printf("%v|%s|%q|%+v|%#v\n", err, err, err, err, err)
	fmt.Println(Both{}, fmt.Sprintf("%s", Both{}))

	t := &Tree{Val: 1, Left: &Tree{Val: 2}, Kids: []*Tree{{Val: 3}}, Attrs: map[string]Color{"z": Blue, "a": Red}}
	fmt.Printf("%v %v\n%+v\n", t.Val, t.Left.Val, *t.Left)
	fmt.Printf("%v %+v\n", t.Attrs, t.Kids[0].Attrs)

	fmt.Printf("%s %x %X %q %v\n", Bytes("hi"), Bytes("hi"), Bytes{0xab}, Bytes("q"), Bytes("v"))
	fmt.Printf("%q %v %s %d\n", Names{"a", "b"}, Names{}, Names(nil), []int(nil))

	// Width, precision, flags on strings and slices.
	fmt.Printf("[%5s][%-5s][%.2s][%5.1q][%x][% x][%#x]\n", "ab", "ab", "abc", "abc", "hi", "hi", "hi")
	fmt.Printf("[%6.2v][%-8v][%08d][%+.1e]\n", 3.14159, Green, -42, 12345.678)
	fmt.Printf("[%v][%d][%s]\n", [3]int{1, 2, 3}, []int8{-1, 2}, []string{"a b", "c"})
	fmt.Printf("%v %v %v\n", struct{}{}, struct{ A *int }{}, [0]int{})
	fmt.Printf("%!|%z|%d\n", 1, "extra")
	fmt.Printf("%[2]d %[1]d %d\n", 10, 20)

	// Sprint spacing: spaces between operands when neither is a string.
	fmt.Println(fmt.Sprint("a", 1, 2, "b", 3.5, Red, Blue))
	fmt.Println(fmt.Sprintln("x", 1) == "x 1\n")

	// nil values.
	var np *Point
	var ns fmt.Stringer
	var nm map[string]int
	var nps *PtrStringer
	fmt.Printf("%v %+v %v %v %v %d\n", np, ns, nm, nps == nil, []any{nil}, nm)

	// Errorf, %w, errors.Is/As through several layers.
	base := ErrNotFound{"cfg"}
	w1 := fmt.Errorf("load: %w", base)
	w2 := fmt.Errorf("start: %w (%w)", w1, os.ErrNotExist)
	fmt.Println(w2)
	var nf ErrNotFound
	fmt.Println(errors.Is(w2, base), errors.Is(w2, os.ErrNotExist), errors.As(w2, &nf), nf.Key)
	joined := errors.Join(errors.New("e1"), nil, w1)
	fmt.Printf("%q %v\n", joined.Error(), errors.Is(joined, base))
	fmt.Println(errors.Unwrap(w1) == error(base), errors.Unwrap(w2) == nil)

	// Sscan family into interpreted named types.
	var c Color
	var name string
	var f float64
	n, err := fmt.Sscanf("2 bob 1.5", "%d %s %g", &c, &name, &f)
	fmt.Println(n, err, c, name, f)
	var a, b int
	n, err = fmt.Sscan("7\n8", &a, &b)
	fmt.Println(n, err, a, b)
	n, err = fmt.Sscanln("9 x", &a, &b)
	fmt.Println(n, err != nil, a)

	// Fprint to a strings.Builder and to os.Stdout.
	var sb strings.Builder
	fmt.Fprintf(&sb, "%v-%v", Point{3, 4, ""}, Blue)
	fmt.Fprintln(os.Stdout, sb.String())
	fmt.Print("print ", 1, 2, "\n")

	// Append variants.
	buf := fmt.Appendf(nil, "%d-%s", 5, Red)
	buf = fmt.Append(buf, " ", Green)
	buf = fmt.Appendln(buf, "", Blue)
	fmt.Print(string(buf))
}
