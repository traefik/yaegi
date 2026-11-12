package main

// encoding/json with values that hold a type of package json itself
// (json.RawMessage, json.Number) and a type with methods inside them. In
// default mode, a call whose value held a json type stayed native, so the
// methods of the inner types were ignored with no error: a golang-jwt
// style claims struct with a json.RawMessage field lost its "exp", a
// nested UnmarshalText was not called. The call now goes to the AOT copy
// of json, which takes the host's json.Number as its own (sameValType):
// a json.Number in an `any` is written as a number, not a string.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type NumericDate struct{ time.Time }

func (d NumericDate) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(d.Unix(), 10)), nil
}

func (d *NumericDate) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	f, err := n.Float64()
	if err != nil {
		return err
	}
	d.Time = time.Unix(int64(f), 0).UTC()
	return nil
}

type Level int

func (l *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "low":
		*l = 1
	case "high":
		*l = 2
	default:
		return fmt.Errorf("bad level %q", b)
	}
	return nil
}

func (l Level) MarshalText() ([]byte, error) {
	return []byte([]string{"none", "low", "high"}[l]), nil
}

type Claims struct {
	Sub    string          `json:"sub"`
	Exp    *NumericDate    `json:"exp,omitempty"`
	Extra  json.RawMessage `json:"extra,omitempty"`
	Count  json.Number     `json:"count,omitempty"`
	Levels map[Level]int   `json:"levels,omitempty"`
	Level  Level           `json:"level"`
}

type Wrapped struct {
	Claims
	Any any `json:"any"`
}

func main() {
	in := `{"sub":"bob","exp":1700000000,"extra":{"a":[1,2]},"count":12.5,"levels":{"high":3},"level":"low"}`
	var c Claims
	err := json.Unmarshal([]byte(in), &c)
	fmt.Println(err, c.Sub, c.Exp != nil && c.Exp.Year() == 2023, string(c.Extra), c.Count, c.Levels, c.Level)
	b, err := json.Marshal(c)
	fmt.Println(string(b), err)

	var d Claims
	dec := json.NewDecoder(bytes.NewBufferString(in))
	dec.UseNumber()
	err = dec.Decode(&d)
	fmt.Println(err, d.Exp != nil && d.Exp.Unix() == 1700000000, d.Level)

	var out bytes.Buffer
	err = json.NewEncoder(&out).Encode(&d)
	fmt.Print(out.String(), err, "\n")

	// a json.Number in an interface, next to a type with methods
	w := Wrapped{Claims: c, Any: json.Number("5")}
	b, err = json.Marshal(w)
	fmt.Println(string(b), err)
	b, err = json.Marshal([]any{json.Number("7"), Level(2), json.RawMessage(`{"r":1}`)})
	fmt.Println(string(b), err)

	var ww Wrapped
	dec = json.NewDecoder(bytes.NewBufferString(`{"sub":"x","exp":1600000000,"any":3,"level":"high"}`))
	dec.UseNumber()
	err = dec.Decode(&ww)
	n, isNumber := ww.Any.(json.Number)
	fmt.Println(err, ww.Exp.Unix(), n, isNumber, ww.Level)

	// errors keep the native types
	err = json.Unmarshal([]byte(`{"count":"x","level":"low"}`), &c)
	fmt.Println(err)
	err = json.Unmarshal([]byte(`{"level":"bad","extra":1}`), &c)
	fmt.Println(err)
}
