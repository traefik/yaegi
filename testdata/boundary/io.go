// Native boundary of io: program types given to io (docs/BOUNDARY.md).
package main

import (
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

func (r src) Read(p []byte) (int, error) { out("src.Read"); return 0, io.EOF }
func (r src) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "WriteTo:"+r.s)
	return int64(n), err
}

// psrc is a reader with a WriteTo method (pointer receiver).
type psrc struct{ s string }

func (r *psrc) Read(p []byte) (int, error) { out("psrc.Read"); return 0, io.EOF }
func (r *psrc) WriteTo(w io.Writer) (int64, error) {
	n, err := io.WriteString(w, "pWriteTo:"+r.s)
	return int64(n), err
}

// gsrc is a generic reader with a WriteTo method.
type gsrc[T any] struct{ v T }

func (r gsrc[T]) Read(p []byte) (int, error) { out("gsrc.Read"); return 0, io.EOF }
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

// dst is a writer with a ReadFrom method (pointer receiver).
type dst struct{ sb strings.Builder }

func (d *dst) Write(p []byte) (int, error) { d.sb.WriteString("[Write]"); return d.sb.Write(p) }
func (d *dst) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	d.sb.WriteString("ReadFrom:" + string(b))
	return int64(len(b)), err
}

// gdst is a generic writer with a ReadFrom method.
type gdst[T any] struct {
	sb  *strings.Builder
	tag T
}

func (d gdst[T]) Write(p []byte) (int, error) { d.sb.WriteString("[gWrite]"); return d.sb.Write(p) }
func (d gdst[T]) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	fmt.Fprintf(d.sb, "gReadFrom(%v):%s", d.tag, b)
	return int64(len(b)), err
}

// plainDst is a writer without ReadFrom.
type plainDst struct{ sb strings.Builder }

func (d *plainDst) Write(p []byte) (int, error) { d.sb.WriteString("[Write]"); return d.sb.Write(p) }

// sw is a writer with WriteString.
type sw struct{ sb strings.Builder }

func (w *sw) Write(p []byte) (int, error) { w.sb.WriteString("[Write]"); return w.sb.Write(p) }
func (w *sw) WriteString(s string) (int, error) {
	w.sb.WriteString("[WriteString]")
	return w.sb.WriteString(s)
}

func main() {
	// row: io Copy,CopyBuffer,CopyN assert x.(io.WriterTo)
	probe("Copy/WriterTo", func() {
		srcs := []io.Reader{src{"a"}, &psrc{"b"}, gsrc[int]{7}, &plainSrc{"plain"}}
		for i, r := range srcs {
			var sb strings.Builder
			n, err := io.Copy(&sb, r)
			out("Copy %d: %d %v %q", i, n, err, sb.String())
			sb.Reset()
			n, err = io.CopyBuffer(&sb, r, make([]byte, 4))
			out("CopyBuffer %d: %d %v %q", i, n, err, sb.String())
		}
		var sb strings.Builder
		n, err := io.CopyN(&sb, src{"cdef"}, 3)
		out("CopyN: %d %v %q", n, err, sb.String())
		var r io.Reader = &psrc{"iface"}
		sb.Reset()
		n, err = io.Copy(&sb, r)
		out("Copy iface: %d %v %q", n, err, sb.String())
	})
	// row: io Copy,CopyBuffer,CopyN assert x.(io.ReaderFrom)
	probe("Copy/ReaderFrom", func() {
		d := &dst{}
		n, err := io.Copy(d, strings.NewReader("abc"))
		out("Copy dst: %d %v %q", n, err, d.sb.String())
		d = &dst{}
		n, err = io.CopyBuffer(d, &plainSrc{"xyz"}, make([]byte, 2))
		out("CopyBuffer dst: %d %v %q", n, err, d.sb.String())
		d = &dst{}
		n, err = io.CopyN(d, strings.NewReader("12345"), 2)
		out("CopyN dst: %d %v %q", n, err, d.sb.String())
		g := gdst[string]{&strings.Builder{}, "t"}
		n, err = io.Copy(g, &plainSrc{"gen"})
		out("Copy gdst: %d %v %q", n, err, g.sb.String())
		p := &plainDst{}
		n, err = io.Copy(p, &plainSrc{"plain"})
		out("Copy plainDst: %d %v %q", n, err, p.sb.String())
		// WriterTo of the source wins over ReadFrom of the destination
		d = &dst{}
		n, err = io.Copy(d, src{"both"})
		out("Copy both: %d %v %q", n, err, d.sb.String())
		var w io.Writer = &dst{}
		n, err = io.Copy(w, &plainSrc{"iface"})
		out("Copy iface: %d %v %q", n, err, w.(*dst).sb.String())
	})
	// row: io NopCloser assert x.(io.WriterTo)
	probe("NopCloser", func() {
		for i, r := range []io.Reader{src{"a"}, &psrc{"b"}, gsrc[string]{"g"}, &plainSrc{"p"}} {
			rc := io.NopCloser(r)
			_, ok := rc.(io.WriterTo)
			var sb strings.Builder
			n, err := io.Copy(&sb, rc)
			out("%d: WriterTo %v, copy %d %v %q, close %v", i, ok, n, err, sb.String(), rc.Close())
		}
	})
	// row: io WriteString assert x.(io.StringWriter)
	probe("WriteString", func() {
		w := &sw{}
		n, err := io.WriteString(w, "hi")
		out("sw: %d %v %q", n, err, w.sb.String())
		p := &plainDst{}
		n, err = io.WriteString(p, "hi")
		out("plain: %d %v %q", n, err, p.sb.String())
		var iw io.Writer = &sw{}
		io.WriteString(iw, "x")
		out("iface: %q", iw.(*sw).sb.String())
	})
}
