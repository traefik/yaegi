package main

// Native decoders called as func values (in a struct field, a map, a slice,
// an interface): the destination is the pointer of the program, never the
// String wrapper of the VM. The resty client of gocloak keeps
// json.Unmarshal in a struct field.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
)

type Result struct {
	Active *bool `json:"active"`
}

func (r *Result) String() string { return "result" }

type Item struct {
	Name string `xml:"name"`
}

func (i *Item) String() string { return "item" }

type Count int

func (c *Count) String() string { return "count" }

type Pair struct{ A, B uint16 }

func (p Pair) String() string { return "pair" }

type client struct {
	JSONUnmarshal func(data []byte, v interface{}) error
	XMLUnmarshal  func(data []byte, v any) error
	Sscan         func(str string, a ...any) (int, error)
}

func active(r Result) bool { return r.Active != nil && *r.Active }

func main() {
	data := []byte(`{"active":true}`)

	c := &client{
		JSONUnmarshal: json.Unmarshal,
		XMLUnmarshal:  xml.Unmarshal,
		Sscan:         fmt.Sscan,
	}
	var r Result
	err := c.JSONUnmarshal(data, &r)
	fmt.Println("struct field:", err, active(r))

	m := map[string]func([]byte, any) error{"json": json.Unmarshal}
	var r2 Result
	err = m["json"](data, &r2)
	fmt.Println("map:", err, active(r2))

	s := []func([]byte, any) error{json.Unmarshal}
	var r3 Result
	err = s[0](data, &r3)
	fmt.Println("slice:", err, active(r3))

	var i any = json.Unmarshal
	var r4 Result
	err = i.(func([]byte, any) error)(data, &r4)
	fmt.Println("interface:", err, active(r4))

	var it Item
	err = c.XMLUnmarshal([]byte(`<item><name>x</name></item>`), &it)
	fmt.Println("xml:", err, it.Name)

	var n Count
	k, err := c.Sscan("42", &n)
	fmt.Println("sscan:", k, err, int(n))

	write := []func(io.Writer, binary.ByteOrder, any) error{binary.Write}
	var buf bytes.Buffer
	err = write[0](&buf, binary.BigEndian, Pair{1, 2})
	fmt.Println("binary.Write:", err, buf.Bytes())

	// Printers keep the String method.
	p := fmt.Sprint
	fmt.Println("print:", p(&r))
}
