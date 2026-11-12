// Maps and slices keep their identity across the native boundary.
// Native code that fills a map or a slice through a
// pointer (json.Unmarshal, json.Decoder.Decode, xml, gob, binary.Read)
// fills the interpreted map or array itself: every alias sees the changes,
// as in Go. Before the fix, the interpreted variable got a new map or a new
// array, and a pointer to a type with methods (golang-jwt's MapClaims) was
// not written back at all.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"
)

type Claims map[string]any

type S struct {
	M map[string]int
	L []int
	A [2]int
	N int
}

// MC has methods, like jwt.MapClaims.
type MC map[string]any

func (c MC) Aud() any { return c["aud"] }

type SM struct{ N int }

func (s SM) Get() int { return s.N }

type X struct {
	XMLName xml.Name `xml:"x"`
	Items   []string `xml:"i"`
}

type W struct {
	V  any
	MS []map[string]int
	AM [1]map[string]int
}

func main() {
	p := map[string]any{}
	q := p
	json.Unmarshal([]byte(`{"aud":"x"}`), &q)
	fmt.Println("1", len(p), len(q))

	c := Claims{}
	c2 := c
	json.Unmarshal([]byte(`{"aud":"x"}`), &c2)
	fmt.Println("2", len(c), len(c2))

	var i any = Claims{"old": 1}
	if cc, ok := i.(Claims); ok {
		json.Unmarshal([]byte(`{"aud":"x"}`), &cc)
	}
	fmt.Println("3", len(i.(Claims)))

	d := map[string]any{}
	d2 := d
	json.NewDecoder(strings.NewReader(`{"a":1}`)).Decode(&d2)
	fmt.Println("4", len(d), len(d2))

	sm := map[string]int{"z": 0}
	s := S{M: sm}
	json.Unmarshal([]byte(`{"M":{"a":1}}`), &s)
	fmt.Println("5", len(sm), len(s.M))

	s2 := &S{M: map[string]int{}}
	alias := s2.M
	json.Unmarshal([]byte(`{"a":2}`), &s2.M)
	fmt.Println("6", len(alias), len(s2.M))

	backing := make([]int, 3, 10)
	sl := backing[:0]
	json.Unmarshal([]byte(`[7,8,9]`), &sl)
	fmt.Println("7", backing, sl, cap(sl))

	outer := map[string]map[string]int{"k": {}}
	inner := outer["k"]
	json.Unmarshal([]byte(`{"k":{"a":1}}`), &outer)
	fmt.Println("8", len(inner), len(outer["k"]))

	var im any = map[string]any{}
	keep := im.(map[string]any)
	json.Unmarshal([]byte(`{"a":1}`), &im)
	fmt.Println("9", len(keep), im)

	ps := &S{N: 1}
	pps := &ps
	orig := ps
	json.Unmarshal([]byte(`{"N":5}`), pps)
	fmt.Println("10", orig.N, ps == orig)

	var arr [2]int
	json.Unmarshal([]byte(`[3,4]`), &arr)
	fmt.Println("11", arr)

	xs := make([]string, 0, 4)
	x := X{Items: xs}
	xml.Unmarshal([]byte(`<x><i>a</i><i>b</i></x>`), &x)
	fmt.Println("12", x.Items, xs[:2])

	var buf bytes.Buffer
	gob.NewEncoder(&buf).Encode(map[string]int{"g": 1})
	gm := map[string]int{"o": 0}
	gm2 := gm
	gob.NewDecoder(&buf).Decode(&gm2)
	fmt.Println("13", len(gm), len(gm2))

	var n int
	var w string
	fmt.Sscan("12 hi", &n, &w)
	fmt.Println("14", n, w)

	type B struct{ A, B uint16 }
	var bb B
	binary.Read(bytes.NewReader([]byte{1, 0, 2, 0}), binary.LittleEndian, &bb)
	bs := make([]uint16, 2)
	bs2 := bs
	binary.Read(bytes.NewReader([]byte{3, 0, 4, 0}), binary.LittleEndian, bs)
	fmt.Println("15", bb, bs2)

	rm := map[string]int{}
	rm2 := rm
	reflect.ValueOf(&rm2).Elem().SetMapIndex(reflect.ValueOf("r"), reflect.ValueOf(1))
	fmt.Println("16", len(rm), len(rm2))

	ms := []map[string]int{{}}
	m0 := ms[0]
	json.Unmarshal([]byte(`[{"a":1}]`), &ms)
	fmt.Println("17", len(m0), len(ms[0]))

	var nm map[string]int
	json.Unmarshal([]byte(`{"a":1}`), &nm)
	fmt.Println("18", nm)

	st := &S{}
	st2 := st
	json.Unmarshal([]byte(`{"N":3,"L":[1]}`), &st)
	fmt.Println("19", st2.N, st2.L, st == st2)

	type V struct{ A int }
	mp := map[string]*V{"k": {A: 1}}
	vk := mp["k"]
	json.Unmarshal([]byte(`{"k":{"A":2}}`), &mp)
	fmt.Println("20", vk.A, mp["k"].A, mp["k"] == vk)

	// untouched fields keep their maps
	vm := map[string]int{"v": 1}
	ws := &W{V: vm, MS: []map[string]int{{"a": 1}}, AM: [1]map[string]int{{"b": 2}}}
	ms0, am0 := ws.MS[0], ws.AM[0]
	json.Unmarshal([]byte(`{}`), ws)
	vm["w"] = 2
	ms0["c"] = 3
	am0["d"] = 4
	fmt.Println("21", ws.V, ws.MS, ws.AM)

	// slice with len > 0 decoded through a pointer: json reuses the array
	l := []int{1, 2, 3, 4}
	l2 := l
	json.Unmarshal([]byte(`[9,8]`), &l)
	fmt.Println("22", l, l2)

	// a map passed twice in the same call
	tw := map[string]int{}
	pair := &struct{ A, B map[string]int }{tw, tw}
	json.Unmarshal([]byte(`{"A":{"x":1}}`), pair)
	fmt.Println("23", tw, pair.B)

	// json.Marshal reads, does not change
	mm := map[string][]int{"k": {1}}
	kk := mm["k"]
	b, _ := json.Marshal(struct{ M map[string][]int }{mm})
	kk[0] = 5
	fmt.Println("24", string(b), mm)

	// pointers to types with methods (no native interface fits)
	var claims interface{ Aud() any } = MC{}
	if mc, ok := claims.(MC); ok {
		json.Unmarshal([]byte(`{"aud":"x"}`), &mc)
	}
	var sm2 SM
	json.Unmarshal([]byte(`{"N":3}`), &sm2)
	fmt.Println("25", claims.Aud(), sm2.Get())

	// binary.Read through a pointer to a slice, into a slice of structs
	is := make([]int32, 2)
	binary.Read(bytes.NewReader([]byte{3, 0, 0, 0, 4, 0, 0, 0}), binary.LittleEndian, &is)
	type T2 struct{ A, B uint8 }
	ts := make([]T2, 1)
	binary.Read(bytes.NewReader([]byte{5, 6}), binary.LittleEndian, ts)
	fmt.Println("26", is, ts)
}
