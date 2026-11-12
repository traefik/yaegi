// encoding/gob decodes a []any of the program, and calls GobDecode of a
// type of the program for one element. In -aot mode the AOT copy of gob
// writes the slice through a shadow of the VM cell (vm/twin_value.go,
// shadowRef). The callback suspends the shadows and resumes them after:
// the resume made a new native slice, and the elements that gob decoded
// after the callback went to the old one (nil in the program).
// A shadow whose cell the callback did not change now
// keeps its native value.
//
//aot:lib encoding/gob
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

type GE struct{ N int }

func (g GE) GobEncode() ([]byte, error) { return []byte{byte(g.N)}, nil }
func (g *GE) GobDecode(b []byte) error  { g.N = int(b[0]) * 10; return nil }

type Circle struct{ R int }

type Named string

func main() {
	gob.Register(GE{})
	gob.Register(Circle{})
	gob.Register(Named(""))
	gob.Register(map[Named]int{})
	var buf bytes.Buffer
	in := []any{1, GE{1}, "s", Circle{5}, GE{2}, Named("n"), map[Named]int{"x": 2}}
	err := gob.NewEncoder(&buf).Encode(in)
	var got []any
	err2 := gob.NewDecoder(&buf).Decode(&got)
	fmt.Println(err, err2, len(got))
	for i, g := range got {
		fmt.Printf("%d: %T %v\n", i, g, g)
	}

	// the same in a struct field and in a map value
	type box struct {
		Items []any
		ByKey map[string]any
	}
	buf.Reset()
	err = gob.NewEncoder(&buf).Encode(box{[]any{GE{3}, "after"}, map[string]any{"k": GE{4}}})
	var b box
	err2 = gob.NewDecoder(&buf).Decode(&b)
	fmt.Println(err, err2, b.Items, b.ByKey)

	// The shadow keeps its native value, and also its conversion: the
	// decoded values reach the same VM map and array, so an alias sees them.
	buf.Reset()
	err = gob.NewEncoder(&buf).Encode(map[string]any{"a": GE{1}, "b": "x"})
	mp := map[string]any{}
	alias := mp
	err2 = gob.NewDecoder(&buf).Decode(&mp)
	fmt.Println(err, err2, len(mp), len(alias), alias["a"], alias["b"])

	buf.Reset()
	err = gob.NewEncoder(&buf).Encode([]any{GE{1}, "s", GE{2}})
	backing := make([]any, 3, 10)
	sl := backing
	err2 = gob.NewDecoder(&buf).Decode(&sl)
	fmt.Println(err, err2, sl, backing)
}
