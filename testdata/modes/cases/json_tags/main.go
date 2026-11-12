// json: omitempty, string option, "-", RawMessage, interfaces holding
// interpreted types, decoding into any.
package main

import (
	"encoding/json"
	"fmt"
)

type Shape interface{ Area() float64 }

type Square struct {
	Side float64 `json:"side"`
}

func (s Square) Area() float64 { return s.Side * s.Side }

type Circle struct {
	R float64 `json:"r"`
}

func (c *Circle) Area() float64 { return 3 * c.R * c.R }

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Opt struct {
	A     int              `json:"a,omitempty"`
	B     string           `json:"b,omitempty"`
	C     []int            `json:"c,omitempty"`
	D     map[string]int   `json:"d,omitempty"`
	E     *int             `json:"e,omitempty"`
	F     int64            `json:"f,string"`
	G     bool             `json:"g,string"`
	H     int              `json:"-"`
	I     int              `json:"-,"`
	J     Square           `json:"j,omitzero"`
	K     any              `json:"k,omitempty"`
	L     Shape            `json:"l"`
	M     []Shape          `json:"m"`
	N     map[string]Shape `json:"n"`
	O     *json.RawMessage `json:"o,omitempty"`
	plain int
}

func main() {
	b, err := json.Marshal(Opt{F: 12, G: true, H: 1, I: 2, L: Square{2}, M: []Shape{Square{1}, &Circle{2}}, N: map[string]Shape{"c": &Circle{1}}, plain: 3})
	fmt.Println(string(b), err)

	var o Opt
	err = json.Unmarshal([]byte(`{"f":"77","g":"false","-":5,"j":{"side":3}}`), &o)
	fmt.Println(err, o.F, o.G, o.I, o.J.Side)

	// Into an interface field holding a pointer to an interpreted type.
	o2 := Opt{L: &Circle{}}
	err = json.Unmarshal([]byte(`{"l":{"r":4}}`), &o2)
	fmt.Println(err, o2.L.Area())
	// Into an interface field holding a non-pointer: an error.
	o3 := Opt{L: Square{}}
	err = json.Unmarshal([]byte(`{"l":{"side":4}}`), &o3)
	fmt.Println(err)

	// RawMessage, two-step decoding.
	var envs []Envelope
	err = json.Unmarshal([]byte(`[{"type":"sq","payload":{"side":5}},{"type":"ci","payload":{"r":1}}]`), &envs)
	fmt.Println(err)
	for _, e := range envs {
		var s Shape
		switch e.Type {
		case "sq":
			var q Square
			_ = json.Unmarshal(e.Payload, &q)
			s = q
		case "ci":
			c := &Circle{}
			_ = json.Unmarshal(e.Payload, c)
			s = c
		}
		fmt.Printf("%s %s %v\n", e.Type, e.Payload, s.Area())
	}
	b, err = json.Marshal(envs)
	fmt.Println(string(b), err)

	// Decoding into any.
	var v any
	_ = json.Unmarshal([]byte(`{"a":[1,"x",true,null,{"b":2.5}]}`), &v)
	fmt.Printf("%v %T\n", v, v.(map[string]any)["a"].([]any)[0])

	var u Opt
	err = json.Unmarshal([]byte(`{"a":"str"}`), &u)
	fmt.Println(err)
}
