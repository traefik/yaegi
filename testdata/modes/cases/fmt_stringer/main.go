// fmt: String() on value and pointer receivers, at top level and nested
// in struct fields, slices, maps, arrays and interfaces.
package main

import (
	"fmt"
	"strings"
)

type Celsius float64

func (c Celsius) String() string { return fmt.Sprintf("%.1f°C", float64(c)) }

type Node struct{ Name string }

func (n *Node) String() string { return "node:" + n.Name }

type Level int

func (l Level) String() string { return strings.Repeat("#", int(l)) }

type Holder struct {
	T     Celsius
	N     Node
	NP    *Node
	L     []Level
	M     map[string]Celsius
	K     map[Level]int
	Arr   [2]Celsius
	Any   any
	inner Celsius
}

type Wrap struct{ Holder }

type Bad struct{}

func (Bad) String() string { panic("boom") }

type NilP struct{ X int }

func (p *NilP) String() string {
	if p == nil {
		return "nil NilP"
	}
	return "NilP"
}

func main() {
	h := Holder{T: 21, N: Node{"v"}, NP: &Node{"p"}, L: []Level{1, 3}, M: map[string]Celsius{"a": 1}, K: map[Level]int{2: 1}, Arr: [2]Celsius{5, 6}, Any: Level(2), inner: 9}
	fmt.Println(Celsius(3), &Node{"top"}, Node{"val"}, Level(4))
	fmt.Println(h)
	fmt.Printf("%v\n%+v\n%s\n", h, h, h)
	fmt.Printf("%d %v %s %q %x\n", Level(3), Level(3), Level(3), Level(3), Level(3))
	fmt.Println([]Celsius{1, 2}, map[Celsius]Level{1: 1}, []*Node{{"a"}, {"b"}}, []any{Celsius(0), Level(1)})
	fmt.Println(Wrap{h}.T, Wrap{Holder{NP: &Node{"w"}}}.NP)
	var np *NilP
	fmt.Println(np, Bad{})
	fmt.Printf("%v|%s|%d\n", Bad{}, Bad{}, Bad{})
	var s fmt.Stringer = Celsius(1)
	fmt.Println(s, s.String())
	fmt.Println(fmt.Sprintf("%10s|%-10v|", Level(2), Celsius(1)))
}
