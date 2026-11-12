// json: json.Number values (Decoder.UseNumber, json.Number literals) given
// back to Marshal and to an Encoder, inside maps and slices of interfaces,
// as JWT claims are decoded and encoded again.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type Claims map[string]any

func main() {
	dec := json.NewDecoder(strings.NewReader(`{"id":7,"f":1.5,"s":"x","n":{"a":2},"l":[3]}`))
	dec.UseNumber()
	var m map[string]any
	fmt.Println(dec.Decode(&m))
	fmt.Printf("%T %v\n", m["id"], m["id"])
	b, err := json.Marshal(m)
	fmt.Println(string(b), err)
	var buf bytes.Buffer
	fmt.Println(json.NewEncoder(&buf).Encode(m), buf.String())

	dec = json.NewDecoder(strings.NewReader(`{"exp":1700000000,"aud":["a"]}`))
	dec.UseNumber()
	var c Claims
	fmt.Println(dec.Decode(&c))
	b, err = json.Marshal(c)
	fmt.Println(string(b), err)
	b, err = json.Marshal([]any{json.Number("3"), Claims{"x": json.Number("4")}})
	fmt.Println(string(b), err)
}
