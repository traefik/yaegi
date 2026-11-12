package main

// encoding/json called with interpreted types in default mode goes to the
// AOT copy of json (vm/aotroute.go). Two things were still wrong there:
//
//   - The errors of the copy come back with the native error types, whose
//     field Type was the native type of the value: the messages named an
//     unnamed struct ("of type struct { Message string ... }") where gc
//     names main.ResponseError. The field is now the native type with the
//     name of the interpreted one, and a MarshalerError is a plain
//     *json.MarshalerError (a type assertion works).
//   - A value that holds a json.RawMessage stayed native, where the
//     methods of the nested interpreted types are ignored and an interface
//     field is decoded without error. The copy uses a native RawMessage
//     through its methods: such a value now goes to the copy.
//
// (From the reflect battery of the modes challenge, cases json_errors and
// json_tags.)

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

type ResponseError struct {
	Message string `json:"msg"`
	Code    int    `json:"code"`
}

type Wrapper struct {
	Err ResponseError `json:"err"`
}

type Bad struct{}

func (Bad) MarshalJSON() ([]byte, error) { return nil, errors.New("bad marshal") }

type Shape interface{ Area() float64 }

type Square struct {
	Side float64 `json:"side"`
}

func (s Square) Area() float64 { return s.Side * s.Side }

type Level int

func (l Level) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote("L" + strconv.Itoa(int(l)))), nil
}

type Envelope struct {
	Type    string           `json:"type"`
	Level   Level            `json:"level"`
	Shape   Shape            `json:"shape"`
	Payload json.RawMessage  `json:"payload"`
	Opt     *json.RawMessage `json:"opt,omitempty"`
}

func main() {
	var r ResponseError
	err := json.Unmarshal([]byte(`{"msg": 12}`), &r)
	fmt.Println(err)
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		fmt.Println(ute.Field, ute.Value, ute.Type, ute.Type.Name(), ute.Type.Kind())
	}
	var w Wrapper
	err = json.Unmarshal([]byte(`{"err": "x"}`), &w)
	fmt.Println(err)
	if errors.As(err, &ute) {
		fmt.Println(ute.Struct, ute.Field, ute.Type, ute.Type.Name(), ute.Type.Kind())
		// the Type is the program's reflect.Type of the interpreted type
		fmt.Println(ute.Type == reflect.TypeOf(r), ute.Type.PkgPath(), reflect.New(ute.Type).Elem().Interface(), reflect.Zero(ute.Type).Interface())
	}
	fmt.Println(json.Unmarshal([]byte(`[1]`), &r))
	fmt.Println(json.Unmarshal([]byte(`{}`), r))
	var ie *json.InvalidUnmarshalError
	fmt.Println(errors.As(json.Unmarshal([]byte(`{}`), r), &ie), ie.Type)

	_, err = json.Marshal(Bad{})
	fmt.Println(err)
	me, ok := err.(*json.MarshalerError)
	fmt.Println(ok, me.Type, errors.Unwrap(err))
	_, err = json.Marshal([]any{1, Bad{}})
	fmt.Println(err, errors.As(err, &me), me.Type)

	// a RawMessage next to interpreted types
	raw := json.RawMessage(`{"x":[1,2]}`)
	b, err := json.Marshal(Envelope{Type: "sq", Level: 3, Shape: Square{2}, Payload: raw, Opt: &raw})
	fmt.Println(string(b), err)
	e := Envelope{Shape: Square{}}
	err = json.Unmarshal([]byte(`{"type":"t","shape":{"side":4},"payload":{"a":1},"opt":[2]}`), &e)
	fmt.Println(err, e.Type, string(e.Payload), e.Opt != nil && string(*e.Opt) == "[2]")
	var e2 Envelope
	err = json.Unmarshal([]byte(`{"type":"t","payload":[true],"level":"x"}`), &e2)
	fmt.Println(err, string(e2.Payload))
}
