//go:build goexperiment.jsonv2

package main

// The API of encoding/json/v2 and encoding/json/jsontext (Go 1.27) with the
// types of the program: their methods (MarshalJSONTo,
// UnmarshalJSONFrom, and those of json v1 and encoding), the tags, the
// options, the errors, the streams, Value and Pointer. In default mode the
// VM runs them with the AOT copies (stdAOTTriggers), which see the types of
// the program; native json/v2 sees a native copy of the values.

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"
)

// level has the methods of json/v2 (MarshalerTo, UnmarshalerFrom).
type level int

func (l level) MarshalJSONTo(enc *jsontext.Encoder) error {
	return enc.WriteToken(jsontext.String(strings.Repeat("*", int(l))))
}

func (l *level) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	*l = level(len(tok.String()))
	return nil
}

// color has the methods of json v1 (Marshaler, Unmarshaler), which v2
// calls too.
type color struct{ R, G, B uint8 }

func (c color) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `"#%02x%02x%02x"`, c.R, c.G, c.B), nil
}

func (c *color) UnmarshalJSON(b []byte) error {
	_, err := fmt.Sscanf(string(b), `"#%02x%02x%02x"`, &c.R, &c.G, &c.B)
	return err
}

// name has a text method.
type name string

func (n name) MarshalText() ([]byte, error) { return []byte(strings.ToUpper(string(n))), nil }

func (n *name) UnmarshalText(b []byte) error {
	*n = name(strings.ToLower(string(b)))
	return nil
}

type inner struct {
	Note string `json:"note"`
}

type record struct {
	ID       int               `json:"id"`
	Name     name              `json:"name"`
	Level    level             `json:"level"`
	Color    color             `json:"color"`
	Tags     []string          `json:"tags,omitempty"`
	Zero     time.Time         `json:"zero,omitzero"`
	When     time.Time         `json:"when"`
	Count    int64             `json:"count,string"`
	Inline   inner             `json:",inline"`
	Extra    jsontext.Value    `json:",unknown"`
	ByName   map[name]level    `json:"by_name"`
	Any      any               `json:"any"`
	Ptr      *record           `json:"ptr,omitempty"`
	Raw      jsontext.Value    `json:"raw"`
	Nums     map[string]string `json:"nums,omitempty"`
	unexport int
}

