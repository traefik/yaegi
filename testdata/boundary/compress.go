// Native boundary of compress/bzip2, compress/flate, compress/gzip,
// compress/lzw and compress/zlib: program readers and writers given to
// these packages (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"compress/bzip2"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"compress/zlib"
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

// counts records which methods of a program reader were called.
type counts struct{ read, readByte bool }

func (c *counts) String() string { return fmt.Sprintf("Read=%v ReadByte=%v", c.read, c.readByte) }

// byteReader has Read and ReadByte (pointer receivers): it is an
// io.ByteReader and a flate.Reader.
type byteReader struct {
	r *bytes.Reader
	c counts
}

func (b *byteReader) Read(p []byte) (int, error) { b.c.read = true; return b.r.Read(p) }
func (b *byteReader) ReadByte() (byte, error)    { b.c.readByte = true; return b.r.ReadByte() }

// valByteReader has Read and ReadByte (value receivers, shared state).
type valByteReader struct {
	r *bytes.Reader
	c *counts
}

func (b valByteReader) Read(p []byte) (int, error) { b.c.read = true; return b.r.Read(p) }
func (b valByteReader) ReadByte() (byte, error)    { b.c.readByte = true; return b.r.ReadByte() }

// genByteReader is a generic reader with ReadByte.
type genByteReader[T any] struct {
	r   *bytes.Reader
	c   *counts
	tag T
}

func (b genByteReader[T]) Read(p []byte) (int, error) { b.c.read = true; return b.r.Read(p) }
func (b genByteReader[T]) ReadByte() (byte, error)    { b.c.readByte = true; return b.r.ReadByte() }

// plainReader has only Read.
type plainReader struct {
	r *bytes.Reader
	c counts
}

func (b *plainReader) Read(p []byte) (int, error) { b.c.read = true; return b.r.Read(p) }

// readers returns the program readers over data, with their counters.
func readers(data []byte) ([]io.Reader, []*counts) {
	br := &byteReader{r: bytes.NewReader(data)}
	vc := &counts{}
	gc := &counts{}
	pr := &plainReader{r: bytes.NewReader(data)}
	var ir io.Reader = &byteReader{r: bytes.NewReader(data)}
	return []io.Reader{br, valByteReader{bytes.NewReader(data), vc}, genByteReader[string]{bytes.NewReader(data), gc, "g"}, pr, ir},
		[]*counts{&br.c, vc, gc, &pr.c, &ir.(*byteReader).c}
}

// bzData is "hello bzip2 boundary\n" compressed by bzip2 -9.
var bzData = []byte{
	0x42, 0x5a, 0x68, 0x39, 0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0xf6, 0x76,
	0x85, 0xdb, 0x00, 0x00, 0x04, 0xd9, 0x80, 0x00, 0x10, 0x40, 0x00, 0x10,
	0x00, 0x36, 0x65, 0xd2, 0x30, 0x20, 0x00, 0x31, 0x43, 0x4d, 0x30, 0x00,
	0x50, 0x34, 0x68, 0x6d, 0x32, 0x87, 0x10, 0x1f, 0x54, 0x1e, 0x75, 0x12,
	0xad, 0x24, 0x85, 0x9f, 0x8b, 0xb9, 0x22, 0x9c, 0x28, 0x48, 0x7b, 0x3b,
	0x42, 0xed, 0x80,
}

