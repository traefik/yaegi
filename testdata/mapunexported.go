// Decoding into a map whose values are structs with an unexported field.
// The value of a map entry has no address in reflect,
// and the VM reads unexported fields through their address when it converts
// the native map back: it panicked with "reflect.Value.Addr of unaddressable
// value". The plugin david-garcia-garcia/traefik-geoblock has such a config
// (DatabaseSources map[string]DatabaseSource).
package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"fmt"
)

type Source struct {
	Path  string
	bound int
}

type Inner struct {
	Name string
	n    int
}

type Outer struct {
	In    Inner
	Arr   [2]Inner
	P     *Inner
	S     []Inner
	M     map[string]Inner
	hid   Inner
	flags [3]bool
}

type XSrc struct {
	Path  string `xml:"path"`
	bound int
}

type XList struct {
	Items []XSrc `xml:"item"`
}

type Config struct {
	Sources map[string]Source
}

func main() {
	m := map[string]Source{}
	err := json.Unmarshal([]byte(`{"seed":{"Path":"/db"}}`), &m)
	fmt.Printf("map: %v %+v\n", err, m)

	// an entry that is already there keeps its unexported field
	m2 := map[string]Source{"a": {Path: "x", bound: 7}}
	err = json.Unmarshal([]byte(`{"b":{"Path":"/b"}}`), &m2)
	fmt.Printf("existing: %v %+v\n", err, m2)

	// nested structs, arrays, pointers, slices and maps in the map value
	mo := map[string]Outer{}
	err = json.Unmarshal([]byte(`{"k":{"In":{"Name":"i"},"Arr":[{"Name":"a0"},{"Name":"a1"}],"P":{"Name":"p"},"S":[{"Name":"s"}],"M":{"z":{"Name":"mz"}}}}`), &mo)
	o := mo["k"]
	fmt.Printf("nested: %v %+v %+v %+v %+v %+v %+v %v\n", err, o.In, o.Arr, *o.P, o.S, o.M, o.hid, o.flags)

	ms := map[string][]Source{}
	err = json.Unmarshal([]byte(`{"l":[{"Path":"1"},{"Path":"2"}]}`), &ms)
	fmt.Printf("map of slices: %v %+v\n", err, ms)

	var sl []Source
	err = json.Unmarshal([]byte(`[{"Path":"s1"}]`), &sl)
	fmt.Printf("slice: %v %+v\n", err, sl)

	ma := map[string][2]Source{}
	err = json.Unmarshal([]byte(`{"a":[{"Path":"x"},{"Path":"y"}]}`), &ma)
	fmt.Printf("map of arrays: %v %+v\n", err, ma)

	mp := map[string]*Source{}
	err = json.Unmarshal([]byte(`{"p":{"Path":"/p"}}`), &mp)
	fmt.Printf("map of pointers: %v %+v\n", err, *mp["p"])

	mm := map[string]map[string]Source{}
	err = json.Unmarshal([]byte(`{"a":{"b":{"Path":"ab"}}}`), &mm)
	fmt.Printf("map of maps: %v %+v\n", err, mm)

	var cfg Config
	err = json.Unmarshal([]byte(`{"Sources":{"x":{"Path":"/x"}}}`), &cfg)
	fmt.Printf("struct field: %v %+v\n", err, cfg)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(map[string]Source{"g": {Path: "/g", bound: 3}}); err != nil {
		fmt.Println("gob encode:", err)
	}
	gm := map[string]Source{}
	err = gob.NewDecoder(&buf).Decode(&gm)
	fmt.Printf("gob: %v %+v\n", err, gm)

	var xl XList
	err = xml.Unmarshal([]byte(`<XList><item><path>/x1</path></item><item><path>/x2</path></item></XList>`), &xl)
	fmt.Printf("xml: %v %+v\n", err, xl)

	var v any = map[string]Source{"r": {Path: "/r", bound: 9}}
	b, err := json.Marshal(v)
	fmt.Printf("marshal: %v %s\n", err, b)
}
