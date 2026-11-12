// Native boundary of fmt (print functions and methods): program types given
// to fmt (docs/BOUNDARY.md). fmt_2.go tests the reflect walk and fmt_3.go
// the scan functions.
package main

import (
	"errors"
	"fmt"
	"strings"
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

// each calls every print function of fmt with the same arguments.
func each(format string, args ...any) {
	out("Sprint %q", fmt.Sprint(args...))
	out("Sprintln %q", fmt.Sprintln(args...))
	out("Sprintf %q", fmt.Sprintf(format, args...))
	var b strings.Builder
	fmt.Fprint(&b, args...)
	out("Fprint %q", b.String())
	b.Reset()
	fmt.Fprintln(&b, args...)
	out("Fprintln %q", b.String())
	b.Reset()
	fmt.Fprintf(&b, format, args...)
	out("Fprintf %q", b.String())
	out("Append %q", string(fmt.Append([]byte("A:"), args...)))
	out("Appendln %q", string(fmt.Appendln([]byte("A:"), args...)))
	out("Appendf %q", string(fmt.Appendf([]byte("A:"), format, args...)))
	out("Errorf %q", fmt.Errorf(format, args...).Error())
	fmt.Print(append([]any{cur + ": Print "}, args...)...)
	fmt.Print("\n")
	fmt.Println(append([]any{cur + ": Println"}, args...)...)
	fmt.Printf(cur+": Printf "+format+"\n", args...)
}

// verbs prints each value with each verb.
func verbs(vs []any, vbs ...string) {
	for i, v := range vs {
		for _, vb := range vbs {
			out("%d %s %s", i, vb, fmt.Sprintf(vb, v))
		}
	}
}

// sv has a String method (value receiver).
type sv struct{ n int }

func (s sv) String() string { return fmt.Sprintf("sv<%d>", s.n) }

// sp has a String method (pointer receiver) that guards against nil.
type sp struct{ n int }

func (s *sp) String() string {
	if s == nil {
		return "sp<nil>"
	}
	return fmt.Sprintf("sp<%d>", s.n)
}

// spn has a String method (pointer receiver) without a nil guard.
type spn struct{ n int }

func (s *spn) String() string { return fmt.Sprintf("spn<%d>", s.n) }

// gstr is a generic type with a String method.
type gstr[T any] struct{ v T }

func (g gstr[T]) String() string { return fmt.Sprintf("gstr<%v>", g.v) }

// color is a named int with a String method.
type color int

func (c color) String() string { return [...]string{"red", "green", "blue"}[c] }

// word is a named string with a String method.
type word string

func (w word) String() string { return "word:" + strings.ToUpper(string(w)) }

// list is a named slice with a String method.
type list []int

func (l list) String() string { return fmt.Sprintf("list%d", len(l)) }

// dict is a named map with a String method.
type dict map[string]int

func (d dict) String() string { return fmt.Sprintf("dict%d", len(d)) }

// action is a named func with a String method.
type action func() int

func (a action) String() string { return fmt.Sprintf("action=%d", a()) }

// pipe is a named chan with a String method.
type pipe chan int

func (p pipe) String() string { return fmt.Sprintf("pipe cap=%d", cap(p)) }

// ev is an error (value receiver).
type ev struct{ msg string }

func (e ev) Error() string { return "ev:" + e.msg }

// ep is an error (pointer receiver).
type ep struct{ msg string }

func (e *ep) Error() string { return "ep:" + e.msg }

// gerr is a generic error.
type gerr[T any] struct{ v T }

func (e gerr[T]) Error() string { return fmt.Sprintf("gerr:%v", e.v) }

// errStr has both Error and String: Error wins.
type errStr struct{}

func (errStr) Error() string  { return "errStr.Error" }
func (errStr) String() string { return "errStr.String" }

// code is a named int that is an error.
type code int

func (c code) Error() string { return fmt.Sprintf("code %d", int(c)) }

// wrapE wraps another error.
type wrapE struct{ inner error }

func (w wrapE) Error() string { return "wrap(" + w.inner.Error() + ")" }
func (w wrapE) Unwrap() error { return w.inner }

// fv has a Format method (value receiver) that prints the state.
type fv struct{ n int }

func (f fv) Format(s fmt.State, verb rune) {
	w, wok := s.Width()
	p, pok := s.Precision()
	fmt.Fprintf(s, "fv[%c w=%d/%t p=%d/%t +%t -%t #%t sp%t 0%t n=%d]", verb, w, wok, p, pok,
		s.Flag('+'), s.Flag('-'), s.Flag('#'), s.Flag(' '), s.Flag('0'), f.n)
}

// fp has a Format method (pointer receiver) that uses fmt.FormatString.
type fp struct{ n int }

func (f *fp) Format(s fmt.State, verb rune) {
	fmt.Fprintf(s, "fp(%s)", fmt.FormatString(s, verb))
	fmt.Fprintf(s, fmt.FormatString(s, verb), f.n)
}

// gfmt is a generic type with a Format method.
type gfmt[T any] struct{ v T }

func (g gfmt[T]) Format(s fmt.State, verb rune) { fmt.Fprintf(s, "gfmt<%c,%v>", verb, g.v) }

// fmtErr is an error and a Formatter: Format wins, also for %w.
type fmtErr struct{}

func (fmtErr) Error() string                 { return "fmtErr.Error" }
func (fmtErr) Format(s fmt.State, verb rune) { fmt.Fprintf(s, "fmtErr.Format(%c)", verb) }
func (fmtErr) String() string                { return "fmtErr.String" }
func (fmtErr) GoString() string              { return "fmtErr.GoString" }

// gsv has GoString and String (value receivers).
type gsv struct{ n int }

func (g gsv) GoString() string { return fmt.Sprintf("gsv{N: %d}", g.n) }
func (g gsv) String() string   { return "gsv.String" }

// gsp has GoString (pointer receiver).
type gsp struct{ n int }

func (g *gsp) GoString() string { return fmt.Sprintf("&gsp{%d}", g.n) }

// ggs is a generic type with GoString.
type ggs[T any] struct{ v T }

func (g ggs[T]) GoString() string { return fmt.Sprintf("ggs(%#v)", g.v) }

// gsInt is a named int with GoString.
type gsInt int

func (g gsInt) GoString() string { return fmt.Sprintf("gsInt(%d)", int(g)) }

// panicS has a String method that panics.
type panicS struct{}

func (panicS) String() string { panic("boom S") }

// panicE has an Error method that panics with an error.
type panicE struct{}

func (panicE) Error() string { panic(errors.New("boom E")) }

// panicF has a Format method that panics with a value of a program type.
type panicF struct{}

func (panicF) Format(fmt.State, rune) { panic(sv{9}) }

// panicG has a GoString method that panics.
type panicG struct{}

func (panicG) GoString() string { panic("boom G") }

// plain has no methods.
type plain struct {
	A int
	b string
}

// holder has fields of interface and concrete types, exported and not.
type holder struct {
	X any
	x any
	S sv
	s sv
	P *sp
}

// nested has a holder and a slice of values.
type nested struct {
	H  holder
	L  []any
	M  map[string]any
	in fmt.Stringer
}

func main() {
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert x.(fmt.Stringer)
	probe("Stringer", func() {
		ch := make(pipe, 3)
		vals := []any{
			sv{1}, &sv{2}, &sp{3}, sp{4}, (*sp)(nil), gstr[int]{5}, gstr[string]{"g"}, &gstr[bool]{true},
			color(1), word("hi"), list{1, 2}, dict{"a": 1}, action(func() int { return 7 }), ch,
		}
		verbs(vals, "%v", "%+v", "%s", "%q", "%x", "%X", "%10v|", "%-8s|")
		verbs([]any{sv{1}, color(2), word("hi"), list{1, 2}, (*sp)(nil)}, "%d", "%#v", "%T")
		var s fmt.Stringer = &sp{6}
		each("%v %s %d", sv{1}, s, color(0))
		each("%v|%v", gstr[float64]{1.5}, word("w"))
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert x.(error)
	probe("Error", func() {
		vals := []any{ev{"a"}, &ev{"b"}, &ep{"c"}, ep{"d"}, gerr[int]{1}, errStr{}, code(42), wrapE{ev{"in"}}}
		verbs(vals, "%v", "%+v", "%s", "%q", "%x", "%d", "%#v", "%T")
		var e error = &ep{"iface"}
		each("%v %s %w", ev{"x"}, e, code(3))
		// %w with program errors, value and pointer receivers
		for _, w := range []error{ev{"w"}, &ep{"w"}, gerr[string]{"w"}, code(1), wrapE{&ep{"deep"}}} {
			err := fmt.Errorf("ctx: %w", w)
			u := errors.Unwrap(err)
			out("Errorf %%w: %v | unwrap %T %v | is %t", err, u, u, errors.Is(err, w))
			var target *ep
			out("as *ep: %t", errors.As(err, &target))
		}
		err := fmt.Errorf("two: %w and %w", ev{"1"}, code(2))
		out("Errorf two %%w: %v %T", err, errors.Unwrap(err))
		if j, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range j.Unwrap() {
				out("  wrapped %T %v", e, e)
			}
		}
		// %w with a value that is not an error, and %w outside Errorf
		wf := strings.Repeat("x %w", 1) // not a constant: vet does not check it
		out("Errorf %%w non-error: %v", fmt.Errorf(wf, sv{1}))
		out("Sprintf %%w: %s", fmt.Sprintf(wf, ev{"s"}))
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert x.(fmt.Formatter)
	probe("Formatter", func() {
		vals := []any{fv{1}, &fv{2}, &fp{3}, fp{4}, gfmt[int]{5}, &gfmt[string]{"p"}, fmtErr{}}
		verbs(vals, "%v", "%+v", "%#v", "%s", "%d", "%x", "%q", "%T")
		verbs([]any{fv{7}}, "%+-8.3v", "% 05d", "%#x", "%6.2v", "%-3x|")
		verbs([]any{&fp{8}}, "%+08.2f", "%-5d|", "%#o")
		var f fmt.Formatter = fv{9}
		each("%v %d %w", f, &fp{10}, fmtErr{})
		err := fmt.Errorf("w: %w", fmtErr{})
		out("Errorf %%w Formatter: %v %T", err, errors.Unwrap(err))
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert x.(fmt.GoStringer)
	probe("GoStringer", func() {
		vals := []any{gsv{1}, &gsv{2}, &gsp{3}, gsp{4}, ggs[int]{5}, ggs[string]{"s"}, gsInt(6), fmtErr{}}
		verbs(vals, "%#v", "%v", "%+v", "%T")
		verbs([]any{
			[]any{gsv{1}, gsp{2}, gsInt(3)},
			[]gsv{{4}, {5}},
			map[string]any{"a": gsv{6}, "b": gsInt(7)},
			map[gsInt]gsv{1: {8}},
			struct {
				G gsv
				g gsv
			}{gsv{9}, gsv{10}},
			[2]any{ggs[int]{11}, nil},
		}, "%#v", "%v")
		var g fmt.GoStringer = gsInt(12)
		each("%#v %#v %v", g, gsv{13}, gsv{14})
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert *
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect kind
	probe("PanicMethods", func() {
		vals := []any{panicS{}, panicE{}, panicF{}, panicG{}, (*spn)(nil), (*sv)(nil), (*ev)(nil), (*fv)(nil), (*gsv)(nil), &spn{1}}
		verbs(vals, "%v", "%s", "%d", "%#v", "%+q", "%T")
		verbs([]any{[]any{panicS{}, (*spn)(nil)}, map[string]any{"k": panicE{}}, holder{X: panicF{}}}, "%v", "%#v")
		each("%v %s %#v", panicS{}, (*sv)(nil), panicG{})
	})
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln assert *
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect walk
	// row: fmt Append,Appendf,Appendln,Errorf,Fprint,Fprintf,Fprintln,Print,Printf,Println,Sprint,Sprintf,Sprintln reflect length
	probe("Containers", func() {
		vals := []any{
			[]any{sv{1}, sp{2}, ev{"e"}, fv{3}, gsv{4}, color(1), plain{1, "p"}, nil},
			[3]any{gstr[int]{5}, code(6), word("w")},
			map[string]any{"s": sv{9}, "e": ep{"x"}, "f": fv{10}, "p": plain{2, "q"}},
			map[any]any{sv{2}: "two", sv{1}: color(0)},
			map[color]sv{2: {1}, 0: {2}, 1: {3}},
			map[word]error{"b": ev{"1"}, "a": code(2)},
			holder{X: sv{11}, x: sv{12}, S: sv{13}, s: sv{14}, P: nil},
			&holder{X: ev{"X"}, x: ev{"x"}, P: nil},
			nested{
				H:  holder{X: fv{16}, x: fv{17}},
				L:  []any{[]any{sv{18}, map[string]any{"in": gsv{19}}}, holder{X: color(2)}},
				M:  map[string]any{"z": []error{code(20)}, "a": [1]any{word("x")}},
				in: sv{21},
			},
			struct {
				E error
				e error
			}{ev{"E"}, ev{"e"}},
			struct {
				A any
				a any
			}{sv{22}, ev{"a"}},
			[]struct{ e error }{{ev{"x"}}},
		}
		verbs(vals, "%v", "%+v", "%#v", "%s", "%d", "%T")
		each("%v %v", []any{sv{1}, ev{"a"}}, map[string]any{"k": fv{2}})
	})
	probe("Containers/Stringers", func() {
		verbs([]any{[]fmt.Stringer{sv{7}, gstr[int]{8}, color(2)}}, "%v", "%+v", "%#v", "%s", "%d", "%T")
	})
}
