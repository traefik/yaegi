// Native boundary of bufio: program types given to bufio (docs/BOUNDARY.md).
package main

import (
	"bufio"
	"fmt"
	"io"
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

// src is a reader with a WriteTo method (value receiver).
type src struct{ s string }

func (r src) Read(p []byte) (int, error) { return 0, io.EOF }
func (r src) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "WriteTo:"+r.s)
	return int64(n), err
}

// psrc is a reader with a WriteTo method (pointer receiver).
type psrc struct{ s string }

func (r *psrc) Read(p []byte) (int, error) { return 0, io.EOF }
func (r *psrc) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "pWriteTo:"+r.s)
	return int64(n), err
}

// gsrc is a generic reader with a WriteTo method.
type gsrc[T any] struct{ v T }

func (r gsrc[T]) Read(p []byte) (int, error) { return 0, io.EOF }
func (r gsrc[T]) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "gWriteTo:%v", r.v)
	return int64(n), err
}

// plainSrc is a reader without WriteTo.
type plainSrc struct{ s string }

func (r *plainSrc) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

// sliceSrc is a reader of an uncomparable type.
type sliceSrc []byte

func (r sliceSrc) Read(p []byte) (int, error) { return 0, io.EOF }

// dst is a writer with ReadFrom and WriteString (pointer receivers).
type dst struct{ sb strings.Builder }

func (d *dst) Write(p []byte) (int, error) { d.sb.WriteString("[W]"); return d.sb.Write(p) }
func (d *dst) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	d.sb.WriteString("[RF]" + string(b))
	return int64(len(b)), err
}
func (d *dst) WriteString(s string) (int, error) {
	d.sb.WriteString("[WS]")
	return d.sb.WriteString(s)
}

// gdst is a generic writer with ReadFrom and WriteString.
type gdst[T any] struct {
	sb  *strings.Builder
	tag T
}

func (d gdst[T]) Write(p []byte) (int, error) { d.sb.WriteString("[gW]"); return d.sb.Write(p) }
func (d gdst[T]) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	fmt.Fprintf(d.sb, "[gRF %v]%s", d.tag, b)
	return int64(len(b)), err
}
func (d gdst[T]) WriteString(s string) (int, error) {
	d.sb.WriteString("[gWS]")
	return d.sb.WriteString(s)
}

// plainDst is a writer without ReadFrom and WriteString.
type plainDst struct{ sb strings.Builder }

func (d *plainDst) Write(p []byte) (int, error) { d.sb.WriteString("[W]"); return d.sb.Write(p) }

// sliceDst is a writer of an uncomparable type.
type sliceDst []string

func (d sliceDst) Write(p []byte) (int, error) { return len(p), nil }

type texter interface{ String() string }

func (d *dst) String() string      { return d.sb.String() }
func (d *plainDst) String() string { return d.sb.String() }

