// Native boundary: the reproducers of the issues that the boundary
// inventory fixed. The probe ids are the old issue numbers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"strconv"
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

// outWriter writes the lines of the current probe.
type outWriter struct{}

func (outWriter) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimSuffix(string(p), "\n"), "\n") {
		out("%s", l)
	}
	return len(p), nil
}

type Point struct{ X, Y int }

type G struct{ V int }

func (g G) GoString() string { return "G!" }

type counter struct{ n int }

func (c counter) Value() int { return c.n }

func f(c counter, p *counter) []counter { return nil }

type Emb struct {
	fmt.Stringer
	N int
}

type M struct{ A int }

func (m M) Format(f fmt.State, verb rune) { fmt.Fprintf(f, "M<%c %d>", verb, m.A) }

type src struct{}

func (src) Read(p []byte) (int, error) { out("Read called"); return 0, io.EOF }
func (src) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "from WriteTo")
	return int64(n), err
}

type dst struct{ sb strings.Builder }

func (d *dst) Write(p []byte) (int, error) { return d.sb.Write(p) }
func (d *dst) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	d.sb.WriteString("ReadFrom:" + string(b))
	return int64(len(b)), err
}

type Opt[T any] struct {
	Set bool
	V   T
}

func (o Opt[T]) MarshalJSON() ([]byte, error) { return json.Marshal(o.V) }

type Pair[V any] struct{ V V }

type BoolCounter int

func (b *BoolCounter) String() string   { return strconv.Itoa(int(*b)) }
func (b *BoolCounter) Set(string) error { *b++; return nil }
func (b *BoolCounter) IsBoolFlag() bool { return true }

type Size int64

func (s *Size) UnmarshalText(b []byte) error {
	n, err := strconv.ParseInt(string(b), 10, 64)
	*s = Size(n)
	return err
}

func (s Size) MarshalText() ([]byte, error) { return []byte(strconv.FormatInt(int64(s), 10)), nil }

func main() {
	probe("178", func() {
		out("%#v", []any{Point{1, 2}})
		out("%#v %#v", [1]any{G{1}}, map[string]any{"a": G{2}})
		out("%T", []any{Point{}}[0])
	})
	probe("179", func() {
		var g func(counter)
		out("%T | %T | %T", counter.Value, f, g)
	})
	probe("180", func() {
		out("%v", Emb{})
		out("%+v", Emb{N: 1})
	})
	probe("183", func() {
		lg := log.New(outWriter{}, "", 0)
		lg.Print(M{1})
		lg.Println(M{2}, "x")
		lg.Printf("%v|%d", M{3}, 4)
	})
	probe("191", func() {
		var sb strings.Builder
		n, err := io.Copy(&sb, src{})
		out("%d %v %q", n, err, sb.String())
		d := &dst{}
		n, err = io.Copy(d, io.LimitReader(strings.NewReader("xyz"), 2))
		out("%d %v %q", n, err, d.sb.String())
	})
	probe("192", func() {
		var a any = Pair[Opt[int]]{Opt[int]{true, 1}}
		b, err := json.Marshal(&a)
		out("%s %v", b, err)
	})
	probe("193", func() {
		fs := flag.NewFlagSet("p", flag.ContinueOnError)
		fs.SetOutput(outWriter{})
		var c BoolCounter
		var size Size
		fs.Var(&c, "v", "verbose")
		fs.TextVar(&size, "size", Size(5), "a size")
		out("%v %d %d %v", fs.Parse([]string{"-v", "-v", "-size", "12", "rest"}), c, size, fs.Args())
		fs.PrintDefaults()
	})
}
