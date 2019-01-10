package stdlib

// Code generated by 'goexports archive/zip'. DO NOT EDIT.

import (
	"archive/zip"
	"reflect"
)

func init() {
	Value["archive/zip"] = map[string]reflect.Value{
		"Deflate":              reflect.ValueOf(zip.Deflate),
		"ErrAlgorithm":         reflect.ValueOf(zip.ErrAlgorithm),
		"ErrChecksum":          reflect.ValueOf(zip.ErrChecksum),
		"ErrFormat":            reflect.ValueOf(zip.ErrFormat),
		"FileInfoHeader":       reflect.ValueOf(zip.FileInfoHeader),
		"NewReader":            reflect.ValueOf(zip.NewReader),
		"NewWriter":            reflect.ValueOf(zip.NewWriter),
		"OpenReader":           reflect.ValueOf(zip.OpenReader),
		"RegisterCompressor":   reflect.ValueOf(zip.RegisterCompressor),
		"RegisterDecompressor": reflect.ValueOf(zip.RegisterDecompressor),
		"Store":                reflect.ValueOf(zip.Store),
	}

	Type["archive/zip"] = map[string]reflect.Type{
		"Compressor":   reflect.TypeOf((*zip.Compressor)(nil)).Elem(),
		"Decompressor": reflect.TypeOf((*zip.Decompressor)(nil)).Elem(),
		"File":         reflect.TypeOf((*zip.File)(nil)).Elem(),
		"FileHeader":   reflect.TypeOf((*zip.FileHeader)(nil)).Elem(),
		"ReadCloser":   reflect.TypeOf((*zip.ReadCloser)(nil)).Elem(),
		"Reader":       reflect.TypeOf((*zip.Reader)(nil)).Elem(),
		"Writer":       reflect.TypeOf((*zip.Writer)(nil)).Elem(),
	}
}
