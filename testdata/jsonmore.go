// encoding/json with interpreted types: struct tags (omitempty, omitzero,
// "-", string, renamed), embedded structs (promoted fields, conflicts,
// embedded pointers), pointers and nil pointers, maps with int and
// TextMarshaler keys, RawMessage, json.Number, custom MarshalJSON and
// UnmarshalJSON on top-level types and on fields, MarshalText, Decoder
// options (DisallowUnknownFields, UseNumber), Indent, and error values.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Level int

func (l Level) MarshalText() ([]byte, error) {
	return []byte([]string{"low", "mid", "high"}[l]), nil
}

func (l *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "low":
		*l = 0
	case "mid":
		*l = 1
	case "high":
		*l = 2
	default:
		return fmt.Errorf("bad level %q", b)
	}
	return nil
}

type Temp float64

func (t Temp) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%.1fC"`, float64(t))), nil
}

func (t *Temp) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%fC", &f); err != nil {
		return err
	}
	*t = Temp(f)
	return nil
}

type Meta struct {
	Created string `json:"created"`
	Owner   string `json:"owner,omitempty"`
}

type Base struct {
	ID   int    `json:"id"`
	Kind string `json:"kind"`
}

type Doc struct {
	Base
	*Meta
	Title    string            `json:"title"`
	Count    int               `json:"count,string"`
	Skip     string            `json:"-"`
	Dash     string            `json:"-,"`
	Empty    []string          `json:"empty,omitempty"`
	Zero     time.Time         `json:"zero,omitzero"`
	Ptr      *int              `json:"ptr"`
	Nested   *Doc              `json:"nested,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	ByLevel  map[Level]int     `json:"by_level,omitempty"`
	ByInt    map[int]bool      `json:"by_int,omitempty"`
	Raw      json.RawMessage   `json:"raw,omitempty"`
	Any      any               `json:"any"`
	Lvl      Level             `json:"lvl"`
	T        Temp              `json:"t"`
	unexport int
}

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Circle struct{ R float64 }
type Square struct{ S float64 }

type Wrapper struct {
	Items []Envelope
}

type Strict struct {
	A int `json:"a"`
}

func main() {
	n := 7
	d := Doc{
		Base:     Base{1, "doc"},
		Meta:     &Meta{Created: "today"},
		Title:    "hello <&>",
		Count:    42,
		Skip:     "hidden",
		Dash:     "dash",
		Ptr:      &n,
		Nested:   &Doc{Title: "child", Lvl: 2, T: -3.25},
		Labels:   map[string]string{"b": "2", "a": "1"},
		ByLevel:  map[Level]int{2: 20, 0: 0},
		ByInt:    map[int]bool{10: true, 2: false},
		Raw:      json.RawMessage(`{"pre":"computed"}`),
		Any:      []any{1, "x", nil, map[string]any{"k": true}},
		Lvl:      1,
		T:        21.55,
		unexport: 9,
	}
	b, err := json.Marshal(d)
	fmt.Println(string(b), err)

	var back Doc
	err = json.Unmarshal(b, &back)
	fmt.Println(err, back.ID, back.Kind, back.Meta.Created, back.Title, back.Count, back.Skip == "", back.Dash, *back.Ptr)
	fmt.Println(back.Nested.Title, back.Nested.Lvl, back.Nested.T, back.ByLevel, back.ByInt, string(back.Raw), back.Lvl, back.T)
	fmt.Printf("%v %T\n", back.Any, back.Any.([]any)[0])

	// Indent and HTML escaping off.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"z": 1, "a": []int{1, 2}, "h": "<b>"})
	fmt.Print(buf.String())

	// Polymorphic decoding with RawMessage.
	in := `{"Items":[{"type":"circle","data":{"R":2}},{"type":"square","data":{"S":3}}]}`
	var w Wrapper
	if err := json.Unmarshal([]byte(in), &w); err != nil {
		fmt.Println(err)
	}
	for _, e := range w.Items {
		switch e.Type {
		case "circle":
			var c Circle
			_ = json.Unmarshal(e.Data, &c)
			fmt.Println("circle", c.R)
		case "square":
			var s Square
			_ = json.Unmarshal(e.Data, &s)
			fmt.Println("square", s.S)
		}
	}

	// Decoder options.
	dec := json.NewDecoder(strings.NewReader(`{"a":1,"b":2}`))
	dec.DisallowUnknownFields()
	var st Strict
	fmt.Println(dec.Decode(&st), st.A)
	dec = json.NewDecoder(strings.NewReader(`{"n":12345678901234567890} {"n":1.5}`))
	dec.UseNumber()
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			fmt.Println(err)
			break
		}
		num := m["n"].(json.Number)
		_, ierr := num.Int64()
		fmt.Println(num.String(), ierr != nil)
	}

	// Errors.
	var target struct{ A int }
	err = json.Unmarshal([]byte(`{"A":"str"}`), &target)
	var ute *json.UnmarshalTypeError
	fmt.Println(errors.As(err, &ute), ute.Field, ute.Value)
	err = json.Unmarshal([]byte(`{bad`), &target)
	var se *json.SyntaxError
	fmt.Println(errors.As(err, &se), se.Offset)
	err = json.Unmarshal([]byte(`"nope"`), new(Level))
	fmt.Println(err)
	_, err = json.Marshal(map[string]any{"f": func() {}})
	fmt.Println(err)
	_, err = json.Marshal(make(chan int))
	fmt.Println(err)

	// Decode into existing values: maps are merged, slices are replaced,
	// pointers are reused.
	m := map[string]int{"keep": 1}
	_ = json.Unmarshal([]byte(`{"new":2}`), &m)
	sl := []int{9, 9, 9}
	_ = json.Unmarshal([]byte(`[1]`), &sl)
	p := &n
	_ = json.Unmarshal([]byte(`100`), &p)
	fmt.Println(m, sl, n, p == &n)

	// null.
	np := &n
	_ = json.Unmarshal([]byte(`null`), &np)
	fmt.Println(np == nil, n)

	// Valid, Compact, HTMLEscape.
	fmt.Println(json.Valid([]byte(`{"a":[1,2]}`)), json.Valid([]byte(`{a}`)))
	var cb bytes.Buffer
	_ = json.Compact(&cb, []byte("{ \"a\" : [ 1 , 2 ] }"))
	fmt.Println(cb.String())

	// Anonymous structs and slices of interpreted types.
	out, _ := json.Marshal([]struct {
		N  string `json:"name"`
		Lv Level
	}{{"a", 0}, {"b", 2}})
	fmt.Println(string(out))
	var temps []Temp
	err = json.Unmarshal([]byte(`["1.0C","-2.5C"]`), &temps)
	fmt.Println(temps, err)
	tm, _ := json.Marshal(map[string]Temp{"x": 3})
	fmt.Println(string(tm))
}
