package main

// json.Unmarshal into recursive types: in default mode,
// native encoding/json must create values of the recursive type itself, not
// map[string]any (vm/recursive.go).

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Rule is recursive through a slice (smerschjohann/mtlswhitelist RawRule).
type Rule struct {
	Name  string
	Rules []Rule
}

// List is recursive through a pointer.
type List struct {
	V    int64 `json:"v"`
	Next *List `json:"next,omitempty"`
}

// Dir is recursive through a map.
type Dir struct {
	Size int
	Sub  map[string]Dir
}

// Tree has two recursive fields and an array of pointers.
type Tree struct {
	L, R *Tree
	Kids [2]*Tree
	N    int
}

// A and B are mutually recursive.
type A struct {
	Name string
	B    *B
}

type B struct {
	As []A
}

// Rules is a named slice of a recursive struct.
type Rules []Node

type Node struct {
	ID   int `json:"id"`
	Kids Rules
}

// Ext embeds a recursive struct and is recursive itself.
type Ext struct {
	Tag string
	Rule
	More []Ext
}

// Shared has two pointers to the same value, and an any whose value native
// code cannot copy (a channel).
type Shared struct {
	V, W  int
	Next  *Shared
	X     any
	Other *Shared
}

func show(err error, v any) {
	b, _ := json.Marshal(v)
	fmt.Printf("%v %s\n", err, b)
}

func depth(l *List) int {
	n := 0
	for ; l != nil; l = l.Next {
		n++
	}
	return n
}

func main() {
	var r Rule
	err := json.Unmarshal([]byte(`{"Rules":[{"Name":"a"}]}`), &r)
	fmt.Printf("%v %+v\n", err, r)

	err = json.Unmarshal([]byte(`{"Name":"top","Rules":[{"Name":"a","Rules":[{"Name":"b","Rules":[{"Name":"c"}]}]},{"Name":"d"}]}`), &r)
	fmt.Printf("%v %+v\n", err, r)

	// A large integer deep in the data keeps its exact value.
	var l List
	err = json.Unmarshal([]byte(`{"v":1,"next":{"v":2,"next":{"v":9007199254740993}}}`), &l)
	fmt.Println(err, l.V, l.Next.V, l.Next.Next.V, l.Next.Next.Next == nil)

	// Deep data: 50 levels.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, `{"v":%d,"next":`, i)
	}
	sb.WriteString("null")
	sb.WriteString(strings.Repeat("}", 50))
	var deep List
	err = json.Unmarshal([]byte(sb.String()), &deep)
	fmt.Println(err, depth(&deep))

	// Unmarshal into an existing value merges, and keeps the pointers.
	tail := &List{V: 7}
	head := List{V: 6, Next: tail}
	err = json.Unmarshal([]byte(`{"next":{"next":{"v":8}}}`), &head)
	fmt.Println(err, head.V, head.Next == tail, tail.V, tail.Next.V)

	var d Dir
	err = json.Unmarshal([]byte(`{"Size":1,"Sub":{"x":{"Size":2,"Sub":{"y":{"Size":3}}}}}`), &d)
	show(err, d)

	var t Tree
	err = json.Unmarshal([]byte(`{"N":1,"L":{"N":2,"R":{"N":3}},"Kids":[null,{"N":4,"Kids":[{"N":5},null]}]}`), &t)
	show(err, t)

	var a A
	err = json.Unmarshal([]byte(`{"Name":"a1","B":{"As":[{"Name":"a2","B":{"As":[{"Name":"a3"}]}}]}}`), &a)
	show(err, a)

	var rs Rules
	err = json.Unmarshal([]byte(`[{"id":1,"Kids":[{"id":2,"Kids":[{"id":3}]}]}]`), &rs)
	show(err, rs)

	// Two recursive fields, 40 levels deep on both sides.
	var deepTree strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&deepTree, `{"N":%d,"R":{"N":-%d},"L":`, i, i)
	}
	deepTree.WriteString(`{"N":99}` + strings.Repeat("}", 40))
	var t2 Tree
	err = json.Unmarshal([]byte(deepTree.String()), &t2)
	n := 0
	for p := &t2; p != nil; p = p.L {
		n += p.N
		if p.R != nil {
			n += p.R.N
		}
	}
	fmt.Println(err, n)

	// An embedded struct whose fields json promotes.
	var e Ext
	err = json.Unmarshal([]byte(`{"Tag":"t","Name":"n","Rules":[{"Name":"m","Rules":[{"Name":"o"}]}],"More":[{"Tag":"u","Name":"p"}]}`), &e)
	fmt.Printf("%v %+v\n", err, e)

	// A cycle in the value already there.
	cyc := &List{V: 1}
	cyc.Next = cyc
	err = json.Unmarshal([]byte(`{"v":5}`), cyc)
	fmt.Println(err, cyc.V, cyc.Next == cyc)

	// Two fields point to the same value, which must get one native copy
	// only: json fills it through both. The channel in X is not copied to
	// native code, and this must not drop the copies made before it.
	sp := &Shared{}
	sr := &Shared{Next: sp, X: make(chan int), Other: sp}
	err = json.Unmarshal([]byte(`{"Next":{"V":1,"Next":{"V":3,"Next":{"V":4}}},"Other":{"W":2,"Next":{"V":5,"Next":{"W":6}}}}`), sr)
	fmt.Println(err, sr.Next == sr.Other, sp.V, sp.W, sp.Next.V, sp.Next.W, sp.Next.Next.V, sp.Next.Next.W)

	var rules []Rule
	err = json.Unmarshal([]byte(`[{"Name":"x","Rules":[{"Name":"y"}]}]`), &rules)
	fmt.Printf("%v %+v\n", err, rules)

	// A type error deep in the data: json goes on and reports it. The
	// message holds the struct name, which native code does not know in
	// default mode: print the other parts.
	var r2 Rule
	err = json.Unmarshal([]byte(`{"Name":"p","Rules":[{"Name":5,"Rules":[{"Name":"q"}]}]}`), &r2)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		fmt.Println(ute.Value, ute.Field, ute.Type, ute.Offset)
	}
	fmt.Printf("%+v\n", r2)

	// Syntax errors.
	err = json.Unmarshal([]byte(`{"Rules":[{"Name":"a"]}`), &r2)
	fmt.Println(err)

	// Maps and slices keep their identity: json fills a non nil map and a
	// slice with room in place, so their aliases see the new values.
	ir := Ident{Named: map[string]Ident{"old": {Name: "o"}}, Tags: map[string]int{"x": 1}, Nums: make([]int, 0, 4)}
	named, tags, nums := ir.Named, ir.Tags, ir.Nums[:4]
	err = json.Unmarshal([]byte(`{"Name":"top","Named":{"new":{"Name":"n","Rules":[{"Name":"deep"}]}},"Tags":{"y":2},"Nums":[7,8],"Rules":[{"Name":"a","Rules":[{"Name":"b"}]}]}`), &ir)
	fmt.Println(err, ir.Name, ir.Rules[0].Rules[0].Name, len(named), named["new"].Rules[0].Name, tags, nums)
	all := map[string]Ident{"k": {Name: "k"}}
	alias := all
	err = json.Unmarshal([]byte(`{"k2":{"Name":"v","Rules":[{"Name":"w"}]}}`), &all)
	fmt.Println(err, len(alias), alias["k2"].Rules[0].Name)
}

// Ident has a map and a slice of itself, and plain maps and slices.
type Ident struct {
	Name  string
	Rules []Ident
	Named map[string]Ident
	Tags  map[string]int
	Nums  []int
}
