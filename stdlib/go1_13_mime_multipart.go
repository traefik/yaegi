// Code generated by 'github.com/containous/yaegi/extract mime/multipart'. DO NOT EDIT.

// +build go1.13,!go1.14

package stdlib

import (
	"mime/multipart"
	"reflect"
)

func init() {
	Symbols["mime/multipart"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"ErrMessageTooLarge": reflect.ValueOf(&multipart.ErrMessageTooLarge).Elem(),
		"NewReader":          reflect.ValueOf(multipart.NewReader),
		"NewWriter":          reflect.ValueOf(multipart.NewWriter),

		// type definitions
		"File":       reflect.ValueOf((*multipart.File)(nil)),
		"FileHeader": reflect.ValueOf((*multipart.FileHeader)(nil)),
		"Form":       reflect.ValueOf((*multipart.Form)(nil)),
		"Part":       reflect.ValueOf((*multipart.Part)(nil)),
		"Reader":     reflect.ValueOf((*multipart.Reader)(nil)),
		"Writer":     reflect.ValueOf((*multipart.Writer)(nil)),

		// interface wrapper definitions
		"_File": reflect.ValueOf((*_mime_multipart_File)(nil)),
	}
}

// _mime_multipart_File is an interface wrapper for File type
type _mime_multipart_File struct {
	WClose  func() error
	WRead   func(p []byte) (n int, err error)
	WReadAt func(p []byte, off int64) (n int, err error)
	WSeek   func(offset int64, whence int) (int64, error)
}

func (W _mime_multipart_File) Close() error                                  { return W.WClose() }
func (W _mime_multipart_File) Read(p []byte) (n int, err error)              { return W.WRead(p) }
func (W _mime_multipart_File) ReadAt(p []byte, off int64) (n int, err error) { return W.WReadAt(p, off) }
func (W _mime_multipart_File) Seek(offset int64, whence int) (int64, error) {
	return W.WSeek(offset, whence)
}