func flateData(s string, dict []byte) []byte {
	var b bytes.Buffer
	w, _ := flate.NewWriterDict(&b, flate.BestCompression, dict)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func gzipData(s, name string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Name = name
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func zlibData(s string, dict []byte) []byte {
	var b bytes.Buffer
	w, _ := zlib.NewWriterLevelDict(&b, zlib.BestCompression, dict)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

// flushWriter has WriteByte and Flush (pointer receivers): it is the
// writer interface of compress/lzw.
type flushWriter struct {
	b       bytes.Buffer
	flushes int
	bytes   bool
}

func (w *flushWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *flushWriter) WriteByte(c byte) error      { w.bytes = true; return w.b.WriteByte(c) }
func (w *flushWriter) Flush() error                { w.flushes++; return nil }

// genFlushWriter is a generic writer with WriteByte and Flush.
type genFlushWriter[T any] struct{ w *flushWriter }

func (g genFlushWriter[T]) Write(p []byte) (int, error) { return g.w.Write(p) }
func (g genFlushWriter[T]) WriteByte(c byte) error      { return g.w.WriteByte(c) }
func (g genFlushWriter[T]) Flush() error                { return g.w.Flush() }

// byteOnlyWriter has WriteByte but no Flush.
type byteOnlyWriter struct {
	b     bytes.Buffer
	bytes bool
}

func (w *byteOnlyWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *byteOnlyWriter) WriteByte(c byte) error      { w.bytes = true; return w.b.WriteByte(c) }

const lzwText = "TOBEORNOTTOBEORTOBEORNOT"

func main() {
	// row: compress/bzip2 NewReader assert x.(io.ByteReader)
	probe("bzip2/NewReader", func() {
		rs, cs := readers(bzData)
		for i, r := range rs {
			b, err := io.ReadAll(bzip2.NewReader(r))
			out("%d: %q %v %v", i, b, err, cs[i])
		}
	})
	// row: compress/flate NewReader,NewReaderDict assert x.(compress/flate.Reader)
	probe("flate/NewReader", func() {
		data := flateData("flate flate flate data", nil)
		rs, cs := readers(data)
		for i, r := range rs {
			fr := flate.NewReader(r)
			b, err := io.ReadAll(fr)
			out("%d: %q %v %v", i, b, err, cs[i])
		}
		dict := []byte("dictionary words ")
		data = flateData("dictionary words again", dict)
		rs, cs = readers(data)
		for i, r := range rs {
			b, err := io.ReadAll(flate.NewReaderDict(r, dict))
			out("dict %d: %q %v %v", i, b, err, cs[i])
		}
	})
	// row: compress/gzip NewReader,(*Reader).Reset assert x.(compress/flate.Reader)
	probe("gzip/NewReader", func() {
		data := gzipData("gzip payload", "a.txt")
		rs, cs := readers(data)
		for i, r := range rs {
			z, err := gzip.NewReader(r)
			if err != nil {
				out("%d: NewReader %v", i, err)
				continue
			}
			b, err := io.ReadAll(z)
			out("%d: %q %q %v %v", i, z.Name, b, err, cs[i])
		}
		z, _ := gzip.NewReader(bytes.NewReader(data))
		rs, cs = readers(gzipData("second payload", "b.txt"))
		for i, r := range rs {
			err := z.Reset(r)
			b, err2 := io.ReadAll(z)
			out("Reset %d: %v %q %q %v %v", i, err, z.Name, b, err2, cs[i])
		}
	})
	// The decompressor of a gzip.Reader is always made by flate.NewReader,
	// so the x.(flate.Resetter) assertion cannot fail. The probe takes the
	// path: a stream of two gzip members (multistream) and Reset.
	// row: compress/gzip NewReader,(*Reader).Read,(*Reader).Reset assert x.(compress/flate.Resetter)
	// row: compress/gzip NewReader,(*Reader).Read,(*Reader).Reset panic x.(compress/flate.Resetter) without ok: *runtime.TypeAssertionError
	probe("gzip/Resetter", func() {
		data := append(gzipData("member one;", "one"), gzipData("member two", "two")...)
		rs, cs := readers(data)
		for i, r := range rs {
			z, err := gzip.NewReader(r)
			if err != nil {
				out("%d: NewReader %v", i, err)
				continue
			}
			b, err := io.ReadAll(z)
			out("%d: %q %q %v %v", i, z.Name, b, err, cs[i])
			err = z.Reset(&plainReader{r: bytes.NewReader(data)})
			z.Multistream(false)
			b, err2 := io.ReadAll(z)
			out("%d: single %v %q %q %v", i, err, z.Name, b, err2)
		}
	})
	// row: compress/lzw NewReader,(*Reader).Reset assert x.(io.ByteReader)
	probe("lzw/NewReader", func() {
		var b bytes.Buffer
		w := lzw.NewWriter(&b, lzw.LSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		rs, cs := readers(b.Bytes())
		for i, r := range rs {
			rc := lzw.NewReader(r, lzw.LSB, 8)
			got, err := io.ReadAll(rc)
			out("%d: %q %v %v", i, got, err, cs[i])
			rs2, cs2 := readers(b.Bytes())
			rc.(*lzw.Reader).Reset(rs2[i], lzw.LSB, 8)
			got, err = io.ReadAll(rc)
			out("Reset %d: %q %v %v", i, got, err, cs2[i])
		}
	})
	// row: compress/lzw NewWriter,(*Writer).Reset assert x.(compress/lzw.writer)
	probe("lzw/NewWriter", func() {
		fw := &flushWriter{}
		w := lzw.NewWriter(fw, lzw.MSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		out("flushWriter: %x WriteByte=%v flushes=%d", fw.b.Bytes(), fw.bytes, fw.flushes)
		gw := genFlushWriter[int]{&flushWriter{}}
		w = lzw.NewWriter(gw, lzw.MSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		out("generic: %x WriteByte=%v flushes=%d", gw.w.b.Bytes(), gw.w.bytes, gw.w.flushes)
		bw := &byteOnlyWriter{}
		w = lzw.NewWriter(bw, lzw.MSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		out("byteOnly: %x WriteByte=%v", bw.b.Bytes(), bw.bytes)
		fw2 := &flushWriter{}
		w.(*lzw.Writer).Reset(fw2, lzw.LSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		out("Reset flushWriter: %x WriteByte=%v flushes=%d", fw2.b.Bytes(), fw2.bytes, fw2.flushes)
		var sb strings.Builder
		w.(*lzw.Writer).Reset(&sb, lzw.LSB, 8)
		w.Write([]byte(lzwText))
		w.Close()
		out("Reset Builder: %x", sb.String())
	})
	// row: compress/zlib NewReader,NewReaderDict assert x.(compress/flate.Reader)
	probe("zlib/NewReader", func() {
		rs, cs := readers(zlibData("zlib zlib zlib", nil))
		for i, r := range rs {
			z, err := zlib.NewReader(r)
			if err != nil {
				out("%d: %v", i, err)
				continue
			}
			b, err := io.ReadAll(z)
			out("%d: %q %v %v", i, b, err, cs[i])
		}
		dict := []byte("zlib dictionary ")
		rs, cs = readers(zlibData("zlib dictionary used", dict))
		for i, r := range rs {
			z, err := zlib.NewReaderDict(r, dict)
			if err != nil {
				out("dict %d: %v", i, err)
				continue
			}
			b, err := io.ReadAll(z)
			out("dict %d: %q %v %v", i, b, err, cs[i])
		}
	})
}
