// github.com/json-iterator/go: the standard library compatible config and
// the fastest config on interpreted types, with Marshaler, Unmarshaler,
// TextMarshaler, embedded structs and interfaces.
package main

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

type Upper string

func (u Upper) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strings.ToUpper(string(u)) + `"`), nil
}

type Lower string

func (l *Lower) UnmarshalJSON(b []byte) error {
	*l = Lower(strings.ToLower(strings.Trim(string(b), `"`)))
	return nil
}

type Key int

func (k Key) MarshalText() ([]byte, error) { return []byte(fmt.Sprintf("k%d", int(k))), nil }

type Base struct {
	ID int `json:"id"`
}

type Doc struct {
	Base
	Title Upper          `json:"title"`
	Low   Lower          `json:"low"`
	Keys  map[Key]string `json:"keys"`
	Opt   string         `json:"opt,omitempty"`
	Any   any            `json:"any"`
	List  []Upper        `json:"list"`
	Ptr   *Base          `json:"ptr"`
	skip  int
}

func main() {
	for _, api := range []jsoniter.API{jsoniter.ConfigCompatibleWithStandardLibrary, jsoniter.ConfigFastest} {
		d := Doc{Base: Base{1}, Title: "hello", Keys: map[Key]string{2: "two"}, Any: Base{5}, List: []Upper{"a"}, Ptr: &Base{9}}
		b, err := api.Marshal(d)
		fmt.Println(string(b), err)
		var back Doc
		err = api.Unmarshal([]byte(`{"id":3,"title":"t","low":"MiXeD","any":{"a":1},"list":["x"],"ptr":{"id":4}}`), &back)
		fmt.Println(err, back.ID, back.Title, back.Low, back.Any, back.List, back.Ptr.ID)
		s, err := api.MarshalToString([]any{Upper("x"), &d.Base, map[string]Upper{"m": "v"}})
		fmt.Println(s, err)
		fmt.Println(api.Get([]byte(`{"a":{"b":[1,2,3]}}`), "a", "b", 1).ToInt())
	}
}