func main() {
	// row: bufio (*Reader).WriteTo,(*ReadWriter).WriteTo assert x.(io.WriterTo)
	probe("Reader.WriteTo/WriterTo", func() {
		for i, r := range []io.Reader{src{"a"}, &psrc{"b"}, gsrc[int]{3}, &plainSrc{"plain"}} {
			var sb strings.Builder
			n, err := bufio.NewReader(r).WriteTo(&sb)
			out("%d Reader: %d %v %q", i, n, err, sb.String())
			sb.Reset()
			rw := bufio.NewReadWriter(bufio.NewReader(r), bufio.NewWriter(&sb))
			n, err = rw.WriteTo(&sb)
			out("%d ReadWriter: %d %v %q", i, n, err, sb.String())
		}
		// buffered data first, then WriteTo
		br := bufio.NewReader(io.MultiReader(strings.NewReader("xy"), src{"z"}))
		b, _ := br.ReadByte()
		var sb strings.Builder
		n, err := br.WriteTo(&sb)
		out("buffered: %c %d %v %q", b, n, err, sb.String())
	})
	// row: bufio (*Reader).WriteTo,(*ReadWriter).WriteTo assert x.(io.ReaderFrom)
	probe("Reader.WriteTo/ReaderFrom", func() {
		ws := []texter{&dst{}, &plainDst{}}
		for i, w := range ws {
			n, err := bufio.NewReader(&plainSrc{"data"}).WriteTo(w.(io.Writer))
			out("%d Reader: %d %v %q", i, n, err, w.String())
		}
		g := gdst[string]{&strings.Builder{}, "t"}
		n, err := bufio.NewReader(&plainSrc{"gen"}).WriteTo(g)
		out("gdst: %d %v %q", n, err, g.sb.String())
		d := &dst{}
		rw := bufio.NewReadWriter(bufio.NewReader(&plainSrc{"rw"}), bufio.NewWriter(io.Discard))
		n, err = rw.WriteTo(d)
		out("ReadWriter: %d %v %q", n, err, d.String())
	})
	// row: bufio (*Writer).ReadFrom,(*ReadWriter).ReadFrom assert x.(io.ReaderFrom)
	probe("Writer.ReadFrom", func() {
		ws := []texter{&dst{}, &plainDst{}}
		for i, w := range ws {
			bw := bufio.NewWriter(w.(io.Writer))
			n, err := bw.ReadFrom(&plainSrc{"data"})
			bw.Flush()
			out("%d Writer: %d %v %q", i, n, err, w.String())
			// with buffered data
			w2 := []texter{&dst{}, &plainDst{}}[i]
			bw = bufio.NewWriter(w2.(io.Writer))
			bw.WriteString("pre-")
			n, err = bw.ReadFrom(&plainSrc{"data"})
			bw.Flush()
			out("%d Writer buffered: %d %v %q", i, n, err, w2.String())
		}
		g := gdst[int]{&strings.Builder{}, 1}
		bw := bufio.NewWriter(g)
		n, err := bw.ReadFrom(&plainSrc{"gen"})
		bw.Flush()
		out("gdst: %d %v %q", n, err, g.sb.String())
		d := &dst{}
		rw := bufio.NewReadWriter(bufio.NewReader(&plainSrc{}), bufio.NewWriter(d))
		n, err = rw.ReadFrom(&plainSrc{"rw"})
		rw.Flush()
		out("ReadWriter: %d %v %q", n, err, d.String())
	})
	// row: bufio (*Writer).WriteString,(*Writer).WriteRune,(*ReadWriter).WriteString,(*ReadWriter).WriteRune assert x.(io.StringWriter)
	probe("Writer.WriteString", func() {
		long := strings.Repeat("x", 20)
		ws := []texter{&dst{}, &plainDst{}}
		for i, w := range ws {
			bw := bufio.NewWriterSize(w.(io.Writer), 16)
			n, err := bw.WriteString(long)
			bw.Flush()
			out("%d WriteString: %d %v %q", i, n, err, w.String())
			w2 := []texter{&dst{}, &plainDst{}}[i]
			bw = bufio.NewWriterSize(w2.(io.Writer), 2)
			n, err = bw.WriteRune('é')
			n2, err2 := bw.WriteRune('€')
			bw.Flush()
			out("%d WriteRune: %d %v %d %v %q", i, n, err, n2, err2, w2.String())
		}
		g := gdst[int]{&strings.Builder{}, 2}
		bw := bufio.NewWriterSize(g, 16)
		bw.WriteString(long)
		bw.Flush()
		out("gdst: %q", g.sb.String())
		d := &dst{}
		rw := bufio.NewReadWriter(bufio.NewReader(&plainSrc{}), bufio.NewWriterSize(d, 2))
		rw.WriteString("abcdef")
		rw.WriteRune('€')
		rw.Flush()
		out("ReadWriter: %q", d.String())
	})
	// row: bufio (*Reader).Reset panic == of two interfaces: comparing uncomparable type
	probe("Reader.Reset", func() {
		br := bufio.NewReader(&plainSrc{"one"})
		br.Reset(sliceSrc{1})
		b, err := br.ReadByte()
		out("slice reader: %d %v", b, err)
		br.Reset(&plainSrc{"two"})
		s, err := br.ReadString(0)
		out("plain reader: %q %v", s, err)
		br.Reset(br)
		out("self reset: %v", br.Buffered())
		br2 := bufio.NewReader(sliceSrc{2})
		out("NewReader slice: %v", br2.Size())
	})
	// row: bufio (*Writer).Reset panic == of two interfaces: comparing uncomparable type
	probe("Writer.Reset", func() {
		p := &plainDst{}
		bw := bufio.NewWriter(p)
		bw.WriteString("a")
		bw.Reset(sliceDst{"x"})
		bw.WriteString("b")
		out("slice writer: %v %q", bw.Flush(), p.String())
		bw.Reset(p)
		bw.WriteString("c")
		out("plain writer: %v %q", bw.Flush(), p.String())
		bw.Reset(bw)
		out("self reset: %v", bw.Buffered())
		out("NewWriter slice: %v", bufio.NewWriter(sliceDst{}).Size())
	})
}
