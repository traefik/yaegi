// Native boundary of encoding/asn1: program types given to asn1 (docs/BOUNDARY.md).
package main

import (
	"encoding/asn1"
	"fmt"
	"math/big"
	"reflect"
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

// Named types.
type (
	NInt   int
	NI64   int64
	NBool  bool
	Named  string
	NB     []byte
	IntSET []NInt // the name ends with SET: a SET OF
	IntSeq []NInt
	BigP   *big.Int
	Strs   []Named
)

// Rec is the main structure, with fields of many forms.
type Rec struct {
	Version NInt `asn1:"optional,explicit,default:1,tag:0"`
	Serial  *big.Int
	Name    Named
	Ok      NBool
	Set     IntSET
	Seq     IntSeq
	Bytes   NB
	Any     any
	Opt     NI64 `asn1:"optional"`
	Strs    Strs `asn1:"omitempty"`
	OID     asn1.ObjectIdentifier
	Sub     Sub
	Enum    asn1.Enumerated
}

type Sub struct {
	A NInt
	B Named `asn1:"utf8"`
}

// WithRaw keeps its raw bytes.
type WithRaw struct {
	Raw asn1.RawContent
	A   NInt
}

// Pair is a generic structure.
type Pair[T any] struct{ A, B T }

// BigField has a field of a named pointer to big.Int.
type BigField struct{ B BigP }

// marshalBoth gives v to Marshal and MarshalWithParams.
func marshalBoth(label string, v any, params string) []byte {
	b, err := asn1.Marshal(v)
	out("%s Marshal: %x %v", label, b, err)
	b2, err := asn1.MarshalWithParams(v, params)
	out("%s MarshalWithParams(%q): %x %v", label, params, b2, err)
	return b
}

// unmarshalBoth decodes data with Unmarshal and UnmarshalWithParams into
// new targets made by mk.
func unmarshalBoth(label string, data []byte, mk func() any, params string) {
	t := mk()
	rest, err := asn1.Unmarshal(data, t)
	out("%s Unmarshal: %s rest=%d %v", label, show(t), len(rest), err)
	t = mk()
	rest, err = asn1.UnmarshalWithParams(data, t, params)
	out("%s UnmarshalWithParams(%q): %s rest=%d %v", label, params, show(t), len(rest), err)
}

func show(v any) string {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Sprintf("%v", v)
	}
	switch v := v.(type) {
	case *Rec:
		return fmt.Sprintf("{%d %v %s %v %v %v %x %T:%v %d %v %v %+v %d}", v.Version, v.Serial, v.Name, v.Ok, v.Set, v.Seq, v.Bytes, v.Any, v.Any, v.Opt, v.Strs, v.OID, v.Sub, v.Enum)
	case *WithRaw:
		return fmt.Sprintf("{%x %d}", []byte(v.Raw), v.A)
	case **big.Int:
		return (*v).String()
	case *BigField:
		if v.B == nil {
			return "{nil}"
		}
		return fmt.Sprintf("{%v}", (*big.Int)(v.B))
	}
	return fmt.Sprintf("%+v", rv.Elem().Interface())
}

