//go:build goexperiment.jsonv2

// Native boundary of encoding/json/v2: program types given to json/v2
// (docs/BOUNDARY.md). The package exists only with Go 1.27, or with
// GOEXPERIMENT=jsonv2.
package main

import (
	"bytes"
	"encoding"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Print(modal(fmt.Sprintf("%s: panic: %T: %v\n", id, r, r)))
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Print(modal(fmt.Sprintf(cur+": "+format+"\n", args...))) }

// modal writes the modal verb of the errors of json/v2 as "cannot": Go
// picks "cannot" or "unable to" at random once per process
// (errorModalVerb), so go run and yaegi v2 printed one or the other.
func modal(s string) string { return strings.ReplaceAll(s, "json: unable to ", "json: cannot ") }

// VM has MarshalJSON (value receiver).
type VM struct{ N int }

func (v VM) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"vm:%d"`, v.N)), nil }

// PM has MarshalJSON and UnmarshalJSON with pointer receivers; the
// marshal method works on a nil receiver.
type PM struct{ N int }

func (p *PM) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte(`"pm:nil"`), nil
	}
	return []byte(fmt.Sprintf(`"pm:%d"`, p.N)), nil
}

func (p *PM) UnmarshalJSON(b []byte) error {
	p.N = len(b)
	return nil
}

// PTo has MarshalJSONTo and UnmarshalJSONFrom (pointer receivers).
type PTo struct{ S string }

func (p *PTo) MarshalJSONTo(enc *jsontext.Encoder) error {
	if p == nil {
		return enc.WriteToken(jsontext.String("to:nil"))
	}
	return enc.WriteToken(jsontext.String("to:" + p.S))
}

func (p *PTo) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	v, err := dec.ReadValue()
	p.S = "from:" + string(v)
	return err
}

// TA has AppendText (pointer receiver).
type TA struct{ S string }

func (t *TA) AppendText(b []byte) ([]byte, error) {
	if t == nil {
		return append(b, "ta:nil"...), nil
	}
	return append(b, "ta:"+t.S...), nil
}

// TM has MarshalText (pointer receiver) and UnmarshalText.
type TM struct{ S string }

func (t *TM) MarshalText() ([]byte, error) {
	if t == nil {
		return []byte("tm:nil"), nil
	}
	return []byte("tm:" + t.S), nil
}

func (t *TM) UnmarshalText(b []byte) error {
	t.S = "untext:" + string(b)
	return nil
}

// GM is a generic type with MarshalJSON.
type GM[T any] struct{ V T }

func (g GM[T]) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"gm:%v"`, g.V)), nil }

// Z has IsZero (value receiver); PZ has it on the pointer.
type Z struct{ N int }

func (z Z) IsZero() bool { return z.N == 7 }

type PZ struct{ N int }

func (z *PZ) IsZero() bool { return z == nil || z.N == 8 }

// Zeroer is a program interface with IsZero.
type Zeroer interface{ IsZero() bool }

// Named basic types.
type (
	Named string
	NInt  int
	NF    float64
	Ints  []NInt
	Arr   [2]NInt
	MapN  map[Named]NInt
)

// Inner and Outer test embedded fields.
type Inner struct{ A, X NInt }

type Outer struct {
	*Inner
	B NInt
}

// Holder has interface fields that hold nil pointers of method types.
type Holder struct {
	To any
	M  any
	TA any
	TM any
	V  any
}

// ZS has omitzero fields.
type ZS struct {
	Z  Z      `json:",omitzero"`
	PZ *PZ    `json:",omitzero"`
	VZ PZ     `json:",omitzero"`
	IZ Zeroer `json:",omitzero"`
	N  NInt   `json:",omitzero"`
}

// ES has omitempty fields, for the legacy rules of v1.
type ES struct {
	S  Named `json:",omitempty"`
	L  Ints  `json:",omitempty"`
	A  Arr   `json:",omitempty"`
	M  MapN  `json:",omitempty"`
	P  *PM   `json:",omitempty"`
	I  any   `json:",omitempty"`
	NI NInt  `json:",omitempty"`
}

