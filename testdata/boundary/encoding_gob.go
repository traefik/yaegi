// Native boundary of encoding/gob: program types given to gob (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"reflect"
	"sort"
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

// GE has GobEncode (value receiver) and GobDecode (pointer receiver).
type GE struct{ n int }

func (g GE) GobEncode() ([]byte, error) { return []byte(fmt.Sprintf("ge%d", g.n)), nil }
func (g *GE) GobDecode(b []byte) error {
	_, err := fmt.Sscanf(string(b), "ge%d", &g.n)
	g.n *= 10
	return err
}

// GG is a generic type with GobEncode and GobDecode.
type GG[T any] struct{ v string }

func (g GG[T]) GobEncode() ([]byte, error) { return []byte("gg:" + g.v), nil }
func (g *GG[T]) GobDecode(b []byte) error {
	var z T
	g.v = fmt.Sprintf("%T<%s>", z, b)
	return nil
}

// BM has MarshalBinary and UnmarshalBinary.
type BM struct{ s string }

func (b BM) MarshalBinary() ([]byte, error) { return []byte("bm:" + b.s), nil }
func (b *BM) UnmarshalBinary(d []byte) error {
	b.s = "un(" + string(d) + ")"
	return nil
}

// TM has MarshalText and UnmarshalText.
type TM struct{ s string }

func (t TM) MarshalText() ([]byte, error) { return []byte("tm:" + t.s), nil }
func (t *TM) UnmarshalText(d []byte) error {
	t.s = "untext(" + string(d) + ")"
	return nil
}

// Named basic types.
type (
	Named string
	NInt  int
	NF    float64
	NBool bool
	NB    []byte
)

// Shape is an interface with two program implementations.
type Shape interface{ Area() int }

type Circle struct{ R int }

func (c Circle) Area() int { return 3 * c.R * c.R }

type Square struct{ S NInt }

func (s *Square) Area() int { return int(s.S * s.S) }

// Unreg is never registered.
type Unreg struct{ X int }

func (Unreg) Area() int { return 0 }

// Node is a recursive type.
type Node struct {
	V    NInt
	Next *Node
}

// RP is a recursive pointer type.
type RP *RP

// Msg has fields of many forms.
type Msg struct {
	Name  Named
	Vals  []NInt
	M     map[Named]NF
	Arr   [3]NBool
	Bools []bool
	NBs   []NBool
	P     *NInt
	I     Shape
	G     GE
	PG    *GE
	GG    GG[NInt]
	B     BM
	List  *Node
	Bs    NB
	Any   any
}

// Msg2 has a part of the fields of Msg, in another order.
type Msg2 struct {
	Vals []int
	Name string
	M    map[string]float64
	G    GE
}

// Bad has a field gob cannot decode into.
type Bad struct {
	Name []int
}

