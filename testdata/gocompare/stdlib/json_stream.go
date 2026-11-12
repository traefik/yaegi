// encoding/json Decoder and Encoder with program types: streams of values,
// Token, More, UseNumber, json.Number, json.RawMessage fields, interfaces
// holding program types, SetIndent and SetEscapeHTML.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Shape interface{ Area() float64 }

type Rect struct{ W, H float64 }

func (r Rect) Area() float64 { return r.W * r.H }

type Circle struct{ R float64 }

func (c *Circle) Area() float64 { return 3 * c.R * c.R }

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Doc struct {
	Shapes []Shape          `json:"shapes"`
	Any    any              `json:"any"`
	Num    json.Number      `json:"num"`
	Raw    json.RawMessage  `json:"raw"`
	RawP   *json.RawMessage `json:"raw_p,omitempty"`
}

func decodeShape(e Envelope) (Shape, error) {
	switch e.Type {
	case "rect":
		var r Rect
		err := json.Unmarshal(e.Data, &r)
		return r, err
	case "circle":
		c := &Circle{}
		err := json.Unmarshal(e.Data, c)
		return c, err
	}
	return nil, fmt.Errorf("unknown type %q", e.Type)
}

func main() {
	raw := json.RawMessage(`{"x": [1, 2]}`)
	d := Doc{Shapes: []Shape{Rect{2, 3}, &Circle{1}, nil}, Any: Rect{1, 1}, Num: "12.50", Raw: json.RawMessage(`[true,  null]`), RawP: &raw}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	fmt.Println(enc.Encode(d))
	enc.SetIndent("", " ")
	fmt.Println(enc.Encode(map[string]Shape{"b": Rect{1, 2}, "a": &Circle{2}}))
	enc.SetIndent("", "")
	fmt.Println(enc.Encode("<a&b>"))
	enc.SetEscapeHTML(false)
	fmt.Println(enc.Encode("<a&b>"))
	fmt.Print(buf.String())

	// Envelopes decoded through RawMessage.
	stream := `{"type":"rect","data":{"W":2,"H":5}}
{"type":"circle","data":{"R":2}}
{"type":"tri","data":{}}
{"type":"rect","data":{"W":"x"}}`
	dec := json.NewDecoder(strings.NewReader(stream))
	for {
		var e Envelope
		err := dec.Decode(&e)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("decode:", err)
			break
		}
		s, err := decodeShape(e)
		if err != nil {
			fmt.Println("shape error:", err)
			continue
		}
		fmt.Printf("%T %v %.2f raw=%s\n", s, s, s.Area(), e.Data)
	}

	// UseNumber and any.
	dec = json.NewDecoder(strings.NewReader(`{"big": 12345678901234567890, "f": 1.5, "i": 3, "l": [1, 2.0]} {"n": -0}`))
	dec.UseNumber()
	for dec.More() {
		var m map[string]any
		fmt.Println(dec.Decode(&m))
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s %T %v\n", k, m[k], m[k])
			if n, ok := m[k].(json.Number); ok {
				i, ierr := n.Int64()
				f, ferr := n.Float64()
				fmt.Println("   ", n.String(), i, ierr, f, ferr)
			}
		}
	}
	var nd struct{ N json.Number }
	fmt.Println(json.Unmarshal([]byte(`{"N": "7e2"}`), &nd), nd.N)
	fmt.Println(json.Unmarshal([]byte(`{"N": "abc"}`), &nd), nd.N)

	// Tokens.
	dec = json.NewDecoder(strings.NewReader(`{"a": [1, "s", true, null, {"b": 2.5}], "c": {}}`))
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("token error:", err)
			break
		}
		fmt.Printf("%T %v more=%v off=%d\n", t, t, dec.More(), dec.InputOffset())
	}
	// Decode into an interface that holds a pointer to a program type.
	var target any = &Rect{}
	fmt.Println(json.Unmarshal([]byte(`{"W": 9, "H": 2}`), &target), target.(*Rect).Area())
	var target2 any = Rect{}
	fmt.Println(json.Unmarshal([]byte(`{"W": 9}`), &target2))
	fmt.Printf("%T %v\n", target2, target2)
	var sh Shape
	err := json.Unmarshal([]byte(`{"W": 1}`), &sh)
	fmt.Println(err)
	// Buffered after an error.
	dec = json.NewDecoder(strings.NewReader(`{"W":1} xyz`))
	var r Rect
	fmt.Println(dec.Decode(&r), r, dec.Decode(&r))
	rest, _ := io.ReadAll(dec.Buffered())
	fmt.Printf("%q\n", rest)
}
