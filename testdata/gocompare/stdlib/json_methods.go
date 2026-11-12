// encoding/json with program methods: MarshalJSON and UnmarshalJSON on
// value and pointer receivers, MarshalText and UnmarshalText (also as map
// keys), and the errors they return.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Celsius float64

func (c Celsius) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatFloat(float64(c), 'f', 1, 64) + "C")), nil
}

func (c *Celsius) UnmarshalJSON(b []byte) error {
	s, err := strconv.Unquote(string(b))
	if err != nil {
		return fmt.Errorf("celsius: %w", err)
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(s, "C"), 64)
	if err != nil {
		return err
	}
	*c = Celsius(f)
	return nil
}

type Color struct{ R, G, B uint8 }

func (c Color) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)), nil
}

func (c *Color) UnmarshalText(b []byte) error {
	if len(b) != 7 || b[0] != '#' {
		return errors.New("bad color " + string(b))
	}
	v, err := strconv.ParseUint(string(b[1:]), 16, 32)
	if err != nil {
		return err
	}
	c.R, c.G, c.B = uint8(v>>16), uint8(v>>8), uint8(v)
	return nil
}

// PtrOnly has MarshalJSON on the pointer receiver only.
type PtrOnly struct{ V int }

func (p *PtrOnly) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"ptr":%d}`, p.V)), nil
}

type Failing struct{}

func (Failing) MarshalJSON() ([]byte, error) { return nil, errors.New("cannot marshal") }

type BadJSON struct{}

func (BadJSON) MarshalJSON() ([]byte, error) { return []byte(`{bad`), nil }

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
		return fmt.Errorf("unknown level %q", b)
	}
	return nil
}

type Reading struct {
	Temp    Celsius          `json:"temp"`
	TempP   *Celsius         `json:"temp_p"`
	Col     Color            `json:"col"`
	Lvl     Level            `json:"lvl"`
	ByColor map[Color]int    `json:"by_color"`
	ByLevel map[Level]string `json:"by_level"`
	ByInt   map[int]string   `json:"by_int"`
	ByU8    map[uint8]bool   `json:"by_u8"`
	Ptr     PtrOnly          `json:"ptr"`
	PtrP    *PtrOnly         `json:"ptr_p"`
	Temps   []Celsius        `json:"temps"`
}

func main() {
	c := Celsius(-3.25)
	r := Reading{
		Temp: 21.5, TempP: &c, Col: Color{255, 16, 1}, Lvl: 2,
		ByColor: map[Color]int{{1, 2, 3}: 1, {0, 0, 0}: 2},
		ByLevel: map[Level]string{0: "zero", 2: "two"},
		ByInt:   map[int]string{10: "ten", -2: "minus", 3: "three"},
		ByU8:    map[uint8]bool{200: true, 7: false},
		Ptr:     PtrOnly{1}, PtrP: &PtrOnly{2},
		Temps: []Celsius{1, 2.25},
	}
	b, err := json.Marshal(r)
	fmt.Println(string(b), err)
	b2, err := json.Marshal(&r)
	fmt.Println(string(b2), err)
	var back Reading
	err = json.Unmarshal(b, &back)
	fmt.Println(err)
	fmt.Printf("%v %v %v %v %v %v %v %v %v\n", back.Temp, *back.TempP, back.Col, back.Lvl, back.ByColor, back.ByLevel, back.ByInt, back.ByU8, back.Temps)

	for _, v := range []any{Failing{}, BadJSON{}, []any{1, Failing{}}, map[string]any{"k": BadJSON{}}, Celsius(1), &c, Color{}, Level(1), map[Level]int{1: 5}} {
		b, err := json.Marshal(v)
		fmt.Printf("%s | %v\n", b, err)
	}
	inputs := []string{
		`{"temp":"10C"}`,
		`{"temp":12}`,
		`{"temp":"abcC"}`,
		`{"col":"#zzzzzz"}`,
		`{"col":"red"}`,
		`{"lvl":"mid"}`,
		`{"lvl":"huge"}`,
		`{"by_level":{"high":"h","low":"l"}}`,
		`{"by_level":{"bad":"x"}}`,
		`{"by_int":{"12":"a","x":"b"}}`,
		`{"by_u8":{"300":true}}`,
		`{"by_color":{"#010203":4}}`,
		`{"temps":["1C","2C",3]}`,
	}
	for _, in := range inputs {
		var r Reading
		err := json.Unmarshal([]byte(in), &r)
		fmt.Printf("%-40s => temp=%v col=%v lvl=%v bl=%v bi=%v bu=%v bc=%v ts=%v err=%v\n", in, r.Temp, r.Col, r.Lvl, r.ByLevel, r.ByInt, r.ByU8, r.ByColor, r.Temps, err)
		var se *json.UnmarshalTypeError
		if errors.As(err, &se) {
			fmt.Println("  type error:", se.Value, se.Type, se.Field, se.Offset)
		}
	}
}
