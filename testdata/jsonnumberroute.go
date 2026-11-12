package main

// A json.Number that a json.Decoder with UseNumber gives to the program goes
// back to json.Marshal and json.Encoder as a number. In default mode both
// run on the AOT copy of json when the value holds plugin values
// (vm/aotroute.go, vm/aotroutejson.go): the Number of the copy comes to the
// program as a native json.Number (routedValType), and when it went back to
// the copy, the copy did not know that type and wrote the number as a
// string ({"id":7} became {"id":"7"}, with no error; agilezebra/
// jwt-middleware, bug G3 of the catalog run). The twin now gives the copy
// its own Number (copyRV, vm/aoterrors.go).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type Claims map[string]any

type Token struct {
	Header map[string]any `json:"header"`
	Claims Claims         `json:"claims"`
}

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

	// claims of a JWT, decoded with UseNumber and encoded again
	dec = json.NewDecoder(strings.NewReader(`{"exp":1700000000,"aud":["a"],"nbf":1.5e9}`))
	dec.UseNumber()
	var c Claims
	fmt.Println(dec.Decode(&c))
	n, ok := c["exp"].(json.Number)
	i, err := n.Int64()
	fmt.Println(ok, i, err)
	b, err = json.Marshal(c)
	fmt.Println(string(b), err)
	b, err = json.Marshal(Token{Header: map[string]any{"alg": "HS256", "v": json.Number("2")}, Claims: c})
	fmt.Println(string(b), err)
	b, err = json.MarshalIndent([]any{c["exp"], json.Number("3"), Claims{"x": json.Number("4")}}, "", " ")
	fmt.Println(string(b), err)

	// a value with a field of type json.Number (it stays native)
	b, err = json.Marshal(struct {
		C Claims
		N json.Number
	}{c, json.Number("8")})
	fmt.Println(string(b), err)

	// Token values of a Decoder
	dec = json.NewDecoder(strings.NewReader(`[1, 2.5]`))
	dec.UseNumber()
	var toks []any
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		toks = append(toks, t)
	}
	b, err = json.Marshal(map[string]any{"toks": toks})
	fmt.Println(string(b), err)
}
