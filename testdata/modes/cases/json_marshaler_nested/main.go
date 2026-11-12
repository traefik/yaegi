// json: MarshalJSON on nested fields, pointer fields, slice elements and
// map values.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Upper string

func (u Upper) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToUpper(string(u)))
}

type Money struct{ Cents int64 }

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%d.%02d EUR"`, m.Cents/100, m.Cents%100)), nil
}

type PtrOnly struct{ N int }

func (p *PtrOnly) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"ptr-%d"`, p.N)), nil
}

type Inner struct {
	Name  Upper  `json:"name"`
	Price Money  `json:"price"`
	PP    *Money `json:"pp"`
}

type Outer struct {
	In     Inner            `json:"in"`
	InPtr  *Inner           `json:"in_ptr"`
	List   []Money          `json:"list"`
	ByName map[string]Money `json:"by_name"`
	Any    any              `json:"any"`
	Addr   PtrOnly          `json:"addr"`
	AddrP  *PtrOnly         `json:"addr_p"`
	Arr    [2]Upper         `json:"arr"`
}

func main() {
	m := Money{1234}
	o := Outer{
		In:     Inner{Name: "abc", Price: Money{100}, PP: &m},
		InPtr:  &Inner{Name: "def", Price: Money{5}},
		List:   []Money{{1}, {250}},
		ByName: map[string]Money{"a": {99}, "b": {100000}},
		Any:    Upper("in any"),
		Addr:   PtrOnly{1},
		AddrP:  &PtrOnly{2},
		Arr:    [2]Upper{"x", "y"},
	}
	b, err := json.Marshal(o)
	fmt.Println(string(b), err)
	// Addressable: through a pointer, Addr gets its pointer method.
	b, err = json.Marshal(&o)
	fmt.Println(string(b), err)
	b, err = json.Marshal([]any{Money{7}, &PtrOnly{3}, map[string]any{"u": Upper("q")}})
	fmt.Println(string(b), err)
}
