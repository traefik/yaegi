// fmt and the methods of program types: String, GoString, Format, Error
// with value and pointer receivers, promoted through embedding, inside
// slices, maps and struct fields, the priority between them, the verbs
// that bypass them (%d on a Stringer int, %T), and panics in String
// and Error (recovered by fmt and printed as %!v(PANIC=...)).
package main

import (
	"errors"
	"fmt"
	"strings"
)

type Color int

func (c Color) String() string {
	switch c {
	case 0:
		return "red"
	case 1:
		return "green"
	}
	return fmt.Sprintf("Color(%d)", int(c))
}

type PtrStr struct{ N int }

func (p *PtrStr) String() string { return fmt.Sprintf("PtrStr<%d>", p.N) }

type Go struct{ V string }

func (g Go) GoString() string { return "Go!" + g.V }
func (g Go) String() string   { return "str:" + g.V }

type Formatted struct{ V int }

func (f Formatted) Format(s fmt.State, verb rune) {
	w, wok := s.Width()
	p, pok := s.Precision()
	flags := ""
	for _, c := range "+-# 0" {
		if s.Flag(int(c)) {
			flags += string(c)
		}
	}
	fmt.Fprintf(s, "F[%c w=%d,%v p=%d,%v f=%q v=%d]", verb, w, wok, p, pok, flags, f.V)
}

type MyErr struct{ Code int }

func (e MyErr) Error() string { return fmt.Sprintf("myerr %d", e.Code) }

type ErrAndStr struct{}

func (ErrAndStr) Error() string  { return "as-error" }
func (ErrAndStr) String() string { return "as-string" }

type Embeds struct {
	Color
	Extra int
}

type EmbedsPtr struct {
	*PtrStr
}

type Wrapper struct {
	C Color
	P PtrStr
	G Go
	E error
}

type Panicky struct{ msg string }

func (p Panicky) String() string {
	if p.msg == "" {
		panic("empty message")
	}
	if p.msg == "err" {
		panic(errors.New("an error value"))
	}
	return p.msg
}

type NilOK struct{}

func (n *NilOK) String() string {
	if n == nil {
		return "nil NilOK"
	}
	return "NilOK"
}

type NilBad struct{ s string }

func (n *NilBad) String() string { return n.s }

type Bytes []byte

func (b Bytes) String() string { return strings.ToUpper(string(b)) }

type Stringerless int

func main() {
	c := Color(1)
	fmt.Println(c, Color(0), Color(7), []Color{0, 1, 2}, map[Color]int{1: 10, 0: 5})
	fmt.Printf("%v %s %d %x %q %+v %#v %T %5s| %-8v| %.2s\n", c, c, c, c, c, c, c, c, c, c, c)
	p := PtrStr{3}
	fmt.Println(p, &p, []PtrStr{{1}}, []*PtrStr{{2}})
	fmt.Printf("%v %v %+v %s %d\n", p, &p, &p, &p, &p)
	g := Go{"x"}
	fmt.Printf("%v %#v %+v %s %#v %#v\n", g, g, g, g, []Go{{"a"}}, map[string]Go{"k": {"v"}})
	f := Formatted{5}
	fmt.Printf("%v|%+v|%#v|%-8.3x|%08d|% s|%q|%T\n", f, f, f, f, f, f, f, f)
	fmt.Println(f, []Formatted{{1}, {2}})
	fmt.Print(f, "\n")
	fmt.Printf("%[2]*[1]v\n", f, 4)
	e := MyErr{42}
	var err error = e
	fmt.Println(e, err, &e, []error{e, nil})
	fmt.Printf("%v %s %q %d %+v %#v %x\n", err, err, err, err, err, err, err)
	fmt.Println(ErrAndStr{})
	fmt.Printf("%v %s\n", ErrAndStr{}, ErrAndStr{})
	em := Embeds{Color: 1, Extra: 9}
	fmt.Println(em)
	fmt.Printf("%v %+v %#v %d\n", em, em, em, em)
	ep := EmbedsPtr{&PtrStr{8}}
	fmt.Println(ep)
	fmt.Printf("%v %+v\n", ep, EmbedsPtr{})
	w := Wrapper{C: 1, P: PtrStr{4}, G: Go{"y"}, E: MyErr{1}}
	fmt.Printf("%v\n%+v\n%#v\n", w, w, w)
	fmt.Println(Panicky{"fine"}, Panicky{}, Panicky{"err"})
	fmt.Printf("%v %s %d\n", Panicky{}, Panicky{}, Panicky{})
	var nok *NilOK
	var nbad *NilBad
	fmt.Println(nok, nbad)
	fmt.Printf("%v %s %+v %d\n", nok, nbad, nbad, nbad)
	fmt.Println(Bytes("abc"), []Bytes{Bytes("x")})
	fmt.Printf("%s %x %q %v %d\n", Bytes("ab"), Bytes("ab"), Bytes("ab"), Bytes("ab"), Bytes("ab"))
	fmt.Println(Stringerless(3), any(Color(0)))
	var st fmt.Stringer = c
	fmt.Println(st, st.String())
	fmt.Println(fmt.Sprint(c, c), fmt.Sprint(c, 1, c), fmt.Sprintln(c, "x"))
}
