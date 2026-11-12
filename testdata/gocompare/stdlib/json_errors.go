// encoding/json errors with program types: syntax errors with offsets, type
// errors with field paths, unknown fields with DisallowUnknownFields,
// invalid unmarshal targets and unsupported values, printed with %v.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

type Inner struct {
	N    int     `json:"n"`
	F    float32 `json:"f"`
	U    uint8   `json:"u"`
	List []int   `json:"list"`
}

type Outer struct {
	Name  string           `json:"name"`
	In    Inner            `json:"in"`
	Ins   []Inner          `json:"ins"`
	ByKey map[string]Inner `json:"by_key"`
	P     *Inner           `json:"p"`
	B     bool             `json:"b"`
}

func report(label string, err error) {
	fmt.Printf("%s: %v\n", label, err)
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	var ie *json.InvalidUnmarshalError
	var ue *json.UnsupportedValueError
	var ut *json.UnsupportedTypeError
	var me *json.MarshalerError
	switch {
	case errors.As(err, &se):
		fmt.Println("  syntax at", se.Offset)
	case errors.As(err, &te):
		fmt.Printf("  type: value=%s type=%v field=%s struct=%s offset=%d\n", te.Value, te.Type, te.Field, te.Struct, te.Offset)
	case errors.As(err, &ie):
		fmt.Println("  invalid:", ie.Type)
	case errors.As(err, &ue):
		fmt.Println("  unsupported value:", ue.Str)
	case errors.As(err, &ut):
		fmt.Println("  unsupported type:", ut.Type)
	case errors.As(err, &me):
		fmt.Println("  marshaler:", me.Type)
	}
}

type Boom struct{}

func (*Boom) MarshalJSON() ([]byte, error) { return nil, errors.New("boom") }

func main() {
	inputs := []string{
		`{"name": 1}`,
		`{"in": {"n": "x"}}`,
		`{"in": {"n": 1.5}}`,
		`{"in": {"u": 256}}`,
		`{"in": {"u": -1}}`,
		`{"in": {"f": 1e40}}`,
		`{"ins": [{"n": 1}, {"list": [1, "two"]}]}`,
		`{"by_key": {"a": {"n": true}}}`,
		`{"p": {"list": {}}}`,
		`{"b": "true"}`,
		`{"name": "x",}`,
		`{"name" "x"}`,
		`[1, 2]`,
		`{"in": {"n": 99999999999999999999}}`,
		`{"name": "\u00zz"}`,
		``,
		`  {"name": "ok"} trailing`,
		`nul`,
		`{"name":"a"}{"name":"b"}`,
	}
	for _, in := range inputs {
		var o Outer
		err := json.Unmarshal([]byte(in), &o)
		report(fmt.Sprintf("%q", in), err)
		fmt.Println("  partial:", o.Name, o.In.N, o.In.U, len(o.Ins), o.P != nil)
	}
	// DisallowUnknownFields with the Decoder.
	for _, in := range []string{`{"name":"x","extra":1}`, `{"in":{"n":1,"zz":2}}`, `{"ins":[{"q":1}]}`, `{"name":"ok"}`} {
		d := json.NewDecoder(strings.NewReader(in))
		d.DisallowUnknownFields()
		var o Outer
		report("strict "+in, d.Decode(&o))
	}
	// Invalid targets.
	var o Outer
	report("nil", json.Unmarshal([]byte(`{}`), nil))
	report("non-pointer", json.Unmarshal([]byte(`{}`), o))
	var np *Outer
	report("nil pointer", json.Unmarshal([]byte(`{}`), np))
	// Marshal errors.
	_, err := json.Marshal(math.Inf(1))
	report("inf", err)
	_, err = json.Marshal(map[string]any{"c": make(chan int)})
	report("chan", err)
	_, err = json.Marshal(struct{ F func() }{})
	report("func", err)
	_, err = json.Marshal([]any{&Boom{}})
	report("boom", err)
	_, err = json.Marshal(map[struct{ K int }]int{{}: 1})
	report("struct key", err)
	// Valid / Compact / Indent / HTMLEscape.
	for _, s := range []string{`{"a":1}`, `{a:1}`, `[1,2,]`, `" <&>"`} {
		fmt.Println(s, json.Valid([]byte(s)))
		var buf bytes.Buffer
		err := json.Compact(&buf, []byte(s))
		fmt.Println("  compact:", buf.String(), err)
		buf.Reset()
		err = json.Indent(&buf, []byte(s), "", "\t")
		fmt.Printf("  indent: %q %v\n", buf.String(), err)
		buf.Reset()
		json.HTMLEscape(&buf, []byte(s))
		fmt.Println("  html:", buf.String())
	}
}
