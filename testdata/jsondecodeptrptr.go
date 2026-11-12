package main

// json.Decode into a **T, when *T has an UnmarshalJSON method: json
// allocates the *T and calls the method (anthropic-sdk-go decodes its
// responses with Decode(&res), res a *Message), or stores nil for null.
// T has a field with an embedded unexported type (respjson.Field), which
// has no native copy.

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type status int8

// Field is like anthropic-sdk-go's respjson.Field.
type Field struct {
	status
	raw string
}

func (j Field) Valid() bool { return j.status > 0 }
func (j Field) Raw() string { return j.raw }

type Message struct {
	ID   string
	Seen int
	JSON struct{ ID Field }
}

func (r *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.ID = "custom:" + raw.ID
	r.Seen++
	r.JSON.ID = Field{status: 1, raw: raw.ID}
	return nil
}

func decode(body string, into any) error {
	return json.NewDecoder(bytes.NewReader([]byte(body))).Decode(into)
}

func main() {
	var res *Message
	var into any = &res
	err := decode(`{"id":"m1"}`, into)
	fmt.Println(err, res.ID, res.Seen, res.JSON.ID.status, res.JSON.ID.raw)

	old := res
	err = json.Unmarshal([]byte(`{"id":"m2"}`), &res)
	fmt.Println(err, res == old, res.ID, res.Seen)

	err = decode(`null`, &res)
	fmt.Println(err, res == nil)

	err = decode(`{"id": 5}`, &res)
	fmt.Println(err != nil, res != nil)
}
