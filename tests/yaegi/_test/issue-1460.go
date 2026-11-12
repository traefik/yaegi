package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
)

func unmarshalJSON[T any](b []byte, x *[]T) error {
	if *x != nil {
		return errors.New("already initialized")
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, x)
}

func SliceOfViews[T ViewCloner[T, V], V StructView[T]](x []T) SliceView[T, V] {
	return SliceView[T, V]{x}
}

type StructView[T any] interface {
	Valid() bool
	AsStruct() T
}

type SliceView[T ViewCloner[T, V], V StructView[T]] struct {
	ж []T
}

type ViewCloner[T any, V StructView[T]] interface {
	View() V
	Clone() T
}

func (v SliceView[T, V]) MarshalJSON() ([]byte, error) { return json.Marshal(v.ж) }

func (v *SliceView[T, V]) UnmarshalJSON(b []byte) error { return unmarshalJSON(b, &v.ж) }

type Slice[T any] struct {
	ж []T
}

func (v Slice[T]) MarshalJSON() ([]byte, error) { return json.Marshal(v.ж) }

func (v *Slice[T]) UnmarshalJSON(b []byte) error { return unmarshalJSON(b, &v.ж) }

func SliceOf[T any](x []T) Slice[T] {
	return Slice[T]{x}
}

type IPPrefixSlice struct {
	ж Slice[netip.Prefix]
}

type viewStruct struct {
	Int        int
	Strings    Slice[string]
	StringsPtr *Slice[string] `json:",omitempty"`
}

func main() {
	ss := SliceOf([]string{"bar"})
	in := viewStruct{
		Int:        1234,
		Strings:    ss,
		StringsPtr: &ss,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "")
	err1 := encoder.Encode(&in)
	b := buf.Bytes()
	var got viewStruct
	err2 := json.Unmarshal(b, &got)
	println(err1 == nil, err2 == nil, reflect.DeepEqual(got, in))
}

// Output:
// true true true
