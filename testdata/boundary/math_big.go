// Native boundary of math/big: program types given to math/big (docs/BOUNDARY.md).
package main

import (
	"fmt"
	"math/big"
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

// state is a fmt.State with Write only.
type state struct {
	sb    strings.Builder
	calls []string
	width int
	flags string
}

func (s *state) Write(p []byte) (int, error) {
	s.calls = append(s.calls, "Write")
	return s.sb.Write(p)
}
func (s *state) Width() (int, bool)     { return s.width, s.width > 0 }
func (s *state) Precision() (int, bool) { return 0, false }
func (s *state) Flag(c int) bool        { return strings.ContainsRune(s.flags, rune(c)) }

// byteState is a fmt.State with WriteByte.
type byteState struct{ state }

func (s *byteState) WriteByte(c byte) error {
	s.calls = append(s.calls, "WriteByte")
	return s.sb.WriteByte(c)
}

// strState is a fmt.State with WriteString.
type strState struct{ state }

func (s *strState) WriteString(str string) (int, error) {
	s.calls = append(s.calls, "WriteString")
	return s.sb.WriteString(str)
}

// genState is a generic fmt.State with WriteByte and WriteString.
type genState[T any] struct {
	state
	tag T
}

func (s *genState[T]) WriteByte(c byte) error {
	s.calls = append(s.calls, "gWriteByte")
	return s.sb.WriteByte(c)
}
func (s *genState[T]) WriteString(str string) (int, error) {
	s.calls = append(s.calls, "gWriteString")
	return s.sb.WriteString(str)
}

// formatter is the Format method of *big.Int and *big.Float.
type formatter interface{ Format(fmt.State, rune) }

// run formats x with each state kind and prints the text and whether the
// optional methods were used.
func run(name string, x formatter, verb rune, width int, flags string) {
	st := &state{width: width, flags: flags}
	bs := &byteState{state{width: width, flags: flags}}
	ss := &strState{state{width: width, flags: flags}}
	gs := &genState[int]{state{width: width, flags: flags}, 1}
	x.Format(st, verb)
	x.Format(bs, verb)
	x.Format(ss, verb)
	x.Format(gs, verb)
	has := func(calls []string, m string) bool {
		for _, c := range calls {
			if c == m {
				return true
			}
		}
		return false
	}
	out("%s plain: %q", name, st.sb.String())
	out("%s byte: %q WriteByte %v", name, bs.sb.String(), has(bs.calls, "WriteByte"))
	out("%s string: %q WriteString %v", name, ss.sb.String(), has(ss.calls, "WriteString"))
	out("%s generic: %q", name, gs.sb.String())
}

func main() {
	// row: math/big (*Int).Format assert x.(io.ByteWriter)
	// row: math/big (*Int).Format assert x.(io.StringWriter)
	probe("Int.Format", func() {
		x := big.NewInt(-1234)
		run("d8", x, 'd', 8, "")
		run("d8-", x, 'd', 8, "-")
		run("d8 0", x, 'd', 8, "0")
		run("x10#", x, 'x', 10, "#")
		run("d", x, 'd', 0, "")
		out("Sprintf: %q", fmt.Sprintf("[%8d|%-8x]", x, x))
	})
	// row: math/big (*Float).Format assert x.(io.ByteWriter)
	// row: math/big (*Float).Format assert x.(io.StringWriter)
	probe("Float.Format", func() {
		f := big.NewFloat(3.25)
		run("g10", f, 'g', 10, "")
		run("f10-", f, 'f', 10, "-")
		run("e12 0", f, 'e', 12, "0")
		run("g", f, 'g', 0, "")
		out("Sprintf: %q", fmt.Sprintf("[%10g|%-10.2f]", f, f))
	})
}
