// Native boundary of archive/zip: program io.ReadCloser values made by a
// program decompressor, given back by (*Reader).Open (docs/BOUNDARY.md).
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
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

const (
	methodFile   = 0xF0 // the decompressor gives a fs.File
	methodNoStat = 0xF1 // the decompressor gives an io.ReadCloser without Stat
	methodGen    = 0xF2 // the decompressor gives a generic fs.File
)

var mtime = time.Date(2023, 1, 2, 3, 4, 6, 0, time.UTC)

// fileRC is an io.ReadCloser with Stat (pointer receivers): it is a fs.File.
type fileRC struct{ r io.Reader }

func (f *fileRC) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *fileRC) Close() error               { return nil }
func (f *fileRC) Stat() (fs.FileInfo, error) { return nil, fmt.Errorf("fileRC.Stat") }

// noStatRC is an io.ReadCloser without Stat.
type noStatRC struct{ r io.Reader }

func (f noStatRC) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f noStatRC) Close() error               { return nil }

// genRC is a generic io.ReadCloser with Stat.
type genRC[T any] struct {
	r   io.Reader
	tag T
}

func (f genRC[T]) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f genRC[T]) Close() error               { return nil }
func (f genRC[T]) Stat() (fs.FileInfo, error) { return nil, fmt.Errorf("genRC.Stat %v", f.tag) }

func nopCompressor(w io.Writer) (io.WriteCloser, error) { return nopWC{w}, nil }

type nopWC struct{ io.Writer }

func (nopWC) Close() error { return nil }

// makeZip writes one file per method, with a program compressor that
// stores the bytes.
func makeZip() []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, m := range []uint16{methodFile, methodNoStat, methodGen, zip.Store} {
		zw.RegisterCompressor(m, nopCompressor)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("m%x.txt", m), Method: m, Modified: mtime})
		fmt.Fprintf(w, "content of method %x", m)
	}
	zw.Close()
	return b.Bytes()
}

func register(r *zip.Reader) {
	r.RegisterDecompressor(methodFile, func(r io.Reader) io.ReadCloser { return &fileRC{r} })
	r.RegisterDecompressor(methodNoStat, func(r io.Reader) io.ReadCloser { return noStatRC{r} })
	r.RegisterDecompressor(methodGen, func(r io.Reader) io.ReadCloser { return genRC[int]{r, 9} })
}

func openAll(open func(string) (fs.File, error)) {
	for _, name := range []string{"mf0.txt", "mf1.txt", "mf2.txt", "m0.txt"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					out("%s: panic: %T: %v", name, r, r)
				}
			}()
			f, err := open(name)
			if err != nil {
				out("%s: %v", name, err)
				return
			}
			b, err := io.ReadAll(f)
			_, serr := f.Stat()
			out("%s: %T %q %v stat err %v", name, f, b, err, serr)
			f.Close()
		}()
	}
}

func main() {
	// row: archive/zip (*Reader).Open assert x.(io/fs.File)
	// row: archive/zip (*Reader).Open panic x.(io/fs.File) without ok: *runtime.TypeAssertionError
	probe("Reader.Open", func() {
		data := makeZip()
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			out("NewReader: %v", err)
			return
		}
		register(r)
		openAll(r.Open)
	})
	// row: archive/zip (*ReadCloser).Open assert x.(io/fs.File)
	// row: archive/zip (*ReadCloser).Open panic x.(io/fs.File) without ok: *runtime.TypeAssertionError
	probe("ReadCloser.Open", func() {
		dir, err := os.MkdirTemp("", "boundary-zip")
		if err != nil {
			out("MkdirTemp: %v", err)
			return
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "a.zip")
		if err := os.WriteFile(path, makeZip(), 0o600); err != nil {
			out("WriteFile: %v", err)
			return
		}
		rc, err := zip.OpenReader(path)
		if err != nil {
			out("OpenReader: %v", err)
			return
		}
		defer rc.Close()
		register(&rc.Reader)
		openAll(rc.Open)
	})
}
