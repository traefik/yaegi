// json: error values that name interpreted types.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

type Invalid struct{}

func (Invalid) MarshalJSON() ([]byte, error) { return []byte(`{"x":`), nil }

func main() {
	var r ResponseError
	err := json.Unmarshal([]byte(`{"msg": 12}`), &r)
	fmt.Println(err)
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		fmt.Println(ute.Field, ute.Value, ute.Type, ute.Type.Name())
	}
	var w Wrapper
	err = json.Unmarshal([]byte(`{"err": "x"}`), &w)
	fmt.Println(err)
	err = json.Unmarshal([]byte(`[1]`), &r)
	fmt.Println(err)

	_, err = json.Marshal(Bad{})
	fmt.Println(err)
	var me *json.MarshalerError
	fmt.Println(errors.As(err, &me))
	_, err = json.Marshal(Invalid{})
	fmt.Println(err)
	_, err = json.Marshal(map[string]any{"c": make(chan int)})
	fmt.Println(err)

	err = json.Unmarshal([]byte(`{`), &r)
	var se *json.SyntaxError
	fmt.Println(err, errors.As(err, &se))
	err = json.Unmarshal([]byte(`{}`), r)
	fmt.Println(err)
}
