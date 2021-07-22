// +build !go1.16

// fs.FS is only available from go 1.16 onwards, so for 1.15
// and below we need to implement something that mimics fs.FS
// as closely as we can.

package fs

import (
	"io"
	"os"
	"sort"
)

// FS tries to mimic the unavailable fs.FS. We have to supply
// interfaces and type that complies with the fs.FS interface
// so as not to break the main code.
//
// We do this by cribbing from the fs.FS implementation
// in 1.16.
//
type FS interface {
	// Note: Open has a different signature to 1.16 - so if
	// someone migrates from using a custom fs under 1.15
	// (maybe we should prevent this at all?) to 1.16+ then
	// they may have some adjustments to make (*os.File vs fs.File)
	//
	Open(name string) (File, error)
}

type File interface {
	Stat() (os.FileInfo, error)
	Read([]byte) (int, error)
	Close() error
	Readdir(n int) ([]os.FileInfo, error)
}

// RealFS complies with the FS interface. It simply overlays
// the existing default filesystem.
type RealFS struct{}

// Open is a thin layer around os.Open to confirm with the mimic of fs.FS.
func (dir RealFS) Open(name string) (File, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ReadDir polyfill that mimics the 1.16 fs.ReadDir as closely as we can.
func ReadDir(fsys FS, name string) ([]os.FileInfo, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	list, err := file.Readdir(-1)
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, err
}

// Stat polyfill that mimics the 1.16 fs.Stat as closely as we can.
func Stat(fsys FS, name string) (os.FileInfo, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

// ReadFile polyfill that mimics the 1.16 fs.ReadFile as closely as we can.
func ReadFile(fsys FS, name string) ([]byte, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var size int
	if info, err := file.Stat(); err == nil {
		size64 := info.Size()
		if int64(int(size64)) == size64 {
			size = int(size64)
		}
	}

	data := make([]byte, 0, size+1)
	for {
		if len(data) >= cap(data) {
			d := append(data[:cap(data)], 0)
			data = d[:len(data)]
		}
		n, err := file.Read(data[len(data):cap(data)])
		data = data[:len(data)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return data, err
		}
	}
}
