package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// io.Copy uses WriteTo of the source and ReadFrom of the destination, also
// when the type has methods that the Reader or Writer family of the
// wrappers (vm/optwrap.go) does not have: Write on a reader, ReadAt.

// Read, Write, WriteTo
type rwt struct{ r io.Reader }

func (x *rwt) Read(p []byte) (int, error)  { return x.r.Read(p) }
func (x *rwt) Write(p []byte) (int, error) { return len(p), nil }
func (x *rwt) WriteTo(w io.Writer) (int64, error) {
	fmt.Println("rwt WriteTo called")
	return io.Copy(w, x.r)
}

// Read, ReadAt, Seek, Close, WriteTo, and the rune methods
type rcs struct{ *strings.Reader }

func (x rcs) Close() error { return nil }
func (x rcs) WriteTo(w io.Writer) (int64, error) {
	fmt.Println("rcs WriteTo called")
	return x.Reader.WriteTo(w)
}

// Read, Write, Close, ReadFrom
type wrf struct{ b bytes.Buffer }

func (x *wrf) Write(p []byte) (int, error) { return x.b.Write(p) }
func (x *wrf) Close() error                { return nil }
func (x *wrf) Read(p []byte) (int, error)  { return 0, io.EOF }
func (x *wrf) ReadFrom(r io.Reader) (int64, error) {
	fmt.Println("wrf ReadFrom called")
	return x.b.ReadFrom(r)
}

func main() {
	n, err := io.Copy(os.Stdout, &rwt{strings.NewReader("hello\n")})
	fmt.Println(n, err)
	n, err = io.Copy(os.Stdout, rcs{strings.NewReader("seek\n")})
	fmt.Println(n, err)
	n, err = io.CopyN(io.Discard, rcs{strings.NewReader("seek\n")}, 3)
	fmt.Println(n, err)
	d := &wrf{}
	n, err = io.Copy(d, strings.NewReader("abc"))
	fmt.Println(n, err, d.b.String())
}