func showMsg(m Msg) string {
	keys := make([]string, 0, len(m.M))
	for k, v := range m.M {
		keys = append(keys, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(keys)
	p := "nil"
	if m.P != nil {
		p = fmt.Sprint(*m.P)
	}
	var list []NInt
	for n := m.List; n != nil; n = n.Next {
		list = append(list, n.V)
	}
	pg := "nil"
	if m.PG != nil {
		pg = fmt.Sprint(m.PG.n)
	}
	area := -1
	if m.I != nil {
		area = m.I.Area()
	}
	return fmt.Sprintf("%s %v %v %v %v %v %s %T:%d %d %s %s %s %v %q %v", m.Name, m.Vals, keys, m.Arr, m.Bools, m.NBs, p, m.I, area, m.G.n, pg, m.GG.v, m.B.s, list, m.Bs, m.Any)
}

// roundTrip encodes v with Encode (or EncodeValue), and decodes it into
// dst with Decode (or DecodeValue).
func roundTrip(label string, v, dst any, value bool) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	var err error
	if value {
		err = enc.EncodeValue(reflect.ValueOf(v))
	} else {
		err = enc.Encode(v)
	}
	if err != nil {
		out("%s encode: %v", label, err)
		return
	}
	dec := gob.NewDecoder(&buf)
	if value {
		err = dec.DecodeValue(reflect.ValueOf(dst))
	} else {
		err = dec.Decode(dst)
	}
	out("%s decode: %v", label, err)
}

func init() {
	gob.Register(Circle{})
	gob.Register(&Square{})
	gob.Register(GE{})
	gob.Register(Named(""))
	gob.Register(map[Named]NInt{})
}

func main() {
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue,(*Decoder).Decode,(*Decoder).DecodeValue reflect *
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue assert x.(T), T unnamed
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue,(*Decoder).Decode,(*Decoder).DecodeValue panic map key of interface type: hash of unhashable type
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue,(*Decoder).Decode,(*Decoder).DecodeValue panic == of two interfaces: comparing uncomparable type
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	probe("RoundTrip/Msg", func() {
		n := NInt(42)
		m := Msg{
			Name: "msg", Vals: []NInt{1, 2}, M: map[Named]NF{"a": 1.5, "b": 2},
			Arr: [3]NBool{true, false, true}, Bools: []bool{true, true}, NBs: []NBool{false, true},
			P: &n, I: &Square{3}, G: GE{4}, PG: &GE{5}, GG: GG[NInt]{"g"}, B: BM{"b"},
			List: &Node{1, &Node{2, &Node{3, nil}}}, Bs: NB("bytes"), Any: Circle{2},
		}
		for _, value := range []bool{false, true} {
			var got Msg
			roundTrip(fmt.Sprint("Msg value=", value), m, &got, value)
			out("got: %s", showMsg(got))
			var got2 Msg2
			roundTrip(fmt.Sprint("Msg->Msg2 value=", value), &m, &got2, value)
			out("got2: %+v", got2)
			var bad Bad
			roundTrip(fmt.Sprint("Msg->Bad value=", value), m, &bad, value)
		}
		m.I = Circle{1}
		m.Any = map[Named]NInt{"k": 7}
		var got Msg
		roundTrip("Circle/map", m, &got, false)
		out("got: %s", showMsg(got))
	})
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue,(*Decoder).Decode,(*Decoder).DecodeValue reflect *
	probe("RoundTrip/top", func() {
		vals := []any{GE{7}, &GE{8}, GG[string]{"x"}, BM{"top"}, TM{"t"}, Named("n"), NInt(-5), NF(0.5), NBool(true), NB("b"),
			[]NInt{1, 2, 3}, map[Named][]NInt{"a": {1}}, [2]Named{"x", "y"}, &Node{V: 1, Next: &Node{V: 2}}}
		for i, v := range vals {
			for _, value := range []bool{false, true} {
				dst := reflect.New(reflect.TypeOf(v))
				roundTrip(fmt.Sprintf("%d value=%v", i, value), v, dst.Interface(), value)
				out("%d got: %v", i, show(dst.Elem().Interface()))
			}
		}
		// decode into a type that does not match
		var s Named
		roundTrip("NInt->Named", NInt(1), &s, false)
		var nn []Named
		roundTrip("[]NInt->[]Named", []NInt{1}, &nn, false)
		var mm map[NInt]Named
		roundTrip("map->map", map[Named]NInt{"a": 1}, &mm, false)
	})
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue assert x.(T), T unnamed
	probe("Interface", func() {
		for i, v := range []Shape{Circle{1}, &Square{2}, Unreg{3}} {
			var buf bytes.Buffer
			err := gob.NewEncoder(&buf).Encode(struct{ S Shape }{v})
			out("%d: encode %v", i, err)
			if err != nil {
				continue
			}
			var got struct{ S Shape }
			err = gob.NewDecoder(&buf).Decode(&got)
			out("%d: decode %T %d %v", i, got.S, got.S.Area(), err)
		}
		var buf bytes.Buffer
		err := gob.NewEncoder(&buf).Encode([]any{Circle{5}, Named("s"), GE{1}, map[Named]NInt{"x": 2}})
		var got []any
		err2 := gob.NewDecoder(&buf).Decode(&got)
		out("[]any: %v %v", err, err2)
		for i, g := range got {
			out("[]any %d: %T %v", i, g, show(g))
		}
		buf.Reset()
		err = gob.NewEncoder(&buf).Encode(map[string]any{"c": Circle{6}})
		var gm map[string]any
		err2 = gob.NewDecoder(&buf).Decode(&gm)
		out("map[string]any: %v %v %T %v", err, err2, gm["c"], gm["c"])
		buf.Reset()
		err = gob.NewEncoder(&buf).Encode([]any{Unreg{1}})
		out("unregistered: %v", err)
	})
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue panic panic message with the type name
	probe("NilPointer", func() {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		func() {
			defer func() { out("Encode: %v", recover()) }()
			out("err %v", enc.Encode((*Node)(nil)))
		}()
		func() {
			defer func() { out("EncodeValue: %v", recover()) }()
			out("err %v", enc.EncodeValue(reflect.ValueOf((*GE)(nil))))
		}()
		out("nil: %v", enc.Encode(nil))
		var s Shape
		out("nil Shape: %v", enc.Encode(s))
		out("nil EncodeValue: %v", enc.EncodeValue(reflect.Value{}))
	})
	// row: encoding/gob (*Encoder).Encode,(*Encoder).EncodeValue,(*Decoder).Decode,(*Decoder).DecodeValue panic == of two interfaces: comparing uncomparable type
	// row: encoding/gob Register,RegisterName panic == of two interfaces: comparing uncomparable type
	probe("RecursivePointer", func() {
		var r RP
		var buf bytes.Buffer
		out("Encode: %v", gob.NewEncoder(&buf).Encode(r))
		out("Encode ptr: %v", gob.NewEncoder(&buf).Encode(&r))
		out("Decode: %v", gob.NewDecoder(strings.NewReader("\x03\x04\x00\x02")).Decode(&r))
		func() {
			defer func() { out("Register: %v", recover()) }()
			gob.Register(r)
		}()
	})
	// row: encoding/gob Register,RegisterName reflect *
	// row: encoding/gob Register,RegisterName panic == of two interfaces: comparing uncomparable type
	probe("Register", func() {
		type A struct{ X int }
		type B struct{ X int }
		gob.RegisterName("dupname", A{})
		gob.RegisterName("dupname", A{}) // same name, same type: no panic
		func() {
			defer func() { out("same name: %v", recover()) }()
			gob.RegisterName("dupname", B{})
		}()
		func() {
			defer func() { out("same type: %v", recover()) }()
			gob.RegisterName("othername", A{})
		}()
		func() {
			defer func() { out("empty name: %v", recover()) }()
			gob.RegisterName("", B{})
		}()
		func() {
			defer func() { out("Register twice: %v", recover()) }()
			gob.Register(Circle{})
			gob.Register(&Square{})
		}()
		func() {
			defer func() { out("Register generic: %v", recover()) }()
			gob.Register(GG[NInt]{})
			gob.Register(&GG[string]{})
			gob.Register([]Circle{})
		}()
		func() {
			defer func() { out("Register value then pointer: %v", recover()) }()
			gob.Register(&Circle{})
		}()
		// a registered generic type goes through an interface
		var buf bytes.Buffer
		err := gob.NewEncoder(&buf).Encode([]any{GG[NInt]{"in any"}, []Circle{{9}}})
		var got []any
		err2 := gob.NewDecoder(&buf).Decode(&got)
		out("generic in []any: %v %v", err, err2)
		for i, g := range got {
			out("%d: %T %v", i, g, show(g))
		}
	})
	// row: encoding/gob NewDecoder assert x.(io.ByteReader)
	probe("NewDecoder", func() {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		enc.Encode(NInt(1))
		enc.Encode(Named("two"))
		data := buf.String()
		for i, r := range []io.Reader{br{strings.NewReader(data)}, &plainR{strings.NewReader(data)}, gbr[int]{strings.NewReader(data)}} {
			d := gob.NewDecoder(r)
			var n NInt
			err := d.Decode(&n)
			out("%d: %v %v left=%d", i, n, err, left(r))
			var s Named
			err = d.Decode(&s)
			out("%d: %v %v", i, s, err)
		}
	})
}

// br is a reader with ReadByte (from the embedded *strings.Reader).
type br struct{ *strings.Reader }

// plainR is a reader without ReadByte.
type plainR struct{ r *strings.Reader }

func (p *plainR) Read(b []byte) (int, error) { return p.r.Read(b) }

// gbr is a generic reader with ReadByte.
type gbr[T any] struct{ r *strings.Reader }

func (g gbr[T]) Read(b []byte) (int, error) { return g.r.Read(b) }
func (g gbr[T]) ReadByte() (byte, error)    { return g.r.ReadByte() }

// left gives the number of bytes not read yet in the reader under r.
func left(r io.Reader) int {
	switch r := r.(type) {
	case br:
		return r.Len()
	case *plainR:
		return r.r.Len()
	case gbr[int]:
		return r.r.Len()
	}
	return -1
}

// show prints a value without its pointers.
func show(v any) string {
	switch v := v.(type) {
	case GE:
		return fmt.Sprint("GE ", v.n)
	case *GE:
		return fmt.Sprint("*GE ", v.n)
	case GG[string]:
		return "GG " + v.v
	case GG[NInt]:
		return "GG " + v.v
	case BM:
		return "BM " + v.s
	case TM:
		return "TM " + v.s
	case *Node:
		s := ""
		for n := v; n != nil; n = n.Next {
			s += fmt.Sprint(n.V, ";")
		}
		return s
	}
	return fmt.Sprint(v)
}