// Rec is a recursive type.
type Rec struct {
	Name string
	Kids []*Rec `json:",omitempty"`
}

// Ch is a type json cannot marshal.
type Ch chan int

// Key is a struct used as a map key: it has a slice, so it is not
// comparable when it is in an interface.
type Key struct{ L []int }

// fmtTag is a program option with the marker method of jsonopts.
type fmtTag struct {
	json.Options
	on bool
}

func (o fmtTag) ExperimentalSupportFormatTag() bool { return o.on }

// Dur has a time.Duration, which has no default representation.
type Dur struct{ D time.Duration }

// m3 marshals v with Marshal, MarshalWrite and MarshalEncode.
func m3(label string, v any, opts ...json.Options) {
	b, err := json.Marshal(v, opts...)
	out("%s Marshal: %s %v", label, b, err)
	var buf bytes.Buffer
	err = json.MarshalWrite(&buf, v, opts...)
	out("%s MarshalWrite: %s %v", label, strings.TrimSpace(buf.String()), err)
	buf.Reset()
	err = json.MarshalEncode(jsontext.NewEncoder(&buf), v, opts...)
	out("%s MarshalEncode: %s %v", label, strings.TrimSpace(buf.String()), err)
}

// u3 decodes data with Unmarshal, UnmarshalRead and UnmarshalDecode into
// new targets made by mk.
func u3(label, data string, mk func() any, show func(any) string, opts ...json.Options) {
	t := mk()
	err := json.Unmarshal([]byte(data), t, opts...)
	out("%s Unmarshal: %s %v", label, show(t), err)
	t = mk()
	err = json.UnmarshalRead(strings.NewReader(data), t, opts...)
	out("%s UnmarshalRead: %s %v", label, show(t), err)
	t = mk()
	err = json.UnmarshalDecode(jsontext.NewDecoder(strings.NewReader(data)), t, opts...)
	out("%s UnmarshalDecode: %s %v", label, show(t), err)
}

func showV(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Sprint(v)
	}
	return fmt.Sprintf("%+v", rv.Elem().Interface())
}

var all = []string{"Marshal", "MarshalEncode", "MarshalWrite", "Unmarshal", "UnmarshalDecode", "UnmarshalRead"}

