package main

import (
	"encoding/xml"
	"fmt"
)

// encoding/xml with interpreted structs that hold the xml package's own
// types (xml.Name, xml.Attr, []xml.Attr): their meaning is kept, in
// Marshal and Unmarshal. These calls stay native (the AOT copy of xml has
// its own xml.Name and xml.Attr types).

type X struct {
	XMLName xml.Name `xml:"x"`
	A       int      `xml:"a,attr"`
}

type Y struct {
	XMLName xml.Name
	Lang    xml.Attr   `xml:"lang,attr"`
	Rest    []xml.Attr `xml:",any,attr"`
	Body    string     `xml:",chardata"`
}

type Inner struct {
	XMLName xml.Name `xml:"inner"`
	V       string   `xml:"v,attr"`
}

type Outer struct {
	XMLName xml.Name `xml:"outer"`
	In      Inner
	N       int `xml:"n"`
}

type G[T any] struct {
	XMLName xml.Name `xml:"g"`
	Val     T        `xml:"val"`
}

func main() {
	var x X
	err := xml.Unmarshal([]byte(`<x a="1"/>`), &x)
	fmt.Printf("%+v %v\n", x, err)
	out, err := xml.Marshal(x)
	fmt.Println(string(out), err)
	out, err = xml.Marshal(X{XMLName: xml.Name{Local: "other"}, A: 2})
	fmt.Println(string(out), err)

	var y Y
	err = xml.Unmarshal([]byte(`<y lang="fr" k="1" j="2">hello</y>`), &y)
	fmt.Printf("%+v %v\n", y, err)
	out, err = xml.Marshal(y)
	fmt.Println(string(out), err)

	o := Outer{In: Inner{V: "q"}, N: 3}
	out, err = xml.MarshalIndent(o, "", " ")
	fmt.Println(string(out), err)
	var o2 Outer
	err = xml.Unmarshal(out, &o2)
	fmt.Printf("%+v %v\n", o2, err)

	out, err = xml.Marshal(&G[int]{Val: 4})
	fmt.Println(string(out), err)
	var g G[string]
	err = xml.Unmarshal([]byte(`<g><val>s</val></g>`), &g)
	fmt.Printf("%+v %v\n", g, err)
}
