// encoding/json with generic program types and embedding rules: generic
// structs and slices, field name conflicts between embedded structs (the
// shallower or tagged one wins, ambiguous ones are dropped), embedded
// interfaces and non-struct types, anonymous struct fields, and
// unexported embedded structs with exported fields. (json.Marshal of a
// pointer to an interface that holds a generic struct is left out: it
// differs in yaegi v2 today.)
package main

import (
	"encoding/json"
	"fmt"
)

type Page[T any] struct {
	Items []T  `json:"items"`
	Next  *int `json:"next,omitempty"`
	Total int  `json:"total"`
}

type Pair[K comparable, V any] struct {
	Key K `json:"k"`
	Val V `json:"v"`
}

type Opt[T any] struct {
	Set bool
	V   T
}

func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if !o.Set {
		return []byte("null"), nil
	}
	return json.Marshal(o.V)
}

func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*o = Opt[T]{}
		return nil
	}
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

type A struct {
	Name string
	X    int
	Dup  string
}

type B struct {
	Name string `json:"Name"`
	Y    int
	Dup  string
}

type C struct {
	Deep A
}

type Conflict struct {
	A
	B
	Z int
}

type Shallow struct {
	A
	Name string
}

type inner struct {
	Exported string
	hidden   string
}

type Stringish string

type Embeds struct {
	inner
	Stringish
	*A `json:"a_ptr"`
	fmt.Stringer
	Anon struct {
		P int `json:"p"`
	} `json:"anon"`
}

type Tree[T any] struct {
	Val  T          `json:"val"`
	Kids []*Tree[T] `json:"kids,omitempty"`
}

func (t *Tree[T]) Walk(f func(T)) {
	f(t.Val)
	for _, k := range t.Kids {
		k.Walk(f)
	}
}

// show prints a value with pointers followed (fmt would print addresses).
func show(v any) string {
	switch v := v.(type) {
	case Page[Pair[string, int]]:
		next := "nil"
		if v.Next != nil {
			next = fmt.Sprint(*v.Next)
		}
		return fmt.Sprintf("items=%+v next=%s total=%d", v.Items, next, v.Total)
	case Embeds:
		a := "nil"
		if v.A != nil {
			a = fmt.Sprintf("%+v", *v.A)
		}
		return fmt.Sprintf("exported=%q hidden=%q stringish=%q a=%s stringer=%v anon=%d", v.Exported, v.hidden, v.Stringish, a, v.Stringer == nil, v.Anon.P)
	}
	return fmt.Sprintf("%+v", v)
}

func roundTrip[T any](label string, v T) {
	b, err := json.Marshal(v)
	fmt.Printf("%s: %s %v\n", label, b, err)
	var back T
	err = json.Unmarshal(b, &back)
	fmt.Printf("  back: %s %v\n", show(back), err)
}

func main() {
	n := 2
	roundTrip("page", Page[Pair[string, int]]{Items: []Pair[string, int]{{"a", 1}, {"b", 2}}, Next: &n, Total: 5})
	roundTrip("page-empty", Page[float64]{})
	roundTrip("pairs", []Pair[int, []string]{{1, []string{"x"}}, {2, nil}})
	roundTrip("opt", []Opt[int]{{true, 3}, {}, {true, 0}})
	roundTrip("optstruct", struct {
		A Opt[string] `json:"a"`
		B Opt[Pair[string, bool]]
	}{Opt[string]{true, "s"}, Opt[Pair[string, bool]]{true, Pair[string, bool]{"k", true}}})
	roundTrip("conflict", Conflict{A{"an", 1, "ad"}, B{"bn", 2, "bd"}, 3})
	roundTrip("shallow", Shallow{A{"inner", 1, "d"}, "outer"})
	roundTrip("embeds", Embeds{inner: inner{"exp", "hid"}, Stringish: "str", A: &A{Name: "ptr"}})
	roundTrip("embeds-nil", Embeds{})
	var e Embeds
	fmt.Println(json.Unmarshal([]byte(`{"Exported":"e","Stringish":"s","a_ptr":{"Name":"n","X":4},"anon":{"p":9}}`), &e), e.Exported, e.Stringish, e.A != nil, e.Anon.P)
	if e.A != nil {
		fmt.Println(*e.A)
	}
	var c Conflict
	fmt.Println(json.Unmarshal([]byte(`{"Name":"x","X":1,"Y":2,"Dup":"d","Z":3}`), &c), c)
	tree := &Tree[string]{"root", []*Tree[string]{{"a", nil}, {"b", []*Tree[string]{{"b1", nil}}}}}
	b, _ := json.Marshal(tree)
	fmt.Println(string(b))
	var back Tree[string]
	fmt.Println(json.Unmarshal(b, &back))
	back.Walk(func(s string) { fmt.Print(s, " ") })
	fmt.Println()
	var ti Tree[int]
	fmt.Println(json.Unmarshal(b, &ti))
	m := map[string]Page[int]{"p": {Items: []int{1}}}
	b, err := json.MarshalIndent(m, "", "\t")
	fmt.Println(string(b), err)
	var anyv any = Pair[string, Opt[int]]{"x", Opt[int]{true, 1}}
	b, err = json.Marshal(anyv)
	fmt.Println(string(b), err)
}