func main() {
	rec := Rec{
		Version: 1, Serial: big.NewInt(1234567), Name: "name", Ok: true,
		Set: IntSET{3, 1, 2}, Seq: IntSeq{3, 1, 2}, Bytes: NB{1, 2},
		Any: Sub{A: 5, B: "in any"}, OID: asn1.ObjectIdentifier{1, 2, 3}, Sub: Sub{A: 7, B: "é"}, Enum: 2,
	}
	var recBytes []byte
	// row: encoding/asn1 Marshal,MarshalWithParams reflect *
	probe("Marshal/Rec", func() {
		marshalBoth("Rec", rec, "")
		marshalBoth("*Rec", &rec, "explicit,tag:3")
		r2 := rec
		r2.Version = 2
		r2.Opt = 9
		r2.Strs = Strs{"a", "b"}
		r2.Any = NInt(4)
		recBytes = marshalBoth("Rec2", r2, "set")
	})
	// row: encoding/asn1 Marshal,MarshalWithParams reflect *
	probe("Marshal/values", func() {
		marshalBoth("NInt", NInt(300), "tag:1")
		marshalBoth("NBool", NBool(true), "")
		marshalBoth("Named", Named("x"), "ia5")
		marshalBoth("NB", NB{9}, "")
		marshalBoth("IntSET", IntSET{2, 1}, "")
		marshalBoth("[]any", []any{NInt(1), Named("a"), Sub{1, "s"}}, "")
		marshalBoth("Pair", Pair[NInt]{1, 2}, "")
		marshalBoth("Pair[Named]", Pair[Named]{"a", "b"}, "")
		marshalBoth("WithRaw", WithRaw{Raw: asn1.RawContent{0x30, 0x03, 0x02, 0x01, 0x07}, A: 1}, "")
		marshalBoth("WithRaw empty", WithRaw{A: 1}, "")
		marshalBoth("BigField", BigField{B: BigP(big.NewInt(5))}, "")
		marshalBoth("map", map[Named]NInt{"a": 1}, "")
		marshalBoth("float", 1.5, "")
		marshalBoth("nil any", nil, "")
		marshalBoth("empty struct", struct{}{}, "")
		marshalBoth("unexported", struct{ a int }{1}, "")
	})
	// row: encoding/asn1 Unmarshal,UnmarshalWithParams reflect *
	// row: encoding/asn1 Unmarshal,UnmarshalWithParams assert x.(T), T unnamed
	probe("Unmarshal/Rec", func() {
		unmarshalBoth("Rec", recBytes, func() any { return &Rec{} }, "")
		unmarshalBoth("WithRaw", []byte{0x30, 0x03, 0x02, 0x01, 0x07}, func() any { return &WithRaw{} }, "")
		unmarshalBoth("Pair", []byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02}, func() any { return &Pair[NInt]{} }, "")
		unmarshalBoth("BigField", []byte{0x30, 0x03, 0x02, 0x01, 0x05}, func() any { return &BigField{} }, "")
		unmarshalBoth("NInt", []byte{0x02, 0x02, 0x01, 0x2c}, func() any { return new(NInt) }, "")
		unmarshalBoth("NInt tag", []byte{0x81, 0x02, 0x01, 0x2c}, func() any { return new(NInt) }, "tag:1")
		unmarshalBoth("Named", []byte{0x13, 0x01, 0x78}, func() any { return new(Named) }, "")
		unmarshalBoth("NBool", []byte{0x01, 0x01, 0xff}, func() any { return new(NBool) }, "")
		unmarshalBoth("NB", []byte{0x04, 0x01, 0x09}, func() any { return new(NB) }, "")
		unmarshalBoth("IntSET", []byte{0x31, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02}, func() any { return new(IntSET) }, "")
		unmarshalBoth("IntSeq from SET", []byte{0x31, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x02}, func() any { return new(IntSeq) }, "")
		unmarshalBoth("*big.Int", []byte{0x02, 0x02, 0x01, 0x2c}, func() any { return new(*big.Int) }, "")
		unmarshalBoth("BigP", []byte{0x02, 0x02, 0x01, 0x2c}, func() any { return new(BigP) }, "")
		unmarshalBoth("any", []byte{0x02, 0x02, 0x01, 0x2c}, func() any { var a any; return &a }, "")
		unmarshalBoth("any seq", []byte{0x30, 0x03, 0x02, 0x01, 0x07}, func() any { var a any; return &a }, "")
		unmarshalBoth("wrong", []byte{0x02, 0x01, 0x01}, func() any { return new(Named) }, "")
		unmarshalBoth("non-pointer", []byte{0x02, 0x01, 0x01}, func() any { return NInt(0) }, "")
		unmarshalBoth("nil pointer", []byte{0x02, 0x01, 0x01}, func() any { return (*NInt)(nil) }, "")
		unmarshalBoth("map", []byte{0x02, 0x01, 0x01}, func() any { return &map[Named]NInt{} }, "")
	})
}
