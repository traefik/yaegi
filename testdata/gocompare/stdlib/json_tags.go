// encoding/json Marshal and Unmarshal of program structs with tags:
// omitempty, omitzero, string, "-", renamed fields, unexported fields,
// embedded structs and embedded pointers, nested slices and maps.
package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type Base struct {
	ID   int    `json:"id"`
	Kind string `json:"kind,omitempty"`
}

type Extra struct {
	Note  string `json:"note"`
	Score float64
}

type Point struct{ X, Y int }

func (p Point) IsZero() bool { return p.X == 0 && p.Y == 0 }

type Item struct {
	Base
	*Extra
	Name     string `json:"name"`
	Alias    string `json:"alias,omitempty"`
	Count    int64  `json:"count,string"`
	Flag     bool   `json:"flag,string,omitempty"`
	Secret   string `json:"-"`
	Dash     string `json:"-,"`
	hidden   int
	Tags     []string           `json:"tags,omitempty"`
	Attrs    map[string]int     `json:"attrs,omitempty"`
	Pos      Point              `json:"pos,omitzero"`
	When     time.Time          `json:"when,omitzero"`
	Ptr      *int               `json:"ptr"`
	Children []Item             `json:"children,omitempty"`
	Meta     map[string]any     `json:"meta,omitempty"`
	Arr      [2]uint8           `json:"arr"`
	Bytes    []byte             `json:"bytes,omitempty"`
	Nested   struct{ A, B int } `json:"nested"`
}

func main() {
	seven := 7
	items := []Item{
		{},
		{Base: Base{ID: 1, Kind: "k"}, Name: "a", Count: 42, Flag: true, Secret: "s", Dash: "d", hidden: 3},
		{Base: Base{ID: 2}, Extra: &Extra{Note: "n", Score: 1.5}, Name: "b", Tags: []string{"x", "y"},
			Attrs: map[string]int{"z": 1, "a": 2}, Pos: Point{1, 2}, Ptr: &seven,
			When:     time.Date(2024, 2, 29, 13, 4, 5, 0, time.UTC),
			Children: []Item{{Name: "c1"}, {Name: "c2", Count: -1}},
			Meta:     map[string]any{"f": 1.25, "s": "str", "l": []any{1, "two", nil}, "b": false},
			Arr:      [2]uint8{9, 8}, Bytes: []byte("hello"), Nested: struct{ A, B int }{3, 4}},
	}
	for i, it := range items {
		b, err := json.Marshal(it)
		fmt.Println(i, string(b), err)
		var back Item
		err = json.Unmarshal(b, &back)
		fmt.Printf("%d back: id=%d kind=%q name=%q count=%d flag=%v secret=%q dash=%q tags=%v attrs=%v pos=%v when=%v extra=%v err=%v\n",
			i, back.ID, back.Kind, back.Name, back.Count, back.Flag, back.Secret, back.Dash, back.Tags, back.Attrs, back.Pos, back.When, back.Extra != nil, err)
		if back.Extra != nil {
			fmt.Println("  extra:", *back.Extra)
		}
		if back.Ptr != nil {
			fmt.Println("  ptr:", *back.Ptr)
		}
		fmt.Println("  children:", len(back.Children), "meta:", back.Meta, "arr:", back.Arr, "bytes:", string(back.Bytes), "nested:", back.Nested)
	}
	b, err := json.MarshalIndent(items[2], ">", "  ")
	fmt.Println(string(b), err)

	var it Item
	err = json.Unmarshal([]byte(`{"id":5,"note":"via embedded ptr","Score":3,"-":"dash","count":"12","flag":"true","name":"N","unknown":1}`), &it)
	fmt.Println(it.ID, it.Extra != nil, it.Name, it.Count, it.Flag, it.Dash, err)
	if it.Extra != nil {
		fmt.Println(it.Note, it.Score)
	}
	// case-insensitive match of keys
	err = json.Unmarshal([]byte(`{"NAME":"upper","ID":9,"Kind":"kk"}`), &it)
	fmt.Println(it.Name, it.ID, it.Kind, err)

	type Anon struct {
		A int `json:"a"`
		B int `json:"b,omitempty"`
		C *Base
		D any
		E []Point
	}
	for _, a := range []Anon{{}, {A: 1, B: 2, C: &Base{ID: 3}, D: Point{4, 5}, E: []Point{{6, 7}}}} {
		b, err := json.Marshal(a)
		fmt.Println(string(b), err)
		var back Anon
		fmt.Println(json.Unmarshal(b, &back), back.A, back.B, back.C != nil, back.D, back.E)
	}
}
