// encoding/gob decodes into a map and a slice of the program, and calls
// GobDecode of a type of the program in the middle. Every alias of the map
// and of the slice array sees the decoded values, also the ones that gob
// wrote after the callback.
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
)

type GE struct{ N int }

func (g GE) GobEncode() ([]byte, error) { return []byte{byte(g.N)}, nil }

func (g *GE) GobDecode(b []byte) error {
	g.N = int(b[0]) * 10
	if hook != nil {
		hook()
	}
	return nil
}

var hook func()

func show(m map[string]any) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	s := ""
	for _, k := range ks {
		s += fmt.Sprintf("%s=%v ", k, m[k])
	}
	return s
}

func main() {
	gob.Register(GE{})
	var buf bytes.Buffer

	gob.NewEncoder(&buf).Encode(map[string]any{"a": GE{1}, "b": "x"})
	m := map[string]any{}
	alias := m
	err := gob.NewDecoder(&buf).Decode(&m)
	fmt.Println(err, len(m), len(alias), alias["a"], alias["b"])

	buf.Reset()
	gob.NewEncoder(&buf).Encode(map[string]any{"a": GE{1}, "b": "x", "c": GE{2}, "d": 4})
	m = map[string]any{"pre": 1}
	alias = m
	n := 0
	hook = func() { n++ }
	err = gob.NewDecoder(&buf).Decode(&m)
	hook = nil
	fmt.Println(err, n, show(m))
	fmt.Println(show(alias))

	buf.Reset()
	gob.NewEncoder(&buf).Encode([]any{GE{1}, "s", GE{2}})
	backing := make([]any, 3, 10)
	got := backing
	err = gob.NewDecoder(&buf).Decode(&got)
	fmt.Println(err, got, backing)
}