func main() {
	r := record{
		ID: 1, Name: "bob", Level: 3, Color: color{1, 2, 255},
		When: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Count: 42,
		Inline: inner{"hi"}, ByName: map[name]level{"x": 1, "y": 2},
		Any: []any{1.5, "s", map[string]any{"k": true}}, Ptr: &record{ID: 2},
		Raw: jsontext.Value(`{"b":1,"a":[1,2]}`), unexport: 7,
	}
	out, err := json.Marshal(r, json.Deterministic(true))
	fmt.Println(string(out), err)

	var back record
	err = json.Unmarshal(append(out[:len(out)-1:len(out)-1], `,"other":1}`...), &back)
	fmt.Println(err)
	fmt.Println(back.ID, back.Name, back.Level, back.Color, back.When.Format(time.DateOnly), back.Count)
	fmt.Println(back.Inline.Note, string(back.Extra), back.ByName, back.Any, back.Ptr.ID, string(back.Raw))

	// options
	out, err = json.Marshal(struct {
		S []int
		M map[string]int
		Z int `json:",omitzero"`
	}{}, json.FormatNilSliceAsNull(true), json.FormatNilMapAsNull(true))
	fmt.Println(string(out), err)
	out, err = json.Marshal(inner{}, json.OmitZeroStructFields(true))
	fmt.Println(string(out), err)
	// (the text of a SemanticError says "cannot" or "unable to" at random)
	err = json.Unmarshal([]byte(`{"note":"a","bad":1}`), new(inner), json.RejectUnknownMembers(true))
	fmt.Println(errors.Is(err, json.ErrUnknownName), strings.Contains(fmt.Sprint(err), `unknown object member name "bad"`))
	var in inner
	err = json.Unmarshal([]byte(`{"NOTE":"b"}`), &in, json.MatchCaseInsensitiveNames(true))
	fmt.Println(in, err)
	out, err = json.Marshal(map[string]int{"b": 1, "a": 2}, json.Deterministic(true), jsontext.WithIndent("  "), jsontext.WithIndentPrefix("\t"))
	fmt.Println(string(out), err)
	out, err = json.Marshal("<&>", jsontext.EscapeForHTML(true))
	fmt.Println(string(out), err)
	out, err = json.Marshal(3.0, json.StringifyNumbers(true))
	fmt.Println(string(out), err)

	// errors
	var se *json.SemanticError
	err = json.Unmarshal([]byte(`{"id":"x"}`), new(record))
	fmt.Println(errors.As(err, &se), se != nil && se.JSONPointer == "/id", se != nil && se.GoType.String() == "int")
	var syn *jsontext.SyntacticError
	err = json.Unmarshal([]byte(`{"id":1,}`), new(record))
	fmt.Println(errors.As(err, &syn), syn != nil && syn.ByteOffset == 8)
	_, err = json.Marshal(make(chan int))
	fmt.Println(err != nil)
	err = json.Unmarshal([]byte(`{"a":1,"a":2}`), new(map[string]int))
	fmt.Println(err != nil, errors.Is(err, jsontext.ErrDuplicateName))
	err = json.Unmarshal([]byte(`{"a":1,"a":2}`), new(map[string]int), jsontext.AllowDuplicateNames(true))
	fmt.Println(err)
	err = json.Unmarshal([]byte(`{"nope":1}`), new(inner), json.RejectUnknownMembers(true))
	fmt.Println(errors.Is(err, json.ErrUnknownName))

	// streams
	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf, jsontext.Multiline(true))
	enc.WriteToken(jsontext.BeginArray)
	for _, x := range []any{level(2), color{3, 4, 5}, name("n"), nil} {
		if err := json.MarshalEncode(enc, x); err != nil {
			fmt.Println(err)
		}
	}
	enc.WriteToken(jsontext.Int(-3))
	enc.WriteToken(jsontext.Uint(3))
	enc.WriteToken(jsontext.Float(0.5))
	enc.WriteToken(jsontext.Bool(true))
	enc.WriteToken(jsontext.Null)
	enc.WriteValue(jsontext.Value(`{"z":0}`))
	fmt.Println(enc.StackDepth(), enc.StackPointer(), enc.OutputOffset())
	enc.WriteToken(jsontext.EndArray)
	fmt.Print(buf.String())

	dec := jsontext.NewDecoder(strings.NewReader(`{"a":[1,"x",{"b":null}],"c":true} {"d":2}`))
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			fmt.Println("end:", err)
			break
		}
		fmt.Println(tok.Kind(), tok.String(), dec.StackDepth(), dec.StackPointer(), dec.InputOffset())
	}
	dec = jsontext.NewDecoder(strings.NewReader(`[{"x":1},{"x":2}] [3]`))
	dec.ReadToken()
	for dec.PeekKind() != ']' {
		var v struct{ X level }
		if err := json.UnmarshalDecode(dec, &v); err != nil {
			fmt.Println(err)
			break
		}
		fmt.Println(v.X)
	}
	dec.ReadToken()
	v, err := dec.ReadValue()
	fmt.Println(string(v), err, dec.UnreadBuffer())

	// Value methods
	val := jsontext.Value(` { "b" : 1 , "a" : [ 2.0 , 1e2 ] } `)
	fmt.Println(val.IsValid(), val.Kind())
	c := val.Clone()
	fmt.Println(c.Compact(), string(c))
	c = val.Clone()
	fmt.Println(c.Indent(jsontext.WithIndent("\t")), string(c))
	c = val.Clone()
	fmt.Println(c.Canonicalize(), string(c))
	fmt.Println(c.Format(jsontext.CanonicalizeRawFloats(true)), string(c))

	// Pointer
	p := jsontext.Pointer("/a/0/b~1c")
	for tok := range p.Tokens() {
		fmt.Print(tok, " ")
	}
	fmt.Println(p.LastToken(), p.Parent(), p.AppendToken("x~y"), p.Contains("/a"), p.IsValid())

	// Marshal/Unmarshal with Write/Read
	buf.Reset()
	err = json.MarshalWrite(&buf, map[name]color{"k": {9, 9, 9}}, json.Deterministic(true))
	fmt.Println(buf.String(), err)
	var mc map[name]color
	err = json.UnmarshalRead(&buf, &mc)
	fmt.Println(mc, err)

	// Options
	opts := json.JoinOptions(json.DefaultOptionsV2(), json.Deterministic(true), jsontext.WithIndent(" "))
	d, ok := json.GetOption(opts, json.Deterministic)
	ind, ok2 := json.GetOption(opts, jsontext.WithIndent)
	pre, ok3 := json.GetOption(opts, jsontext.WithIndentPrefix)
	fmt.Printf("%v %v %q %v %q %v\n", d, ok, ind, ok2, pre, ok3)
	out, err = json.Marshal([]int{1}, opts)
	fmt.Println(string(out), err)
}
