// +build go1.12,!go1.13

package stdlib

// Code generated by 'goexports hash'. DO NOT EDIT.

import (
	"hash"
	"reflect"
)

func init() {
	Value["hash"] = map[string]reflect.Value{
		// function, constant and variable definitions

		// type definitions
		"Hash":   reflect.ValueOf((*hash.Hash)(nil)),
		"Hash32": reflect.ValueOf((*hash.Hash32)(nil)),
		"Hash64": reflect.ValueOf((*hash.Hash64)(nil)),
	}
	Wrapper["hash"] = map[string]reflect.Type{
		"Hash":   reflect.TypeOf((*_hash_Hash)(nil)),
		"Hash32": reflect.TypeOf((*_hash_Hash32)(nil)),
		"Hash64": reflect.TypeOf((*_hash_Hash64)(nil)),
	}
}

// _hash_Hash is an interface wrapper for Hash type
type _hash_Hash struct {
	WBlockSize func() int
	WReset     func()
	WSize      func() int
	WSum       func(b []byte) []byte
	WWrite     func(p []byte) (n int, err error)
}

func (W _hash_Hash) BlockSize() int                    { return W.WBlockSize() }
func (W _hash_Hash) Reset()                            { W.WReset() }
func (W _hash_Hash) Size() int                         { return W.WSize() }
func (W _hash_Hash) Sum(b []byte) []byte               { return W.WSum(b) }
func (W _hash_Hash) Write(p []byte) (n int, err error) { return W.WWrite(p) }

// _hash_Hash32 is an interface wrapper for Hash32 type
type _hash_Hash32 struct {
	WBlockSize func() int
	WReset     func()
	WSize      func() int
	WSum       func(b []byte) []byte
	WSum32     func() uint32
	WWrite     func(p []byte) (n int, err error)
}

func (W _hash_Hash32) BlockSize() int                    { return W.WBlockSize() }
func (W _hash_Hash32) Reset()                            { W.WReset() }
func (W _hash_Hash32) Size() int                         { return W.WSize() }
func (W _hash_Hash32) Sum(b []byte) []byte               { return W.WSum(b) }
func (W _hash_Hash32) Sum32() uint32                     { return W.WSum32() }
func (W _hash_Hash32) Write(p []byte) (n int, err error) { return W.WWrite(p) }

// _hash_Hash64 is an interface wrapper for Hash64 type
type _hash_Hash64 struct {
	WBlockSize func() int
	WReset     func()
	WSize      func() int
	WSum       func(b []byte) []byte
	WSum64     func() uint64
	WWrite     func(p []byte) (n int, err error)
}

func (W _hash_Hash64) BlockSize() int                    { return W.WBlockSize() }
func (W _hash_Hash64) Reset()                            { W.WReset() }
func (W _hash_Hash64) Size() int                         { return W.WSize() }
func (W _hash_Hash64) Sum(b []byte) []byte               { return W.WSum(b) }
func (W _hash_Hash64) Sum64() uint64                     { return W.WSum64() }
func (W _hash_Hash64) Write(p []byte) (n int, err error) { return W.WWrite(p) }
