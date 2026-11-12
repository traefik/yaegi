package main

// A pointer method of a native type, called through an interface on an
// interpreted variable, writes back into it: json.RawMessage's
// UnmarshalJSON does *m = append((*m)[0:0], data...), and the new slice
// header must reach the variable (anthropic-sdk-go's apijson calls
// v.Addr().Interface().(json.Unmarshaler).UnmarshalJSON).

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
)

type Block struct {
	Name  string
	Input json.RawMessage
}

func main() {
	var b Block
	var u json.Unmarshaler = &b.Input
	err := u.UnmarshalJSON([]byte(`{"a":1}`))
	fmt.Println(err, string(b.Input))

	// a shorter value: the old bytes stay in the array, the length changes
	err = u.UnmarshalJSON([]byte(`[2]`))
	fmt.Println(err, string(b.Input), len(b.Input))

	// through reflect, as apijson
	bs := make([]Block, 2)
	v := reflect.ValueOf(bs).Index(1).Field(1)
	err = v.Addr().Interface().(json.Unmarshaler).UnmarshalJSON([]byte(`"x"`))
	fmt.Println(err, string(bs[1].Input), bs[0].Input == nil)

	// a struct pointer through an interface
	var x url.URL
	var bu interface{ UnmarshalBinary([]byte) error } = &x
	fmt.Println(bu.UnmarshalBinary([]byte("http://h/p")), x.Host, x.Path)
}
