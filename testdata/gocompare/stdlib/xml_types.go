// encoding/xml Marshal and Unmarshal of program structs: attributes,
// chardata, innerxml, comments, omitempty, nested paths a>b, XMLName,
// custom MarshalXML/UnmarshalXML and MarshalXMLAttr, and errors.
package main

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type Address struct {
	City  string   `xml:"city"`
	Zip   string   `xml:"zip,attr,omitempty"`
	Lines []string `xml:"lines>line"`
}

type Upper string

func (u Upper) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(strings.ToUpper(string(u)), start)
}

func (u *Upper) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	*u = Upper(strings.ToLower(s))
	return nil
}

type Flag bool

func (f Flag) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	v := "no"
	if f {
		v = "yes"
	}
	return xml.Attr{Name: name, Value: v}, nil
}

func (f *Flag) UnmarshalXMLAttr(a xml.Attr) error {
	*f = a.Value == "yes"
	return nil
}

type Person struct {
	XMLName xml.Name `xml:"person"`
	ID      int      `xml:"id,attr"`
	Active  Flag     `xml:"active,attr"`
	Name    string   `xml:"name"`
	Nick    string   `xml:"nick,omitempty"`
	Email   []string `xml:"contact>email"`
	Phone   string   `xml:"contact>phone,omitempty"`
	Addr    *Address `xml:"address,omitempty"`
	Title   Upper    `xml:"title"`
	Comment string   `xml:",comment"`
	Age     uint8    `xml:"age"`
	Score   float64  `xml:"score"`
	Ignored string   `xml:"-"`
	private int
}

type Note struct {
	XMLName xml.Name `xml:"note"`
	Lang    string   `xml:"lang,attr"`
	Text    string   `xml:",chardata"`
}

type Raw struct {
	XMLName xml.Name `xml:"raw"`
	Inner   string   `xml:",innerxml"`
}

type Any struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Kids    []Note     `xml:",any"`
}

func main() {
	p := Person{ID: 7, Active: true, Name: "Ann <&>", Email: []string{"a@x", "b@y"}, Addr: &Address{City: "Lyon", Zip: "69000", Lines: []string{"1 rue", "bis"}},
		Title: "Dr", Comment: " vip ", Age: 30, Score: 1.5, Ignored: "x", private: 1}
	b, err := xml.Marshal(p)
	fmt.Println(string(b), err)
	b2, err := xml.MarshalIndent(Person{Name: "Bob"}, "", "  ")
	fmt.Println(string(b2), err)
	var back Person
	err = xml.Unmarshal(b, &back)
	fmt.Println(err, back.XMLName.Local, back.ID, back.Active, back.Name, back.Email, back.Title, back.Comment, back.Age, back.Score, back.Ignored)
	fmt.Println(*back.Addr)

	for _, in := range []string{
		`<person id="x"><name>z</name></person>`,
		`<person><age>300</age></person>`,
		`<other><name>z</name></other>`,
		`<person><name>z</person>`,
		`<person active="no" id="3"><title>MiXeD</title><contact><email>e</email><phone>1</phone></contact></person>`,
		``,
	} {
		var q Person
		err := xml.Unmarshal([]byte(in), &q)
		fmt.Printf("%q => %v | %d %v %q %q %v %q\n", in, err, q.ID, q.Active, q.Name, q.Title, q.Email, q.Phone)
	}

	n := Note{Lang: "fr", Text: "bonjour & <salut>"}
	b, _ = xml.Marshal(n)
	fmt.Println(string(b))
	var n2 Note
	fmt.Println(xml.Unmarshal(b, &n2), n2.Lang, n2.Text)

	var r Raw
	fmt.Println(xml.Unmarshal([]byte(`<raw><a x="1">t</a><!-- c --><b/></raw>`), &r), r.Inner)
	b, _ = xml.Marshal(Raw{Inner: "<keep>as is</keep>"})
	fmt.Println(string(b))

	var a Any
	fmt.Println(xml.Unmarshal([]byte(`<root k="v" z="w"><note lang="en">hi</note><note lang="de">hallo</note></root>`), &a))
	fmt.Println(a.XMLName.Local, a.Attrs, a.Kids)

	// Token stream.
	d := xml.NewDecoder(strings.NewReader(`<?xml version="1.0"?><a x="1"><b>t</b><!--c--><c/></a>`))
	for {
		t, err := d.Token()
		if err != nil {
			fmt.Println("end:", err)
			break
		}
		switch t := t.(type) {
		case xml.StartElement:
			fmt.Println("start", t.Name.Local, t.Attr)
		case xml.EndElement:
			fmt.Println("end", t.Name.Local)
		case xml.CharData:
			fmt.Printf("chardata %q\n", string(t))
		case xml.Comment:
			fmt.Printf("comment %q\n", string(t))
		case xml.ProcInst:
			fmt.Printf("procinst %s %q\n", t.Target, string(t.Inst))
		}
	}
	// Errors.
	_, err = xml.Marshal(map[string]int{"a": 1})
	fmt.Println(err)
	_, err = xml.Marshal(make(chan int))
	fmt.Println(err)
	var sb strings.Builder
	xml.EscapeText(&sb, []byte(`"a" < 'b' & c`))
	fmt.Println(sb.String())
	enc := xml.NewEncoder(&sb)
	enc.Indent("", "\t")
	fmt.Println(enc.Encode([]Note{{Lang: "a", Text: "1"}, {Lang: "b", Text: "2"}}), enc.Flush())
	fmt.Println(sb.String())
}
