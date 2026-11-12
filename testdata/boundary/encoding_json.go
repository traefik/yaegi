// Native boundary of encoding/json: program types given to json (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
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

// VM has a MarshalJSON method with a value receiver.
type VM struct{ N int }

func (v VM) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"vm:%d"`, v.N)), nil }

// PM has a MarshalJSON method with a pointer receiver.
type PM struct{ N int }

func (p *PM) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"pm:%d"`, p.N)), nil }

// GM is a generic type with a MarshalJSON method (value receiver).
type GM[T any] struct{ V T }

func (g GM[T]) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"gm:%v"`, g.V)), nil }

// TM has MarshalText (value receiver) and UnmarshalText (pointer receiver).
type TM struct{ S string }

func (t TM) MarshalText() ([]byte, error) { return []byte("tm:" + t.S), nil }
func (t *TM) UnmarshalText(b []byte) error {
	t.S = "un:" + string(b)
	return nil
}

// UM has an UnmarshalJSON method (pointer receiver).
type UM struct{ Raw string }

func (u *UM) UnmarshalJSON(b []byte) error {
	u.Raw = "um:" + string(b)
	return nil
}

// GU is a generic type with an UnmarshalJSON method.
type GU[T any] struct{ Raw string }

func (g *GU[T]) UnmarshalJSON(b []byte) error {
	var z T
	g.Raw = fmt.Sprintf("gu[%T]:%s", z, b)
	return nil
}

// CSV is a slice type with an UnmarshalJSON method.
type CSV []string

func (c *CSV) UnmarshalJSON(b []byte) error {
	*c = CSV{"csv", string(b)}
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

// KS is a map key type with text methods.
type KS string

func (k KS) MarshalText() ([]byte, error) { return []byte("ks-" + string(k)), nil }
func (k *KS) UnmarshalText(b []byte) error {
	*k = KS("from-" + string(b))
	return nil
}

// KT is a struct map key with MarshalText.
type KT struct{ A, B int }

func (k KT) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("%d.%d", k.A, k.B)), nil }

// Rec is a recursive type.
type Rec struct {
	Name string
	Kids []*Rec `json:",omitempty"`
}

// Inner and PInner are embedded in Outer.
type Inner struct {
	A int
	X string `json:"x,omitempty"`
}

type PInner struct{ P int }

type Outer struct {
	Inner
	*PInner
	B  int
	In Inner
}

// Holder has fields of many forms.
type Holder struct {
	V    VM
	P    PM
	PP   *PM
	G    GM[string]
	T    TM
	N    Named
	I    NInt `json:"i,string"`
	F    NF
	OK   NBool
	Raw  NB
	Any  any
	L    []any
	M    map[string]any
	Skip int `json:"-"`
	priv int
}

// Empty has omitempty fields of each kind.
type Empty struct {
	S  Named           `json:",omitempty"`
	Sl []NInt          `json:",omitempty"`
	M  map[KS]int      `json:",omitempty"`
	Ar [0]int          `json:",omitempty"`
	P  *PM             `json:",omitempty"`
	I  any             `json:",omitempty"`
	N  NInt            `json:",omitempty"`
	F  NF              `json:",omitempty"`
	B  NBool           `json:",omitempty"`
	St struct{ A int } `json:",omitempty"`
}

// Bad has methods that fail.
type Bad struct{}

func (Bad) MarshalJSON() ([]byte, error) { return nil, errors.New("boom") }

type BadT struct{}

func (BadT) MarshalText() ([]byte, error) { return nil, errors.New("tboom") }

// Ch is a type json cannot marshal.
type Ch chan int

// Box is a named empty interface.
type Box interface{}

// Stringer is a non-empty interface.
type Stringer interface{ String() string }

// W192 is the struct of issue 192: a generic struct with a field of a
// type with a value-receiver MarshalJSON.
type W192[T any] struct {
	F VM
	V T
}

// enc3 marshals v with Marshal, MarshalIndent and Encoder.Encode.
func enc3(label string, v any) {
	b, err := json.Marshal(v)
	out("%s Marshal: %s %v", label, b, err)
	b, err = json.MarshalIndent(v, "", " ")
	out("%s MarshalIndent: %s %v", label, strings.ReplaceAll(string(b), "\n", "|"), err)
	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(v)
	out("%s Encode: %s %v", label, strings.TrimSpace(buf.String()), err)
}

// dec2 decodes data with Unmarshal and Decoder.Decode into a new target
// made by mk, and prints the target.
func dec2(label, data string, mk func() any, show func(any) string) {
	t := mk()
	err := json.Unmarshal([]byte(data), t)
	out("%s Unmarshal: %s %v", label, show(t), err)
	t = mk()
	err = json.NewDecoder(strings.NewReader(data)).Decode(t)
	out("%s Decode: %s %v", label, show(t), err)
}

func showV(v any) string { return fmt.Sprintf("%+v", reflect.ValueOf(v).Elem().Interface()) }

func main() {
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect method set
	probe("Marshal/methods", func() {
		enc3("VM", VM{1})
		enc3("*VM", &VM{2})
		enc3("PM", PM{3})
		enc3("*PM", &PM{4})
		enc3("GM", GM[int]{5})
		enc3("*GM", &GM[[]int]{[]int{6}})
		enc3("TM", TM{"a"})
		enc3("*TM", &TM{"b"})
		var nilPM *PM
		enc3("nil*PM", nilPM)
		var s Stringer
		enc3("nilStringer", s)
		enc3("[]any", []any{VM{7}, &PM{8}, PM{9}, GM[string]{"g"}, TM{"t"}})
		enc3("map", map[string]any{"a": VM{10}, "b": &PM{11}, "c": TM{"c"}})
		enc3("Holder", Holder{V: VM{1}, P: PM{2}, PP: &PM{3}, G: GM[string]{"x"}, T: TM{"y"}, N: "n", I: 5, F: 1.5, OK: true, Raw: NB("hi"), Any: &PM{4}, L: []any{VM{5}}, M: map[string]any{"k": GM[int]{6}}, Skip: 9, priv: 1})
		enc3("&Holder", &Holder{P: PM{2}})
	})
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect method set
	probe("Marshal/mapkeys", func() {
		enc3("KS", map[KS]int{"b": 2, "a": 1})
		enc3("KT", map[KT]string{{1, 2}: "x", {0, 5}: "y"})
		enc3("NInt", map[NInt]bool{3: true, -1: false})
		enc3("Named", map[Named]VM{"z": {1}, "y": {2}})
	})
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect kind
	probe("Marshal/kinds", func() {
		enc3("Named", Named("s<&>"))
		enc3("NInt", NInt(-3))
		enc3("NF", NF(2.5))
		enc3("NBool", NBool(true))
		enc3("NB", NB("abc"))
		enc3("arr", [2]NInt{1, 2})
		enc3("ptrptr", func() **NInt { n := NInt(4); p := &n; return &p }())
		enc3("Rec", &Rec{Name: "root", Kids: []*Rec{{Name: "a"}, {Name: "b", Kids: []*Rec{{Name: "c"}}}}})
		enc3("Box", Box(VM{1}))
		var any1 any = GM[NInt]{2}
		enc3("&any", &any1)
	})
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect length
	probe("Marshal/omitempty", func() {
		enc3("zero", Empty{})
		enc3("full", Empty{S: "s", Sl: []NInt{1}, M: map[KS]int{"k": 1}, P: &PM{1}, I: VM{2}, N: 1, F: 1, B: true})
		enc3("emptyNonNil", Empty{Sl: []NInt{}, M: map[KS]int{}})
	})
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect type name
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect walk
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode panic map key of interface type: hash of unhashable type
	probe("Marshal/fields", func() {
		enc3("Outer", Outer{Inner: Inner{A: 1}, PInner: &PInner{P: 2}, B: 3, In: Inner{A: 4, X: "x"}})
		enc3("OuterNilP", Outer{B: 5})
		type local struct {
			Inner
			A    string // shadows Inner.A
			Tag  int    `json:"renamed"`
			Dup1 int    `json:"dup"`
			Dup2 int    `json:"dup"`
		}
		enc3("local", local{Inner: Inner{A: 1}, A: "top", Tag: 2, Dup1: 3, Dup2: 4})
		enc3("anon", struct {
			X NInt
			Y []TM
		}{1, []TM{{"p"}}})
	})
	// row: encoding/json Marshal,MarshalIndent,(*Encoder).Encode reflect method set
	// issue 192: json.Marshal(&x), x an any holding a generic struct with a
	// value-receiver MarshalJSON field.
	probe("Marshal/issue192", func() {
		var x any = W192[int]{F: VM{1}, V: 2}
		b, err := json.Marshal(&x)
		out("&any: %s %v", b, err)
		b, err = json.Marshal(x)
		out("any: %s %v", b, err)
		y := W192[string]{F: VM{3}, V: "s"}
		b, err = json.Marshal(&y)
		out("&y: %s %v", b, err)
		var z any = &W192[[]int]{F: VM{4}}
		b, err = json.Marshal(&z)
		out("&any ptr: %s %v", b, err)
	})
	// row: encoding/json (*MarshalerError).Error reflect type name
	probe("MarshalerError", func() {
		for i, v := range []any{Bad{}, &Bad{}, BadT{}, map[string]any{"x": Bad{}}, []any{BadT{}}} {
			_, err := json.Marshal(v)
			var me *json.MarshalerError
			ok := errors.As(err, &me)
			out("%d: %v %v", i, ok, err)
		}
	})
	// row: encoding/json (*UnsupportedTypeError).Error reflect type name
	probe("UnsupportedTypeError", func() {
		for i, v := range []any{Ch(nil), map[string]any{"c": make(Ch)}, struct{ F func() }{}} {
			_, err := json.Marshal(v)
			var ue *json.UnsupportedTypeError
			ok := errors.As(err, &ue)
			out("%d: %v %v", i, ok, err)
		}
		out("made: %v", (&json.UnsupportedTypeError{Type: reflect.TypeOf(Ch(nil))}).Error())
	})
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect method set
	probe("Unmarshal/methods", func() {
		dec2("UM", `{"a":1}`, func() any { return &UM{} }, showV)
		dec2("GU", `[1,2]`, func() any { return &GU[NInt]{} }, showV)
		dec2("CSV", `[1,2]`, func() any { return &CSV{} }, showV)
		dec2("TM", `"txt"`, func() any { return &TM{} }, showV)
		dec2("TMnum", `12`, func() any { return &TM{} }, showV)
		type S struct {
			U  UM
			PU *UM
			G  GU[string]
			T  TM
			PT *TM
			L  []UM
			M  map[string]*TM
			K  map[KS]int
			NK map[NInt]Named
		}
		dec2("struct", `{"U":true,"PU":[1],"G":"g","T":"t","PT":"pt","L":[1,"x"],"M":{"a":"m"},"K":{"k":1},"NK":{"7":"seven"}}`, func() any { return &S{} }, func(v any) string {
			s := v.(*S)
			return fmt.Sprintf("%v %v %v %v %v %v %v %v %v", s.U, *s.PU, s.G, s.T, *s.PT, s.L, *s.M["a"], s.K, s.NK)
		})
	})
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect kind
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect length
	probe("Unmarshal/kinds", func() {
		dec2("arr2short", `[1,2,3]`, func() any { return &[2]NInt{} }, showV)
		dec2("arr2long", `[1]`, func() any { a := [2]NInt{8, 9}; return &a }, showV)
		dec2("slice", `["a","b"]`, func() any { return &[]Named{} }, showV)
		dec2("slicegrow", `[1,2,3,4,5,6,7,8,9]`, func() any { s := make([]NInt, 1, 2); return &s }, showV)
		dec2("any", `[1,"a",{"b":null}]`, func() any { var a any; return &a }, showV)
		dec2("Box", `[true]`, func() any { var b Box; return &b }, showV)
		dec2("Stringer", `[1]`, func() any { var s Stringer; return &s }, showV)
		dec2("NF", `1.5`, func() any { var f NF; return &f }, showV)
		dec2("NBool", `true`, func() any { var b NBool; return &b }, showV)
		dec2("NB", `"aGk="`, func() any { var b NB; return &b }, func(v any) string { return string(*v.(*NB)) })
		dec2("ptrptr", `5`, func() any { var p **NInt; return &p }, func(v any) string { return fmt.Sprint(**(*v.(***NInt))) })
		dec2("Rec", `{"Name":"r","Kids":[{"Name":"k","Kids":[{"Name":"g"}]}]}`, func() any { return &Rec{} }, func(v any) string {
			r := v.(*Rec)
			return r.Name + ">" + r.Kids[0].Name + ">" + r.Kids[0].Kids[0].Name
		})
		dec2("mapNInt", `{"1":2,"-3":4}`, func() any { m := map[NInt]NInt{}; return &m }, showV)
	})
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect comparability
	probe("Unmarshal/inany", func() {
		var c any = &Outer{}
		err := json.Unmarshal([]byte(`{"A":7,"B":8}`), &c)
		o := c.(*Outer)
		out("any holding *Outer: %d %d %v", o.A, o.B, err)
		var d any = &UM{}
		err = json.Unmarshal([]byte(`"q"`), &d)
		out("any holding *UM: %v %v", *d.(*UM), err)
		var e any = UM{}
		err = json.Unmarshal([]byte(`"q"`), &e)
		out("any holding UM: %v %v", e, err)
	})
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect type name
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect walk
	// row: encoding/json Unmarshal,(*Decoder).Decode panic map key of interface type: hash of unhashable type
	// row: encoding/json (*UnmarshalTypeError).Error reflect type name
	probe("Unmarshal/errors", func() {
		dec2("Outer", `{"A":1,"P":2,"B":"x","In":{"A":"y"}}`, func() any { return &Outer{} }, func(v any) string {
			o := v.(*Outer)
			p := -1
			if o.PInner != nil {
				p = o.P
			}
			return fmt.Sprintf("%d %d %d %+v", o.A, p, o.B, o.In)
		})
		dec2("NInt", `"s"`, func() any { var n NInt; return &n }, showV)
		dec2("Holder", `{"i":"x"}`, func() any { return &Holder{} }, func(v any) string { return fmt.Sprint(v.(*Holder).I) })
		out("made: %v", (&json.UnmarshalTypeError{Value: "number", Type: reflect.TypeOf(NInt(0)), Struct: "Outer", Field: "B"}).Error())
	})
	// row: encoding/json (*InvalidUnmarshalError).Error reflect *
	probe("InvalidUnmarshalError", func() {
		var nilUM *UM
		for i, v := range []any{UM{}, nilUM, nil, Named("x"), map[string]int{}} {
			err := json.Unmarshal([]byte(`{}`), v)
			var ie *json.InvalidUnmarshalError
			ok := errors.As(err, &ie)
			out("%d: %v %v", i, ok, err)
		}
	})
	// row: encoding/json (*UnmarshalFieldError).Error reflect type name
	probe("UnmarshalFieldError", func() {
		e := &json.UnmarshalFieldError{Key: "k", Type: reflect.TypeOf(Outer{}), Field: reflect.TypeOf(Outer{}).Field(2)}
		out("%v", e.Error())
	})
	// row: encoding/json (*Decoder).Token reflect *
	// row: encoding/json (*Decoder).Token panic map key of interface type: hash of unhashable type
	probe("Token", func() {
		d := json.NewDecoder(strings.NewReader(`{"a":[1,"s",true,null,{"b":2.5}]} [3]`))
		d.UseNumber()
		for {
			t, err := d.Token()
			if err != nil {
				out("end: %v", err)
				break
			}
			out("%T %v more=%v", t, t, d.More())
		}
		d = json.NewDecoder(strings.NewReader(`[{"A":1},{"A":2}]`))
		t, _ := d.Token()
		out("open %v", t)
		for d.More() {
			var in Inner
			err := d.Decode(&in)
			out("elem %+v %v", in, err)
		}
	})
	// The self pointer probe is the last one: it may end the program with a
	// stack overflow. Go 1.27 itself overflows the stack there (json v1 on
	// top of json/v2), so the probe runs only with Go 1.26.
	// row: encoding/json Unmarshal,(*Decoder).Decode reflect comparability
	probe("Unmarshal/selfptr", func() {
		if !strings.HasPrefix(runtime.Version(), "go1.26") {
			out("skipped")
			return
		}
		var b Box
		b = &b
		err := json.Unmarshal([]byte(`[1]`), &b)
		out("Box: %v %v", b, err)
		var a any
		a = &a
		err = json.NewDecoder(strings.NewReader(`{"x":1}`)).Decode(&a)
		out("any: %v %v", a, err)
	})
}
