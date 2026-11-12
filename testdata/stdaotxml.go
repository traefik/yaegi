package main

// encoding/xml in default mode runs as its AOT copy (stdAOTModes in
// vm/aot.go): a value that holds xml.Attr stayed native, which ignored
// MarshalXML and MarshalXMLAttr of the program's types.
// (From the reflect battery of the modes challenge.)
//
// encoding/xml: MarshalXML, UnmarshalXML, MarshalXMLAttr,
// UnmarshalXMLAttr, MarshalText and UnmarshalText on interpreted types.

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

type Temp float64

func (t Temp) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "unit"}, Value: "C"})
	return e.EncodeElement(strconv.FormatFloat(float64(t), 'f', 1, 64), start)
}

func (t *Temp) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	f, err := strconv.ParseFloat(s, 64)
	*t = Temp(f * 10)
	return err
}

type Level int

func (l Level) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{Name: name, Value: strings.Repeat("*", int(l))}, nil
}

func (l *Level) UnmarshalXMLAttr(a xml.Attr) error {
	*l = Level(len(a.Value))
	return nil
}

type Tag string

func (t Tag) MarshalText() ([]byte, error) { return []byte("#" + string(t)), nil }

func (t *Tag) UnmarshalText(b []byte) error {
	*t = Tag(strings.TrimPrefix(string(b), "#"))
	return nil
}

type Reading struct {
	XMLName xml.Name `xml:"reading"`
	Lvl     Level    `xml:"lvl,attr"`
	T       Temp     `xml:"t"`
	TP      *Temp    `xml:"tp"`
	Temps   []Temp   `xml:"temps>t"`
	Tag     Tag      `xml:"tag"`
	TagAttr Tag      `xml:"tagattr,attr"`
}

func main() {
	tp := Temp(3.25)
	r := Reading{Lvl: 3, T: 21.5, TP: &tp, Temps: []Temp{1, 2}, Tag: "go", TagAttr: "vm"}
	b, err := xml.Marshal(r)
	fmt.Println(string(b), err)
	var q Reading
	err = xml.Unmarshal(b, &q)
	fmt.Println(err, q.Lvl, q.T, *q.TP, q.Temps, q.Tag, q.TagAttr)
	b, err = xml.Marshal(Temp(7))
	fmt.Println(string(b), err)
}
