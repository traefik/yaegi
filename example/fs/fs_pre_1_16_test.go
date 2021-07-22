//+build !go1.16

package fs1

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/fs"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// fakeFS and friends are more complicated than I'd like
// in a test, but when we're older than go.1.16 we need to
// create our own equivalent of a fstest.MapFS to test that
// filesystems still work when fs.FS is not available.
//
type fakeFS map[string]*fakeFile

func (f fakeFS) Open(name string) (fs.File, error) {
	// split the path into bits and walk into subdirs to
	// find what we're after.
	pathBits := strings.Split(name, "/")
	currentFS := &f
	var file *fakeFile
	for _, findPath := range pathBits {
		var found bool
		if file, found = (*currentFS)[findPath]; !found {
			file = nil
			break
		}
		if file.fileInfo.kids != nil {
			currentFS = file.fileInfo.kids
		}
	}
	if file == nil {
		return nil, os.ErrNotExist
	}
	return file, nil
}

// fakeFile is both an io.File and a io.DirEntry for convenience.
type fakeFile struct {
	fileInfo   *fakeFileInfo
	dataReader *bytes.Reader
}

func (ff *fakeFile) Stat() (os.FileInfo, error) {
	return ff.fileInfo, nil
}
func (ff *fakeFile) Read(buffer []byte) (int, error) {
	var out bytes.Buffer
	if ff.dataReader == nil {
		ff.dataReader = bytes.NewReader(ff.fileInfo.data)
	}
	nBytes, err := io.CopyN(&out, ff.dataReader, int64(cap(buffer)))

	copy(buffer, out.Bytes())

	return int(nBytes), err
}
func (ff *fakeFile) Close() error {
	return nil
}
func (ff *fakeFile) Readdir(n int) ([]os.FileInfo, error) {
	dirEntries := []os.FileInfo{}
	if ff.fileInfo.kids != nil {
		for _, kid := range *ff.fileInfo.kids {
			dirEntries = append(dirEntries, kid.fileInfo)
		}
		return dirEntries, nil
	}
	return dirEntries, io.EOF
}

type fakeFileInfo struct {
	name string
	data []byte  // if a File will have data
	kids *fakeFS // if a Dir it will have kids
}

// IsDir is required to look like an os.File.
func (ffi *fakeFileInfo) IsDir() bool {
	return ffi.kids != nil
}

// ModTime is required to look like an os.File.
func (ffi *fakeFileInfo) ModTime() time.Time {
	return time.Now()
}

// Mode is required to look like an os.File.
func (ffi *fakeFileInfo) Mode() os.FileMode {
	if ffi.IsDir() {
		return os.ModeDir
	}
	return os.ModePerm
}

// Name is required to look like an os.File.
func (ffi *fakeFileInfo) Name() string {
	return ffi.name
}

// Size is required to look like an os.File.
func (ffi *fakeFileInfo) Size() int64 {
	return int64(len(ffi.data))
}

// Sys is required to look like an os.File.
func (ffi *fakeFileInfo) Sys() interface{} {
	return "n/a"
}

var (
	testingFS = &fakeFS{
		"main.go": &fakeFile{
			fileInfo: &fakeFileInfo{
				name: "main.go",
				data: []byte(`package main

import (
	"./localfoo"
)

func main() {
	localfoo.PrintSomething()
}
`),
			},
		},
		"localfoo": &fakeFile{
			fileInfo: &fakeFileInfo{
				name: "localfoo",
				kids: &fakeFS{
					"foo.go": &fakeFile{
						fileInfo: &fakeFileInfo{
							name: "foo.go",
							data: []byte(`package localfoo

import "fmt"

func PrintSomething() {
	fmt.Println("This is localfoo printing something!")
}
`),
						},
					},
				},
			},
		},
	}
)

func TestFilesystemMapFS(t *testing.T) {
	i := interp.New(interp.Options{
		GoPath:     "./_pkg",
		Filesystem: testingFS,
	})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.EvalPath(`main.go`)
	if err != nil {
		t.Fatal(err)
	}
}
