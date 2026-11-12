// encoding/xml: struct tags, attributes, chardata, nested and embedded
// fields, omitempty, pointers, interfaces.
package main

import (
	"encoding/xml"
	"fmt"
)

type Address struct {
	City  string `xml:"city"`
	State string `xml:"state,omitempty"`
}

type Common struct {
	ID string `xml:"id,attr"`
}

type Note struct {
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

type Person struct {
	XMLName xml.Name `xml:"person"`
	Common
	Name    string   `xml:"name"`
	Emails  []string `xml:"email"`
	Addr    *Address `xml:"address,omitempty"`
	Home    Address  `xml:"home"`
	Notes   []Note   `xml:"notes>note"`
	Comment string   `xml:",comment"`
	Age     int      `xml:"age,omitempty"`
	Any     any      `xml:"any"`
	Skip    string   `xml:"-"`
}

func main() {
	p := Person{Common: Common{"p1"}, Name: "Ann", Emails: []string{"a@x", "b@x"}, Home: Address{City: "Lyon"},
		Notes: []Note{{"en", "hi"}, {"fr", "salut"}}, Comment: " c ", Any: Address{City: "Nice"}, Skip: "no"}
	b, err := xml.MarshalIndent(p, "", " ")
	fmt.Println(string(b), err)

	var q Person
	err = xml.Unmarshal(b, &q)
	fmt.Println(err, q.ID, q.Name, q.Emails, q.Addr == nil, q.Home.City, q.Notes, q.Age)

	in := `<person id="p2"><name>Bob</name><address><city>Paris</city><state>IDF</state></address><age>40</age></person>`
	var r Person
	err = xml.Unmarshal([]byte(in), &r)
	fmt.Println(err, r.ID, r.Name, *r.Addr, r.Age, r.XMLName.Local)

	err = xml.Unmarshal([]byte(`<person><age>x</age></person>`), &r)
	fmt.Println(err)
}
