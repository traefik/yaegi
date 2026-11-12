// json: MarshalText and UnmarshalText on values, fields and map keys.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Color int

var names = []string{"red", "green", "blue"}

func (c Color) MarshalText() ([]byte, error) { return []byte(names[c]), nil }

func (c *Color) UnmarshalText(b []byte) error {
	for i, n := range names {
		if n == string(b) {
			*c = Color(i)
			return nil
		}
	}
	return fmt.Errorf("bad color %q", b)
}

type Key struct{ A, B string }

func (k Key) MarshalText() ([]byte, error) { return []byte(k.A + "/" + k.B), nil }

func (k *Key) UnmarshalText(b []byte) error {
	a, bb, _ := strings.Cut(string(b), "/")
	k.A, k.B = a, bb
	return nil
}

type Paint struct {
	Main   Color          `json:"main"`
	Others []Color        `json:"others"`
	ByKey  map[Key]int    `json:"by_key"`
	ByCol  map[Color]bool `json:"by_col"`
	Ptr    *Color         `json:"ptr"`
}

func main() {
	g := Color(1)
	p := Paint{Main: 2, Others: []Color{0, 1}, ByKey: map[Key]int{{"x", "y"}: 1, {"a", "b"}: 2}, ByCol: map[Color]bool{0: true, 2: false}, Ptr: &g}
	b, err := json.Marshal(p)
	fmt.Println(string(b), err)

	var q Paint
	err = json.Unmarshal(b, &q)
	fmt.Println(err, q.Main, q.Others, q.ByKey[Key{"a", "b"}], q.ByCol[0], q.Ptr != nil && *q.Ptr == 1)

	err = json.Unmarshal([]byte(`{"main":"pink"}`), &q)
	fmt.Println(err)

	b, err = json.Marshal(Color(0))
	fmt.Println(string(b), err)
	b, err = json.Marshal(map[Key]string{{"k", "v"}: "z"})
	fmt.Println(string(b), err)
}
