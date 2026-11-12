// AOT test program: gopkg.in/yaml.v3 decoding into `any` fields of
// interpreted structs. yaml.v3 asks every field for an Unmarshaler with
// v.Addr().Interface(): for a field of type any, the twin reflect handed out
// a native shadow of the field, and the value that the decoder then set
// into the field itself was lost when the shadow went back into it at the
// end of the call (the field stayed nil). The twin now reads and writes a
// cell with a shadow through the shadow, also for an interface. (From the
// reflect battery of the modes challenge, case lib_yaml in AOT mode.)
//
//aot:lib gopkg.in/yaml.v3
package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Name  string         `yaml:"name"`
	Any   any            `yaml:"any"`
	Map   any            `yaml:"map"`
	Str   any            `yaml:"str"`
	Num   any            `yaml:"num"`
	Extra map[string]any `yaml:",inline"`
}

type Plain struct {
	A any `yaml:"a"`
	N int `yaml:"n"`
}

type Outer struct {
	In   Plain  `yaml:"in"`
	Ptr  *Plain `yaml:"ptr"`
	List []any  `yaml:"list"`
}

func main() {
	var s Server
	err := yaml.Unmarshal([]byte("name: web\nany: [1, two]\nmap: {k: v}\nstr: x\nnum: 2.5\nother: [3]\n"), &s)
	fmt.Println(err, s.Name, s.Any, s.Map, s.Str, s.Num, s.Extra)
	fmt.Printf("%T %T %T %T\n", s.Any, s.Map, s.Str, s.Num)

	p := Plain{A: "old"}
	err = yaml.Unmarshal([]byte("a: {x: [1]}\nn: 3\n"), &p)
	fmt.Println(err, p.A, p.N)

	var o Outer
	err = yaml.Unmarshal([]byte("in: {a: 1}\nptr: {a: [b]}\nlist: [{c: d}, 4]\n"), &o)
	fmt.Println(err, o.In.A, o.Ptr.A, o.List)

	out, err := yaml.Marshal(s)
	fmt.Print(string(out))
	fmt.Println(err)
}
