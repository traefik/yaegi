// Native boundary of fmt (scan functions): program Scanner types, program
// readers and named destinations given to the scan functions of fmt
// (docs/BOUNDARY.md).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

var tmpDir string

// withStdin runs f with os.Stdin reading the given text from a file.
func withStdin(text string, f func()) {
	name := filepath.Join(tmpDir, "stdin")
	if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
		panic(err)
	}
	file, err := os.Open(name)
	if err != nil {
		panic(err)
	}
	old := os.Stdin
	os.Stdin = file
	defer func() {
		os.Stdin = old
		file.Close()
	}()
	f()
}

// all calls the nine scan functions on the same input. mk makes fresh
// destinations, show prints them after the scan.
func all(input, format string, mk func() []any, show func([]any) string) {
	type call struct {
		name string
		f    func(args []any) (int, error)
	}
	calls := []call{
		{"Sscan", func(a []any) (int, error) { return fmt.Sscan(input, a...) }},
		{"Sscanln", func(a []any) (int, error) { return fmt.Sscanln(input, a...) }},
		{"Sscanf", func(a []any) (int, error) { return fmt.Sscanf(input, format, a...) }},
		{"Fscan", func(a []any) (int, error) { return fmt.Fscan(strings.NewReader(input), a...) }},
		{"Fscanln", func(a []any) (int, error) { return fmt.Fscanln(strings.NewReader(input), a...) }},
		{"Fscanf", func(a []any) (int, error) { return fmt.Fscanf(strings.NewReader(input), format, a...) }},
		{"Scan", func(a []any) (n int, err error) {
			withStdin(input, func() { n, err = fmt.Scan(a...) })
			return
		}},
		{"Scanln", func(a []any) (n int, err error) {
			withStdin(input, func() { n, err = fmt.Scanln(a...) })
			return
		}},
		{"Scanf", func(a []any) (n int, err error) {
			withStdin(input, func() { n, err = fmt.Scanf(format, a...) })
			return
		}},
	}
	for _, c := range calls {
		args := mk()
		n, err := c.f(args)
		out("%s %q: n=%d err=%v %T | %s", c.name, input, n, err, err, show(args))
	}
}

// tok is a Scanner (pointer receiver) that reads one token.
type tok struct {
	s    string
	verb rune
}

func (t *tok) Scan(st fmt.ScanState, verb rune) error {
	b, err := st.Token(true, nil)
	t.s, t.verb = string(b), verb
	return err
}

// vsc is a Scanner with a value receiver: it writes through a pointer.
type vsc struct{ p *string }

func (v vsc) Scan(st fmt.ScanState, verb rune) error {
	b, err := st.Token(true, func(r rune) bool { return r != ' ' && r != '\n' })
	*v.p = "vsc:" + string(b)
	return err
}

// gsc is a generic Scanner.
type gsc[T any] struct {
	got string
	tag T
}

func (g *gsc[T]) Scan(st fmt.ScanState, verb rune) error {
	b, err := st.Token(true, nil)
	g.got = fmt.Sprintf("%v:%s:%c", g.tag, b, verb)
	return err
}

// runeSc reads runes one by one, with SkipSpace, Width and UnreadRune.
type runeSc struct{ got []string }

func (r *runeSc) Scan(st fmt.ScanState, verb rune) error {
	st.SkipSpace()
	w, ok := st.Width()
	if !ok {
		w = 2
	}
	for i := 0; i < w; i++ {
		c, size, err := st.ReadRune()
		if err != nil {
			r.got = append(r.got, "err:"+err.Error())
			return nil
		}
		if c == ' ' || c == '\n' {
			st.UnreadRune()
			break
		}
		r.got = append(r.got, fmt.Sprintf("%c/%d", c, size))
	}
	return nil
}

// scanErr is an error of the program.
type scanErr struct{ verb rune }

func (e scanErr) Error() string { return fmt.Sprintf("scanErr(%c)", e.verb) }

// errSc is a Scanner that fails with a program error.
type errSc struct{}

func (errSc) Scan(st fmt.ScanState, verb rune) error { return scanErr{verb} }

