// json-iterator on interpreted types. Its reflect2 dependency reaches the
// runtime with //go:linkname (mapiterinit, mapiternext, unsafe_New,
// unsafe_NewArray, typedmemmove, typedslicecopy, mapassign, mapaccess):
// the VM emulates these functions (vm/reflect2.go). Also a slice in an
// interface, read through its data word (docs/UNSAFE.md).
package main

import (
	"fmt"
	"sort"

	jsoniter "github.com/json-iterator/go"
)

type Point struct {
	X, Y int
}

type Doc struct {
	Name   string           `json:"name"`
	Tags   []string         `json:"tags"`
	Counts map[string]int   `json:"counts"`
	Points map[string]Point `json:"points"`
	List   []Point          `json:"list"`
	Ptr    *Point           `json:"ptr"`
	Any    any              `json:"any"`
	Bytes  []byte           `json:"bytes"`
}

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func show(v any) {
	b, err := json.Marshal(v)
	fmt.Println(string(b), err)
}

func main() {
	// map iteration (mapiterinit, mapiternext), keys sorted by the config
	show(map[string]int{"b": 2, "a": 1, "c": 3})
	show(map[string]Point{"p": {1, 2}, "q": {3, 4}})
	show(map[int]string{3: "c", 1: "a"})
	show(map[string]int(nil))
	show(map[string]int{})

	// interfaces holding slices, maps and structs
	show([]any{1, "x", []any{true, nil}, map[string]any{"k": []int{1, 2}}})
	show(Point{5, 6})

	d := Doc{
		Name:   "d",
		Tags:   []string{"a", "b"},
		Counts: map[string]int{"x": 1, "y": 2},
		Points: map[string]Point{"o": {}},
		List:   []Point{{1, 1}, {2, 2}},
		Ptr:    &Point{7, 8},
		Any:    []any{"s", 1.5},
		Bytes:  []byte("hi"),
	}
	show(d)

	// decoding: new values (unsafe_New), slices that grow (unsafe_NewArray,
	// typedslicecopy), map entries (makemap, mapassign)
	var back Doc
	b, _ := json.Marshal(d)
	err := json.Unmarshal(b, &back)
	fmt.Println(err, back.Name, back.Tags, back.Counts, back.Points, back.List, *back.Ptr, back.Any, string(back.Bytes))

	var many []Point
	err = json.Unmarshal([]byte(`[{"X":1},{"X":2},{"X":3},{"X":4},{"X":5},{"X":6},{"X":7}]`), &many)
	fmt.Println(err, len(many), many)

	var mm map[string][]int
	err = json.Unmarshal([]byte(`{"a":[1,2],"b":[3]}`), &mm)
	keys := make([]string, 0, len(mm))
	for k := range mm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println(err, k, mm[k])
	}

	// an existing map: decoding adds to it
	m := map[string]Point{"old": {9, 9}}
	err = json.Unmarshal([]byte(`{"new":{"X":1,"Y":2}}`), &m)
	fmt.Println(err, len(m), m["old"], m["new"])

	var anyv any
	err = json.Unmarshal([]byte(`{"a":[1,"two",{"b":null}]}`), &anyv)
	fmt.Println(err, anyv)

	// Get walks a document
	fmt.Println(jsoniter.Get(b, "counts", "y").ToInt(), jsoniter.Get(b, "list", 1, "X").ToInt())

	// errors
	err = json.Unmarshal([]byte(`{"counts":{"x":"no"}}`), &back)
	fmt.Println(err != nil)
}
