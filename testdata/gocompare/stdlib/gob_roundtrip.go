// encoding/gob round trips of program types: nested structs, pointers,
// maps, slices, registered interface values, GobEncoder/GobDecoder
// methods, and the errors of mismatched types.
package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"sort"
)

type Animal interface{ Sound() string }

type Dog struct {
	Name string
	Age  int
}

func (d Dog) Sound() string { return d.Name + " says woof" }

type Cat struct{ Lives int8 }

func (c *Cat) Sound() string { return fmt.Sprint("meow x", c.Lives) }

type Version struct{ Major, Minor int }

func (v Version) GobEncode() ([]byte, error) {
	return []byte(fmt.Sprintf("%d.%d", v.Major, v.Minor)), nil
}

func (v *Version) GobDecode(b []byte) error {
	_, err := fmt.Sscanf(string(b), "%d.%d", &v.Major, &v.Minor)
	return err
}

type Node struct {
	Val  int
	Next *Node
}

type Zoo struct {
	Name    string
	Animals []Animal
	ByName  map[string]Animal
	Counts  map[int]uint
	Head    *Node
	Ver     Version
	VerP    *Version
	Grid    [2][3]int16
	Data    []byte
	F       float32
	C       complex128
	private int
	Empty   []string
}

type Small struct {
	Name string
	F    float32
}

type Wrong struct {
	Name int
}

func main() {
	gob.Register(Dog{})
	gob.Register(&Cat{})
	z := Zoo{
		Name:    "zoo",
		Animals: []Animal{Dog{"rex", 3}, &Cat{9}},
		ByName:  map[string]Animal{"b": &Cat{1}, "a": Dog{"fido", 1}},
		Counts:  map[int]uint{3: 30, -1: 10},
		Head:    &Node{1, &Node{2, &Node{3, nil}}},
		Ver:     Version{1, 27},
		VerP:    &Version{2, 0},
		Grid:    [2][3]int16{{1, 2, 3}, {-4, -5, -6}},
		Data:    []byte{0, 1, 255},
		F:       2.5,
		C:       complex(1, -2),
		private: 9,
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	fmt.Println(enc.Encode(z))
	fmt.Println(enc.Encode(Small{"second", 1}))
	fmt.Println(enc.Encode(Small{"third", 2}))
	dec := gob.NewDecoder(&buf)
	var back Zoo
	fmt.Println(dec.Decode(&back))
	fmt.Println(back.Name, len(back.Animals), back.Counts, back.Ver, *back.VerP, back.Grid, back.Data, back.F, back.C, back.private, back.Empty == nil)
	for _, a := range back.Animals {
		fmt.Printf("%T %s\n", a, a.Sound())
	}
	keys := make([]string, 0)
	for k := range back.ByName {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s %T %s\n", k, back.ByName[k], back.ByName[k].Sound())
	}
	for n := back.Head; n != nil; n = n.Next {
		fmt.Print(n.Val, " ")
	}
	fmt.Println()
	// The next values on the same stream decode into a smaller type.
	var s Small
	fmt.Println(dec.Decode(&s), s)
	var w Wrong
	err := dec.Decode(&w)
	fmt.Println(err != nil, err)
	fmt.Println(dec.Decode(&s))

	// Decode a struct into one with a subset of fields.
	buf.Reset()
	fmt.Println(gob.NewEncoder(&buf).Encode(Zoo{Name: "only", F: 3}))
	fmt.Println(gob.NewDecoder(&buf).Decode(&s), s)

	// Unregistered interface type.
	type Hidden struct{ X int }
	buf.Reset()
	err = gob.NewEncoder(&buf).Encode(struct{ A Animal }{Bird{"tweet"}})
	fmt.Println(err)

	// Basic values.
	for _, v := range []any{42, "str", []int{1, 2}, map[string]bool{"t": true}, 3.5, Version{4, 5}} {
		buf.Reset()
		err := gob.NewEncoder(&buf).Encode(v)
		fmt.Print(err, " ")
		switch v.(type) {
		case int:
			var x int
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		case string:
			var x string
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		case []int:
			var x []int
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		case map[string]bool:
			var x map[string]bool
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		case float64:
			var x float64
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		case Version:
			var x Version
			fmt.Println(gob.NewDecoder(&buf).Decode(&x), x)
		}
	}
	var x int
	err = gob.NewDecoder(bytes.NewReader([]byte{1, 2, 3})).Decode(&x)
	fmt.Println(err, errors.Is(err, nil))
}

type Bird struct{ S string }

func (b Bird) Sound() string { return b.S }