func main() {
	_ = all
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite reflect method set
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite reflect kind
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite reflect type name
	probe("Marshal/methods", func() {
		m3("VM", VM{1})
		m3("*PM", &PM{2})
		m3("PM", PM{3})
		m3("*PTo", &PTo{"p"})
		m3("*TA", &TA{"a"})
		m3("*TM", &TM{"m"})
		m3("GM", GM[NInt]{4})
		m3("[]any", []any{VM{5}, &PM{6}, &PTo{"q"}, &TA{"b"}, &TM{"n"}, GM[string]{"g"}})
		m3("map", map[string]any{"a": VM{7}, "b": &TM{"o"}}, json.Deterministic(true))
		m3("Named", Named("s"))
		m3("NInt", NInt(-1))
		m3("Ints", Ints{1, 2})
		m3("nil Ints", Ints(nil))
		m3("Arr", Arr{1, 2})
		m3("MapN", MapN{"b": 2, "a": 1}, json.Deterministic(true))
		m3("map[TM]", map[TM]int{{"k"}: 1})
		m3("Rec", &Rec{Name: "r", Kids: []*Rec{{Name: "k"}}})
		var a any = GM[int]{8}
		m3("&any", &a)
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite assert x.(encoding/json/v2.MarshalerTo)
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite assert x.(encoding/json/v2.Marshaler)
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite assert x.(encoding.TextAppender)
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite assert x.(encoding.TextMarshaler)
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite panic x.(encoding/json/v2.MarshalerTo) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite panic x.(encoding/json/v2.Marshaler) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite panic x.(encoding.TextAppender) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite panic x.(encoding.TextMarshaler) without ok: *runtime.TypeAssertionError
	probe("Marshal/legacynil", func() {
		h := Holder{To: (*PTo)(nil), M: (*PM)(nil), TA: (*TA)(nil), TM: (*TM)(nil), V: (*VM)(nil)}
		m3("v2", h)
		m3("legacy", h, jsonv1.CallMethodsWithLegacySemantics(true))
		m3("v1 defaults", h, jsonv1.DefaultOptionsV1())
		// the same with interface fields of the method type
		type IH struct {
			To json.MarshalerTo
			M  json.Marshaler
			TA encoding.TextAppender
			TM encoding.TextMarshaler
		}
		ih := IH{To: (*PTo)(nil), M: (*PM)(nil), TA: (*TA)(nil), TM: (*TM)(nil)}
		m3("iface", ih)
		m3("iface legacy", ih, jsonv1.CallMethodsWithLegacySemantics(true))
		m3("iface non-nil legacy", IH{To: &PTo{"a"}, M: &PM{1}, TA: &TA{"b"}, TM: &TM{"c"}}, jsonv1.CallMethodsWithLegacySemantics(true))
		b, err := jsonv1.Marshal(h)
		out("v1 Marshal: %s %v", b, err)
	})
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(encoding/json/v2.MarshalerTo)
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(encoding/json/v2.Marshaler)
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(encoding.TextAppender)
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(encoding.TextMarshaler)
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(encoding/json/v2.MarshalerTo) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(encoding/json/v2.Marshaler) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(encoding.TextAppender) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(encoding.TextMarshaler) without ok: *runtime.TypeAssertionError
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead reflect method set
	probe("Unmarshal/methods", func() {
		u3("PM", `[1,2]`, func() any { return &PM{} }, showV)
		u3("PTo", `{"a":1}`, func() any { return &PTo{} }, showV)
		u3("TM", `"t"`, func() any { return &TM{} }, showV)
		show := func(v any) string {
			h := v.(*Holder)
			return fmt.Sprintf("%T:%v %T:%v %T:%v", h.To, showV(h.To), h.M, showV(h.M), h.TM, showV(h.TM))
		}
		mk := func() any { return &Holder{To: &PTo{}, M: &PM{}, TM: &TM{}} }
		u3("Holder", `{"To":"x","M":[1],"TM":"y"}`, mk, show)
		u3("Holder legacy", `{"To":"x","M":[1],"TM":"y"}`, mk, show, jsonv1.DefaultOptionsV1())
		mk = func() any { return &Holder{To: (*PTo)(nil), M: (*PM)(nil)} }
		u3("Holder nil", `{"To":"x","M":[1]}`, mk, show)
		u3("Holder nil legacy", `{"To":"x","M":[1]}`, mk, show, jsonv1.DefaultOptionsV1())
		u3("map[TM]", `{"k":1}`, func() any { return &map[TM]NInt{} }, showV)
		u3("GM in any", `{"V":1}`, func() any { var a any = &GM[NInt]{}; return &a }, func(v any) string { return showV(*v.(*any)) })
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(encoding/json/v2.isZeroer)
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(encoding/json/v2.isZeroer) without ok: *runtime.TypeAssertionError
	probe("omitzero", func() {
		m3("zero", ZS{})
		m3("IsZero true", ZS{Z: Z{7}, PZ: &PZ{8}, VZ: PZ{8}, IZ: Z{7}})
		m3("IsZero false", ZS{Z: Z{1}, PZ: &PZ{2}, VZ: PZ{3}, IZ: &PZ{4}, N: 5})
		m3("nil ptr in iface", ZS{IZ: (*PZ)(nil)})
		m3("&ZS", &ZS{VZ: PZ{8}})
		u3("ZS", `{"Z":{"N":1},"PZ":{"N":2},"N":3}`, func() any { return &ZS{} }, func(v any) string {
			z := v.(*ZS)
			return fmt.Sprintf("%v %v %v %v", z.Z, *z.PZ, z.VZ, z.N)
		})
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead reflect length
	probe("omitempty", func() {
		for i, o := range []json.Options{nil, jsonv1.OmitEmptyWithLegacySemantics(true)} {
			m3(fmt.Sprint(i, " zero"), ES{}, o)
			m3(fmt.Sprint(i, " empty"), ES{L: Ints{}, M: MapN{}}, o)
			m3(fmt.Sprint(i, " full"), ES{S: "s", L: Ints{1}, A: Arr{1}, M: MapN{"a": 1}, P: &PM{1}, I: VM{1}, NI: 1}, o)
		}
		u3("ES", `{"S":"s","L":[1,2],"A":[3],"M":{"k":4}}`, func() any { return &ES{} }, showV, jsonv1.OmitEmptyWithLegacySemantics(true))
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead reflect walk
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead panic map key of interface type: hash of unhashable type
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead reflect kind
	// row: encoding/json/v2 Unmarshal,UnmarshalDecode,UnmarshalRead reflect type name
	probe("embedded", func() {
		m3("Outer", Outer{Inner: &Inner{A: 1, X: 2}, B: 3})
		m3("Outer nil", Outer{B: 4})
		type local struct {
			Inner
			A   string // shadows Inner.A
			Tag int    `json:"renamed"`
		}
		m3("local", local{Inner: Inner{A: 1, X: 2}, A: "top", Tag: 5})
		u3("Outer", `{"A":1,"X":2,"B":3}`, func() any { return &Outer{} }, func(v any) string {
			o := v.(*Outer)
			return fmt.Sprintf("%+v %d", *o.Inner, o.B)
		})
		u3("Rec", `{"Name":"r","Kids":[{"Name":"k"}]}`, func() any { return &Rec{} }, func(v any) string {
			r := v.(*Rec)
			return r.Name + ">" + r.Kids[0].Name
		})
		u3("Ints", `[1,2]`, func() any { return &Ints{} }, showV)
		u3("Arr", `[1,2,3]`, func() any { return &Arr{} }, showV)
		u3("MapN", `{"a":1}`, func() any { return &MapN{} }, showV)
		u3("**NInt", `5`, func() any { var p *NInt; return &p }, func(v any) string { return fmt.Sprint(**v.(**NInt)) })
		u3("non-pointer", `1`, func() any { return NInt(0) }, showV)
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead assert x.(T), T unnamed
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead panic x.(T) without ok, T unnamed: *runtime.TypeAssertionError
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead reflect method set
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead panic == of two interfaces: comparing uncomparable type
	probe("Marshalers", func() {
		mo := json.WithMarshalers(json.JoinMarshalers(
			json.MarshalFunc(func(n NInt) ([]byte, error) { return []byte(fmt.Sprintf(`"n%d"`, n)), nil }),
			json.MarshalToFunc(func(enc *jsontext.Encoder, z Zeroer) error {
				return enc.WriteToken(jsontext.String(fmt.Sprint("zeroer:", z.IsZero())))
			}),
			json.MarshalFunc(func(p *Named) ([]byte, error) { return []byte(`"named ptr"`), nil }),
		))
		// twice: the second time takes the function from the cache
		for i := 0; i < 2; i++ {
			m3(fmt.Sprint(i, " values"), []any{NInt(1), Z{1}, &PZ{2}, Named("x"), Ints{3}}, mo)
			n := Named("y")
			m3(fmt.Sprint(i, " *Named"), &n, mo)
		}
		uo := json.WithUnmarshalers(json.JoinUnmarshalers(
			json.UnmarshalFunc(func(b []byte, n *NInt) error { *n = NInt(len(b)); return nil }),
			json.UnmarshalFromFunc(func(dec *jsontext.Decoder, z Zeroer) error {
				_, err := dec.ReadValue()
				if p, ok := z.(*PZ); ok {
					p.N = 99
				}
				return err
			}),
		))
		for i := 0; i < 2; i++ {
			u3(fmt.Sprint(i, " Ints"), `["abc",1]`, func() any { return &Ints{} }, showV, uo)
			u3(fmt.Sprint(i, " PZ"), `{"N":1}`, func() any { return &PZ{} }, showV, uo)
		}
	})
	// row: encoding/json/v2 Marshal,MarshalEncode,MarshalWrite,Unmarshal,UnmarshalDecode,UnmarshalRead reflect comparability
	probe("mapkey", func() {
		m3("map[any]", map[any]NInt{Named("b"): 2, "a": 1}, json.Deterministic(true))
		m3("map[any] struct key", map[any]NInt{Inner{A: 1}: 1})
		uo := json.WithUnmarshalers(json.UnmarshalFunc(func(b []byte, k *any) error {
			*k = Key{L: []int{len(b)}}
			return nil
		}))
		u3("map[any]", `{"a":1,"b":2}`, func() any { return &map[any]NInt{} }, func(v any) string { return fmt.Sprint(len(*v.(*map[any]NInt))) }, uo)
		u3("map[any] legacy", `{"a":1}`, func() any { return &map[any]NInt{} }, func(v any) string { return fmt.Sprint(len(*v.(*map[any]NInt))) }, uo, jsonv1.DefaultOptionsV1())
		u3("map[any] plain", `{"a":1}`, func() any { return &map[any]NInt{} }, showV)
	})
	// row: encoding/json/v2 (*SemanticError).Error reflect *
	probe("SemanticError", func() {
		errs := []error{}
		_, err := json.Marshal(Ch(nil))
		errs = append(errs, err)
		_, err = json.Marshal(struct{ C Ch }{})
		errs = append(errs, err)
		var n NInt
		errs = append(errs, json.Unmarshal([]byte(`"x"`), &n))
		var o Outer
		errs = append(errs, json.Unmarshal([]byte(`{"B":"x"}`), &o))
		var l Ints
		errs = append(errs, json.Unmarshal([]byte(`[1,"x"]`), &l))
		errs = append(errs, json.Unmarshal([]byte(`1`), n))
		for i, e := range errs {
			var se *json.SemanticError
			out("%d: %v %v", i, errors.As(e, &se), e)
		}
		out("made: %v", (&json.SemanticError{GoType: reflect.TypeOf(Outer{}), JSONKind: '"', Err: errors.New("boom")}).Error())
		out("made ptr: %v", (&json.SemanticError{GoType: reflect.TypeOf(&Inner{}), JSONPointer: "/a/0"}).Error())
		out("made action: %v", (&json.SemanticError{GoType: reflect.TypeOf(Ints{}), ByteOffset: 3}).Error())
	})
	// row: encoding/json/v2 JoinOptions,MarshalEncode,UnmarshalDecode assert x.(encoding/json/internal/jsonopts.experimentalFormatTagSupporter)
	probe("JoinOptions", func() {
		for i, o := range []json.Options{fmtTag{on: true}, fmtTag{on: false}, json.JoinOptions(fmtTag{on: true}, json.Deterministic(true))} {
			jo := json.JoinOptions(o, jsontext.Multiline(false))
			v, ok := json.GetOption(jo, json.Deterministic)
			out("%d GetOption: %v %v", i, v, ok)
			m3(fmt.Sprint(i, " Dur"), Dur{}, jo)
			var buf bytes.Buffer
			err := json.MarshalEncode(jsontext.NewEncoder(&buf), Dur{}, o)
			out("%d MarshalEncode opt: %v", i, err)
			var d Dur
			err = json.UnmarshalDecode(jsontext.NewDecoder(strings.NewReader(`{"D":1}`)), &d, o)
			out("%d UnmarshalDecode opt: %v", i, err)
		}
	})
}
