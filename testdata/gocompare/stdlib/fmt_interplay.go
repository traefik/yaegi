// fmt with program types passed through other packages: Stringer, error,
// GoStringer and Formatter values inside slices, maps, structs, pointers
// and interfaces, printed by fmt, log (to a buffer), strings.Builder,
// text/template and errors wrapping; and fmt.Sscan into program types.
// (log with a Formatter and the message of a panicking String method are
// left out: they differ in yaegi v2 today.)
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"
	"text/template"
)

type Money struct {
	Cents int64
	Cur   string
}

func (m Money) String() string { return fmt.Sprintf("%d.%02d %s", m.Cents/100, m.Cents%100, m.Cur) }

type PtrStringer struct{ N int }

func (p *PtrStringer) String() string { return fmt.Sprint("ptr#", p.N) }

type Code int

func (c Code) Error() string    { return fmt.Sprintf("E%03d", int(c)) }
func (c Code) GoString() string { return fmt.Sprintf("Code(%d)", int(c)) }

type Matrix [2][2]int

func (m Matrix) Format(f fmt.State, verb rune) {
	w, wok := f.Width()
	p, pok := f.Precision()
	fmt.Fprintf(f, "M<%c w=%d/%v p=%d/%v plus=%v minus=%v sharp=%v zero=%v space=%v>", verb, w, wok, p, pok, f.Flag('+'), f.Flag('-'), f.Flag('#'), f.Flag('0'), f.Flag(' '))
	fmt.Fprint(f, m[0], m[1])
}

type Wrapper struct {
	M   Money
	MP  *Money
	P   PtrStringer
	PP  *PtrStringer
	C   Code
	Any any
	Err error
}

type Celsius float64

type Pt struct{ X, Y int }

func (p *Pt) Scan(state fmt.ScanState, verb rune) error {
	_, err := fmt.Fscanf(state, "(%d,%d)", &p.X, &p.Y)
	return err
}

type Panicky struct{}

func (Panicky) String() string { panic("stringer panicked") }

type NilPtr struct{ s string }

func (n *NilPtr) String() string { return "nilptr:" + n.s }

func main() {
	m := Money{12345, "EUR"}
	w := Wrapper{M: m, MP: &m, P: PtrStringer{1}, PP: &PtrStringer{2}, C: 7, Any: Code(9), Err: fmt.Errorf("wrapped: %w", Code(3))}
	fmt.Printf("%v\n%+v\n", w, w)
	fmt.Printf("%s|%d|%x|%q\n", m, m, m, m)
	fmt.Printf("%v %v %s %d\n", []Money{m, {5, "USD"}}, map[Money]Code{{1, "A"}: 1, {0, "B"}: 2}, []Code{1, 22}, []Code{1, 22})
	fmt.Printf("%#v %#v %#v\n", Code(4), []Code{5}, map[string]Code{"k": 6})
	fmt.Printf("%#v\n", Money{1, "X"})
	fmt.Printf("%v %s %d\n", Matrix{{1, 2}, {3, 4}}, Matrix{}, Matrix{{5}})
	fmt.Printf("%+8.3x|%-#05v|% v\n", Matrix{}, Matrix{}, Matrix{})
	fmt.Printf("[%10s] [%-10v] [%.3s] [%q]\n", m, Code(1), m, Code(2))
	fmt.Println(&PtrStringer{3}, PtrStringer{4}, []*PtrStringer{{5}}, []PtrStringer{{6}})
	var np *NilPtr
	fmt.Println(np == nil)
	fmt.Printf("%v\n", np)
	fmt.Printf("%d %!\n", m)
	fmt.Printf("%d %d\n", 1)
	fmt.Printf("%z %v\n", Celsius(1.5), Celsius(2))
	fmt.Println(fmt.Sprint(m, "x", 3, 4, "y", Code(5)), fmt.Sprintln(m, Code(1)))
	fmt.Println(fmt.Append([]byte("pre:"), m, Code(2)), string(fmt.Appendf(nil, "%v|%v", m, &m)))
	var buf bytes.Buffer
	lg := log.New(&buf, "[app] ", log.Lmsgprefix)
	lg.Printf("money=%v code=%v err=%v", m, Code(8), w.Err)
	lg.Println("line", m, Code(9))
	lg.SetPrefix("> ")
	lg.SetFlags(0)
	lg.Print(m)
	fmt.Print(buf.String())
	err := fmt.Errorf("level2: %w", fmt.Errorf("level1: %w", Code(42)))
	var c Code
	fmt.Println(err, errors.As(err, &c), c, fmt.Sprintf("%#v", c))
	t := template.Must(template.New("t").Parse("{{.M}} {{.MP}} {{.P}} {{.PP}} {{.C}} {{.Any}} {{.Err}} {{printf \"%05d\" .C}}\n"))
	t.Execute(&buf, w)
	fmt.Print(buf.String()[strings.LastIndex(buf.String()[:buf.Len()-1], "\n")+1:])
	// Scanning into program types.
	var p Pt
	var name string
	var cel Celsius
	var code Code
	n, err := fmt.Sscanf("pt=(3,-4) bob 21.5 77", "pt=%v %s %g %d", &p, &name, &cel, &code)
	fmt.Println(n, err, p, name, cel, code)
	n, err = fmt.Sscan("(1,2) (5,6)", &p, &p)
	fmt.Println(n, err, p)
	n, err = fmt.Sscanf("(x,1)", "%v", &p)
	fmt.Println(n, err)
	var a, b int
	n, err = fmt.Sscanln("10 20\n30", &a, &b)
	fmt.Println(n, err, a, b)
	n, err = fmt.Fscan(strings.NewReader("7"), &a, &b)
	fmt.Println(n, err, a)
}
