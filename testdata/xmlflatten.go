package main

// Methods that encoding/xml does not call, in values that hold an xml.Name
// (so that the call stays native in default mode): the check of
// vm/lostmethods.go must not refuse them. Go flattens an embedded struct
// (its own MarshalXML is not called: two embedded marshalers are ambiguous,
// and none is called), uses MarshalXMLAttr and MarshalText only for an
// attribute, MarshalXML only for an element, and ignores unexported and
// `xml:"-"` fields. The same for decoding.

import (
	"encoding/xml"
	"fmt"
)

// Elem has MarshalXML and UnmarshalXML: not used for an attribute.
type Elem string

func (e Elem) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return enc.EncodeElement("elem:"+string(e), start)
}

func (e *Elem) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := dec.DecodeElement(&s, &start); err != nil {
		return err
	}
	*e = Elem("got:" + s)
	return nil
}

// Attr has MarshalXMLAttr and UnmarshalXMLAttr: not used for an element.
type Attr struct{ N int }

func (a Attr) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{Name: name, Value: fmt.Sprint("attr:", a.N)}, nil
}

func (a *Attr) UnmarshalXMLAttr(attr xml.Attr) error {
	a.N = len(attr.Value)
	return nil
}

// Left and Right both have the methods: embedded together, they are
// ambiguous, and the fields are flattened.
type Left struct{ L int }

func (Left) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return fmt.Errorf("Left.MarshalXML called")
}

func (*Left) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	return fmt.Errorf("Left.UnmarshalXML called")
}

type Right struct{ R int }

func (Right) MarshalXML(enc *xml.Encoder, start xml.StartElement) error {
	return fmt.Errorf("Right.MarshalXML called")
}

func (*Right) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	return fmt.Errorf("Right.UnmarshalXML called")
}

type Doc struct {
	XMLName xml.Name `xml:"doc"`
	Left
	*Right
	A      Elem `xml:"a,attr"`
	E      Attr `xml:"e"`
	Skip   Elem `xml:"-"`
	hidden Elem
}

func main() {
	d := Doc{Left: Left{1}, Right: &Right{2}, A: "x", E: Attr{3}, Skip: "s", hidden: "h"}
	b, err := xml.Marshal(d)
	fmt.Println(string(b), err)
	b, err = xml.Marshal(&d)
	fmt.Println(string(b), err)

	var got Doc
	err = xml.Unmarshal([]byte(`<doc a="y"><L>4</L><R>5</R><e><N>6</N></e></doc>`), &got)
	fmt.Println(err, got.L, got.Right != nil && got.R == 5, got.A, got.E.N)
}
