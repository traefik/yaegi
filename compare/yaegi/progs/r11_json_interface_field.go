package main

import (
	"encoding/json"
	"fmt"
)

type Shape interface{ Area() float64 }

type Sq struct{ S float64 }

func (s Sq) Area() float64 { return s.S * s.S }

type Doc struct {
	Shapes []Shape `json:"shapes"`
	Any    any     `json:"any"`
}

func main() {
	b, err := json.Marshal(Doc{Shapes: []Shape{Sq{2}}, Any: Sq{3}})
	fmt.Println(string(b), err)
	var raw json.RawMessage
	json.Unmarshal([]byte(`{"a":1}`), &raw)
	fmt.Println(string(raw))
	dec := json.NewDecoder(nil)
	_ = dec
}