// hexInt is a named int whose Scan method scans again with fmt.
type hexInt int

func (h *hexInt) Scan(st fmt.ScanState, verb rune) error {
	var v int
	_, err := fmt.Fscanf(st, "%x", &v)
	*h = hexInt(v)
	return err
}

// panicSc is a Scanner that panics with a program error.
type panicSc struct{}

func (panicSc) Scan(st fmt.ScanState, verb rune) error { panic(scanErr{verb}) }

// Named basic types used as destinations.
type (
	B    bool
	C64  complex64
	C128 complex128
	F32  float32
	F64  float64
	I    int
	I8   int8
	I16  int16
	I32  int32
	I64  int64
	S    string
	U    uint
	U8   uint8
	U16  uint16
	U32  uint32
	U64  uint64
	Uptr uintptr
	Bs   []byte
)

type pt struct{ X, Y int }

// plainR is a reader without ReadRune.
type plainR struct {
	s     string
	reads int
}

func (r *plainR) Read(p []byte) (int, error) {
	r.reads++
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

// runeR is a reader with ReadRune and UnreadRune (an io.RuneScanner).
type runeR struct {
	s              []rune
	i              int
	reads, unreads int
}

func (r *runeR) Read(p []byte) (int, error) { return 0, errors.New("runeR.Read called") }
func (r *runeR) ReadRune() (rune, int, error) {
	r.reads++
	if r.i >= len(r.s) {
		return 0, 0, io.EOF
	}
	c := r.s[r.i]
	r.i++
	return c, len(string(c)), nil
}
func (r *runeR) UnreadRune() error {
	r.unreads++
	if r.i == 0 {
		return errors.New("runeR: nothing to unread")
	}
	r.i--
	return nil
}

// onlyRR has ReadRune but no UnreadRune: not an io.RuneScanner.
type onlyRR struct {
	plainR
	runes int
}

func (r *onlyRR) ReadRune() (rune, int, error) { r.runes++; return 0, 0, io.EOF }

func main() {
	var err error
	tmpDir, err = os.MkdirTemp("", "boundary-fmt")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln assert x.(fmt.Scanner)
	probe("Scanner", func() {
		all("abc 12 xyz\n", "%v %d %s", func() []any {
			var s string
			return []any{&tok{}, vsc{&s}, &gsc[int]{tag: 7}}
		}, func(a []any) string {
			return fmt.Sprintf("%+v %q %+v", *a[0].(*tok), *a[1].(vsc).p, *a[2].(*gsc[int]))
		})
		all("héllo w\n", "%3v %v", func() []any { return []any{&runeSc{}, &runeSc{}} }, func(a []any) string {
			return fmt.Sprintf("%v %v", a[0].(*runeSc).got, a[1].(*runeSc).got)
		})
		all("ff 10\n", "%x %v", func() []any { return []any{new(hexInt), new(hexInt)} }, func(a []any) string {
			return fmt.Sprintf("%d %d", *a[0].(*hexInt), *a[1].(*hexInt))
		})
		all("x\n", "%q", func() []any { return []any{errSc{}} }, func(a []any) string { return "-" })
		all("x\n", "%v", func() []any { return []any{panicSc{}} }, func(a []any) string { return "-" })
		var sc fmt.Scanner = &tok{}
		n, err := fmt.Sscan("iface", sc)
		out("Sscan iface: %d %v %+v", n, err, *sc.(*tok))
	})
	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln assert x.(io.RuneScanner)
	probe("RuneScanner", func() {
		// Fscan* with a program reader: with and without UnreadRune.
		for _, mk := range []func() io.Reader{
			func() io.Reader { return &plainR{s: "12 ab\n34 cd\n"} },
			func() io.Reader { return &runeR{s: []rune("12 ab\n34 cd\n")} },
			func() io.Reader { return &onlyRR{plainR: plainR{s: "12 ab\n34 cd\n"}} },
		} {
			for _, name := range []string{"Fscan", "Fscanln", "Fscanf"} {
				r := mk()
				var a, c I
				var b, d S
				var n1, n2 int
				var e1, e2 error
				switch name {
				case "Fscan":
					n1, e1 = fmt.Fscan(r, &a, &b)
					n2, e2 = fmt.Fscan(r, &c, &d)
				case "Fscanln":
					n1, e1 = fmt.Fscanln(r, &a, &b)
					n2, e2 = fmt.Fscanln(r, &c, &d)
				case "Fscanf":
					n1, e1 = fmt.Fscanf(r, "%d %s\n", &a, &b)
					n2, e2 = fmt.Fscanf(r, "%d %s\n", &c, &d)
				}
				extra := ""
				switch rr := r.(type) {
				case *plainR:
					extra = fmt.Sprintf("plainR reads>0=%t", rr.reads > 0)
				case *runeR:
					extra = fmt.Sprintf("runeR reads=%d unreads=%d", rr.reads, rr.unreads)
				case *onlyRR:
					extra = fmt.Sprintf("onlyRR reads>0=%t runes=%d", rr.reads > 0, rr.runes)
				}
				out("%T %s: %d %v %d %v | %d %q %d %q | %s", r, name, n1, e1, n2, e2, a, b, c, d, extra)
			}
		}
		// Scan* (os.Stdin) and Sscan* (fmt's own reader): the reader is not
		// a program type.
		all("5 six\n", "%d %s", func() []any { return []any{new(I), new(S)} }, func(a []any) string {
			return fmt.Sprintf("%d %q", *a[0].(*I), *a[1].(*S))
		})
	})
	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln assert x.(T), T unnamed
	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln reflect kind
	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln reflect type name
	// row: fmt Fscan,Fscanf,Fscanln,Scan,Scanf,Scanln,Sscan,Sscanf,Sscanln reflect walk
	probe("Named", func() {
		mk := func() []any {
			return []any{new(B), new(C64), new(C128), new(F32), new(F64), new(I), new(I8), new(I16), new(I32), new(I64),
				new(S), new(U), new(U8), new(U16), new(U32), new(U64), new(Uptr), new(Bs)}
		}
		show := func(a []any) string {
			var parts []string
			for _, p := range a {
				parts = append(parts, fmt.Sprintf("%T=%v", p, fmt.Sprint(deref(p))))
			}
			return strings.Join(parts, " ")
		}
		in := "true (1+2i) -3i 1.5 2.25 -6 -8 16 32 64 str 7 200 65535 32 64 4660 bytes\n"
		all(in, "%t %v %g %f %e %d %d %d %d %d %s %d %d %d %d %d %v %s", mk, show)
		all("0x1f 0b101 0o17 'x' \"q\" 6162\n", "%v %v %v %c %q %x", func() []any {
			return []any{new(I), new(U8), new(I16), new(I32), new(S), new(Bs)}
		}, show)
		// errors that name the type
		all("300\n", "%d", func() []any { return []any{new(I8)} }, show)
		all("x\n", "%d", func() []any { return []any{new(I)} }, show)
		all("1 2\n", "%d %d", func() []any { return []any{new(pt), new([]I)} }, show)
		all("1\n", "%d", func() []any { return []any{I(1)} }, func([]any) string { return "-" })
		all("1\n", "%d", func() []any { return []any{new(*I)} }, func([]any) string { return "-" })
		var iface any = new(F64)
		n, err := fmt.Sscan("9.5", iface)
		out("Sscan iface: %d %v %v", n, err, *iface.(*F64))
	})
}

// deref returns what a pointer points to.
func deref(p any) any {
	switch p := p.(type) {
	case *B:
		return *p
	case *C64:
		return *p
	case *C128:
		return *p
	case *F32:
		return *p
	case *F64:
		return *p
	case *I:
		return *p
	case *I8:
		return *p
	case *I16:
		return *p
	case *I32:
		return *p
	case *I64:
		return *p
	case *S:
		return *p
	case *U:
		return *p
	case *U8:
		return *p
	case *U16:
		return *p
	case *U32:
		return *p
	case *U64:
		return *p
	case *Uptr:
		return *p
	case *Bs:
		return string(*p)
	case *pt:
		return *p
	case *[]I:
		return *p
	}
	return "?"
}
