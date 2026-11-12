package main

// encoding/json called with interpreted types in default mode: the methods
// of interpreted types (MarshalJSON, UnmarshalJSON, MarshalText,
// UnmarshalText) are called also when the value is nested, as in Go.
// Native json sees a native copy of the underlying type, which has no
// methods: it printed "level":2 for a nested Level. The calls now go to the
// AOT copy of json (aotroute.go), also for values that hold a named type of
// package json itself (RawMessage, Number: see jsonpkgtypes.go).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Level int

func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("L" + strconv.Itoa(int(l)))), nil
}

func (l *Level) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "L"))
	*l = Level(n)
	return err
}

type Color struct{ R, G, B uint8 }

func (c Color) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)), nil
}

func (c *Color) UnmarshalText(b []byte) error {
	_, err := fmt.Sscanf(string(b), "#%02x%02x%02x", &c.R, &c.G, &c.B)
	return err
}

type Item struct {
	Name   string            `json:"name"`
	Level  Level             `json:"level"`
	Levels []Level           `json:"levels,omitempty"`
	Color  Color             `json:"color"`
	ByCol  map[Color]int     `json:"by_color,omitempty"`
	Extra  map[string]any    `json:"extra,omitempty"`
	Ptr    *Level            `json:"ptr,omitempty"`
	Inner  *Item             `json:"inner,omitempty"`
	Pair   Pair[string, int] `json:"pair"`
	hidden int
}

// WithRaw holds a type of package json: it stays native.
type WithRaw struct {
	Name string          `json:"name"`
	Raw  json.RawMessage `json:"raw,omitempty"`
}

type Pair[K comparable, V any] struct {
	Key K `json:"k"`
	Val V `json:"v"`
}

type Bad struct{}

func (Bad) MarshalJSON() ([]byte, error) { return nil, fmt.Errorf("bad value") }

func show(v any) {
	b, err := json.Marshal(v)
	fmt.Println(string(b), err)
}

func main() {
	lv := Level(7)
	it := Item{
		Name: "a", Level: 2, Levels: []Level{1, 3}, Color: Color{255, 0, 16},
		ByCol: map[Color]int{{1, 2, 3}: 4}, Extra: map[string]any{"l": Level(5), "n": 1},
		Ptr: &lv, Inner: &Item{Name: "in", Level: 9}, Pair: Pair[string, int]{"x", 1},
	}
	show(it)
	show([]any{Level(1), Color{0, 1, 2}, Pair[int, bool]{1, true}})
	show(map[string]Level{"a": 1})
	b, err := json.MarshalIndent(it.Inner, "", "  ")
	fmt.Println(string(b), err)

	// json types of package json itself stay native
	show(WithRaw{Name: "raw", Raw: json.RawMessage(`{"x":[1,2]}`)})
	var num struct{ N json.Number }
	fmt.Println(json.Unmarshal([]byte(`{"N":12.5}`), &num), num.N)

	// decoding into interpreted types, with their methods
	var back Item
	err = json.Unmarshal([]byte(`{"name":"b","level":"L4","levels":["L1","L2"],"color":"#0a0b0c","by_color":{"#010203":5},"ptr":"L8","inner":{"name":"c","level":"L3"},"pair":{"k":"y","v":2}}`), &back)
	fmt.Println(err, back.Name, back.Level, back.Levels, back.Color, back.ByCol, *back.Ptr, back.Inner.Name, back.Inner.Level, back.Pair)
	var anyv any
	err = json.Unmarshal([]byte(`{"a":[1,"x",true,null]}`), &anyv)
	fmt.Println(err, anyv)
	var lvs []Level
	fmt.Println(json.Unmarshal([]byte(`["L5","L6"]`), &lvs), lvs)

	// errors
	show(Bad{})
	show([]any{Bad{}})
	fmt.Println(json.Unmarshal([]byte(`{"level":"oops"}`), &back) != nil)
	var tgt Item
	fmt.Println(json.Unmarshal([]byte(`{"name":1}`), &tgt))
	// the errors of the AOT copy are given back with the native types
	var se *json.SyntaxError
	err = json.Unmarshal([]byte(`{bad`), &tgt)
	fmt.Println(errors.As(err, &se), err)
	var ute *json.UnmarshalTypeError
	err = json.Unmarshal([]byte(`{"inner":{"name":true}}`), &tgt)
	fmt.Println(errors.As(err, &ute), ute.Value, ute.Field, ute.Type, err)
	var uve *json.UnsupportedValueError
	cyc := map[string]any{}
	cyc["x"] = []any{cyc, Level(1)}
	_, err = json.Marshal(cyc)
	fmt.Println(errors.As(err, &uve), err)
	var me *json.MarshalerError
	_, err = json.Marshal([]any{Bad{}})
	fmt.Println(errors.Unwrap(err), me == nil)
	fmt.Println(err)
	fmt.Println(errors.As(err, &me), me != nil && me.Err == errors.Unwrap(err))
	fmt.Println(me.Err)

	// an Encoder (a method: it stays native)
	var buf bytes.Buffer
	fmt.Println(json.NewEncoder(&buf).Encode(Level(3)), strings.TrimSpace(buf.String()))
}
