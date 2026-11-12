// json: embedded structs, embedded pointers, promoted marshal methods.
package main

import (
	"encoding/json"
	"fmt"
)

type Base struct {
	ID   int    `json:"id"`
	Kind string `json:"kind,omitempty"`
}

type Meta struct {
	Tags []string `json:"tags"`
}

type Item struct {
	Base
	*Meta
	Name string `json:"name"`
}

type Stamp struct{ Unix int64 }

func (s Stamp) MarshalJSON() ([]byte, error) { return []byte(fmt.Sprintf(`"T%d"`, s.Unix)), nil }

// Event embeds a type with MarshalJSON: the method is promoted, so the
// whole Event marshals as the Stamp.
type Event struct {
	Stamp
	What string
}

type named struct{ Hidden int }

type WithUnexported struct {
	named
	Visible int
}

type Conflict struct {
	Base
	ID string `json:"id"`
}

func main() {
	b, err := json.Marshal(Item{Base: Base{ID: 1}, Meta: &Meta{Tags: []string{"a"}}, Name: "n"})
	fmt.Println(string(b), err)
	b, err = json.Marshal(Item{Base: Base{ID: 2, Kind: "k"}, Name: "nil meta"})
	fmt.Println(string(b), err)
	b, err = json.Marshal(Event{Stamp{42}, "boot"})
	fmt.Println(string(b), err)
	b, err = json.Marshal(WithUnexported{named{5}, 6})
	fmt.Println(string(b), err)
	b, err = json.Marshal(Conflict{Base{ID: 3}, "outer"})
	fmt.Println(string(b), err)

	var it Item
	err = json.Unmarshal([]byte(`{"id":9,"kind":"z","tags":["t1","t2"],"name":"x"}`), &it)
	fmt.Println(err, it.ID, it.Kind, it.Meta != nil && len(it.Tags) == 2, it.Name)
	var w WithUnexported
	err = json.Unmarshal([]byte(`{"Hidden":7,"Visible":8}`), &w)
	fmt.Println(err, w.Hidden, w.Visible)
}
