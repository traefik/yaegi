package main

// Interpreted values with a MarshalText or MarshalJSON method, in a []any
// or a map[string]any given to json.Marshal, in default mode (the call goes
// to the AOT copy of json, vm/aotroute.go). json v2 (Go 1.27) copies the
// top-level value, and the value of each map entry, into a new variable
// before it looks at it: a native copy there lost the methods, and the
// values were encoded as structs ({"R":0,"G":1,"B":2}).

import (
	"encoding/json"
	"fmt"
	"os"
)

type Color struct{ R, G, B uint8 }

func (c Color) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)), nil
}

type Level int

func (l Level) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"L%d"`, int(l))), nil }

type Box struct{ V any }

func show(v any) {
	b, err := json.Marshal(v)
	fmt.Println(string(b), err)
}

func main() {
	c := Color{0, 1, 2}
	show([]any{c, Level(3), 4})
	show(map[string]any{"c": c, "l": Level(5), "n": 6})
	show(map[string]any{"in": []any{c, map[string]any{"c": c}}})
	show(Box{[]any{c}})
	show([]Color{c})
	show(map[Color]any{c: c})
	b, err := json.MarshalIndent(map[string]any{"c": c}, "", " ")
	fmt.Println(string(b), err)
	enc := json.NewEncoder(os.Stdout)
	fmt.Println(enc.Encode([]any{c, Level(7)}))
	enc.SetIndent(">", "  ")
	fmt.Println(enc.Encode(map[string]any{"c": c}))
}
