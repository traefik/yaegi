// Native boundary of encoding/xml: program types given to xml (docs/BOUNDARY.md).
package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

var cur string

// probe runs one probe: its lines start with its id, and its panic is printed.
func probe(id string, f func()) {
	cur = id
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("%s: panic: %T: %v\n", id, r, r)
		}
	}()
	f()
}

// out prints one line of the current probe.
func out(format string, args ...any) { fmt.Printf(cur+": "+format+"\n", args...) }

// XM has MarshalXML (value receiver) and UnmarshalXML (pointer receiver).
type XM struct{ S string }

func (x XM) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "by"}, Value: "XM"})
	return e.EncodeElement("xm:"+x.S, start)
}

func (x *XM) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	x.S = "un:" + start.Name.Local + ":" + s
	return nil
}

// PX has MarshalXML with a pointer receiver.
type PX struct{ N int }

func (p *PX) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(fmt.Sprintf("px:%d", p.N), start)
}

// GX is a generic type with MarshalXML.
type GX[T any] struct{ V T }

func (g GX[T]) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(fmt.Sprintf("gx:%v", g.V), start)
}

// XA has MarshalXMLAttr and UnmarshalXMLAttr.
type XA struct{ V string }

func (a XA) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{Name: name, Value: "xa:" + a.V}, nil
}

func (a *XA) UnmarshalXMLAttr(attr xml.Attr) error {
	a.V = "unattr:" + attr.Value
	return nil
}

// TM has MarshalText and UnmarshalText.
type TM struct{ S string }

func (t TM) MarshalText() ([]byte, error) { return []byte("tm:" + t.S), nil }
func (t *TM) UnmarshalText(b []byte) error {
	t.S = "untext:" + string(b)
	return nil
}

// Named basic types.
type (
	Raw   string
	RawB  []byte
	Cmt   string
	CD    []byte
	NInt  int
	NF    float64
	NBool bool
	Named string
	Ints  []NInt
)

// Ch is a type xml cannot marshal.
type Ch chan int

// Doc has fields of many forms.
type Doc struct {
	XMLName xml.Name `xml:"doc"`
	ID      NInt     `xml:"id,attr"`
	At      XA       `xml:"at,attr"`
	TA      TM       `xml:"ta,attr"`
	Opt     Named    `xml:"opt,attr,omitempty"`
	X       XM       `xml:"x"`
	P       PX       `xml:"p"`
	G       GX[NInt] `xml:"g"`
	T       TM       `xml:"t"`
	N       Named    `xml:"a>b"`
	L       Ints     `xml:"l"`
	F       NF       `xml:"f"`
	B       NBool    `xml:"bb"`
	E       Ints     `xml:"e,omitempty"`
	Any     any      `xml:"any"`
	C       Cmt      `xml:",comment"`
}

// Inner holds raw XML in named types.
type Inner struct {
	XMLName xml.Name `xml:"in"`
	R       Raw      `xml:",innerxml"`
}

type InnerB struct {
	XMLName xml.Name `xml:"inb"`
	R       RawB     `xml:",innerxml"`
}

type InnerS struct {
	XMLName xml.Name `xml:"ins"`
	R       string   `xml:",innerxml"`
}

type Chars struct {
	XMLName xml.Name `xml:"cd"`
	D       CD       `xml:",chardata"`
	Cd      Named    `xml:",cdata"`
}

// Rec is a recursive type.
type Rec struct {
	Name string `xml:"name,attr"`
	Kids []*Rec `xml:"rec"`
}

func (r *Rec) String() string { return fmt.Sprintf("%s%v", r.Name, r.Kids) }

// enc4 marshals v with Marshal, MarshalIndent, Encoder.Encode and
// Encoder.EncodeElement.
func enc4(label string, v any) {
	b, err := xml.Marshal(v)
	out("%s Marshal: %s %v", label, b, err)
	b, err = xml.MarshalIndent(v, "", " ")
	out("%s MarshalIndent: %s %v", label, strings.ReplaceAll(string(b), "\n", "|"), err)
	var buf bytes.Buffer
	err = xml.NewEncoder(&buf).Encode(v)
	out("%s Encode: %s %v", label, buf.String(), err)
	buf.Reset()
	e := xml.NewEncoder(&buf)
	err = e.EncodeElement(v, xml.StartElement{Name: xml.Name{Local: "top"}})
	e.Flush()
	out("%s EncodeElement: %s %v", label, buf.String(), err)
}

// dec3 decodes data with Unmarshal, Decoder.Decode and
// Decoder.DecodeElement into new targets made by mk.
func dec3(label, data string, mk func() any) {
	t := mk()
	err := xml.Unmarshal([]byte(data), t)
	out("%s Unmarshal: %+v %v", label, reflect.ValueOf(t).Elem().Interface(), err)
	t = mk()
	err = xml.NewDecoder(strings.NewReader(data)).Decode(t)
	out("%s Decode: %+v %v", label, reflect.ValueOf(t).Elem().Interface(), err)
	t = mk()
	d := xml.NewDecoder(strings.NewReader(data))
	tok, _ := d.Token()
	se, _ := tok.(xml.StartElement)
	err = d.DecodeElement(t, &se)
	out("%s DecodeElement: %+v %v", label, reflect.ValueOf(t).Elem().Interface(), err)
}

// br is a reader with ReadByte (from the embedded *strings.Reader).
type br struct{ *strings.Reader }

// plainR is a reader without ReadByte.
type plainR struct{ r *strings.Reader }

func (p *plainR) Read(b []byte) (int, error) { return p.r.Read(b) }

// gbr is a generic reader with ReadByte.
type gbr[T any] struct {
	r   *strings.Reader
	tag T
}

func (g gbr[T]) Read(b []byte) (int, error) { return g.r.Read(b) }
func (g gbr[T]) ReadByte() (byte, error)    { return g.r.ReadByte() }

const latin = `<?xml version="1.0" encoding="latin1"?><a k="v"><b>x</b><c/></a>`

// readers gives one reader of each kind over s.
func readers(s string) []io.Reader {
	return []io.Reader{br{strings.NewReader(s)}, &plainR{strings.NewReader(s)}, gbr[int]{strings.NewReader(s), 1}, strings.NewReader(s)}
}

func charset(label string, r io.Reader) (io.Reader, error) {
	out("CharsetReader %s got %T", label, r)
	return r, nil
}

func main() {
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect kind
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect length
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect type name
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect walk
	probe("Marshal/Doc", func() {
		d := Doc{ID: 7, At: XA{"a"}, TA: TM{"ta"}, X: XM{"x"}, P: PX{1}, G: GX[NInt]{2}, T: TM{"t"}, N: "n<&>", L: Ints{1, 2}, F: 1.5, B: true, Any: XM{"in any"}, C: "note"}
		enc4("Doc", d)
		enc4("*Doc", &d)
		enc4("zero", Doc{})
	})
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect kind
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement reflect type name
	probe("Marshal/top", func() {
		enc4("XM", XM{"top"})
		enc4("*XM", &XM{"ptop"})
		enc4("PX", PX{3})
		enc4("*PX", &PX{4})
		enc4("GX", GX[string]{"g"})
		enc4("TM", TM{"t"})
		enc4("Named", Named("s"))
		enc4("NInt", NInt(5))
		enc4("Ints", Ints{1, 2})
		enc4("[]any", []any{XM{"a"}, &PX{5}, TM{"b"}})
		var a any = GX[int]{9}
		enc4("&any", &a)
		enc4("Rec", &Rec{Name: "r", Kids: []*Rec{{Name: "k", Kids: []*Rec{{Name: "g"}}}}})
		enc4("anon", struct {
			XMLName xml.Name `xml:"anon"`
			A       NInt     `xml:"a,attr"`
		}{A: 1})
	})
	// row: encoding/xml Marshal,MarshalIndent,(*Encoder).Encode,(*Encoder).EncodeElement assert x.(T), T unnamed
	probe("Marshal/innerxml", func() {
		enc4("Raw", Inner{R: "<raw>&</raw>"})
		enc4("RawB", InnerB{R: RawB("<rawb/>")})
		enc4("string", InnerS{R: "<s/>"})
		enc4("*Raw", &Inner{R: "<p/>"})
		enc4("Chars", Chars{D: CD("a<b"), Cd: "c]]>d"})
	})
	// row: encoding/xml (*UnsupportedTypeError).Error reflect type name
	probe("UnsupportedTypeError", func() {
		for i, v := range []any{Ch(nil), struct{ C Ch }{}, map[Named]int{"a": 1}, struct {
			F func()
		}{}} {
			_, err := xml.Marshal(v)
			var ue *xml.UnsupportedTypeError
			out("%d: %v %v", i, errors.As(err, &ue), err)
		}
		out("made: %v", (&xml.UnsupportedTypeError{Type: reflect.TypeOf(Ch(nil))}).Error())
	})
	// row: encoding/xml Unmarshal,(*Decoder).Decode,(*Decoder).DecodeElement reflect kind
	// row: encoding/xml Unmarshal,(*Decoder).Decode,(*Decoder).DecodeElement reflect length
	// row: encoding/xml Unmarshal,(*Decoder).Decode,(*Decoder).DecodeElement reflect type name
	// row: encoding/xml Unmarshal,(*Decoder).Decode,(*Decoder).DecodeElement reflect walk
	probe("Unmarshal/Doc", func() {
		data := `<doc id="7" at="a" ta="ta" opt="o"><x>xx</x><g>2</g><t>tt</t><a><b>nested</b></a><l>1</l><l>2</l><f>2.5</f><bb>true</bb><!--c--></doc>`
		dec3("Doc", data, func() any { return &Doc{} })
		dec3("XM", `<top>v</top>`, func() any { return &XM{} })
		dec3("TM", `<top>v</top>`, func() any { return &TM{} })
		dec3("NInt", `<top>12</top>`, func() any { return new(NInt) })
		dec3("Ints", `<top>3</top>`, func() any { return &Ints{} })
		dec3("Named", `<top>s</top>`, func() any { return new(Named) })
		dec3("Inner", `<in><x>1</x></in>`, func() any { return &Inner{} })
		dec3("InnerB", `<inb><y/></inb>`, func() any { return &InnerB{} })
		dec3("Chars", `<cd>t<![CDATA[x]]></cd>`, func() any { return &Chars{} })
		dec3("Rec", `<rec name="r"><rec name="k"><rec name="g"/></rec></rec>`, func() any { return &Rec{} })
		dec3("badint", `<top>x</top>`, func() any { return new(NInt) })
		dec3("wrongname", `<other/>`, func() any { return &Doc{} })
		var a any
		err := xml.Unmarshal([]byte(`<a/>`), a)
		out("nil any: %v", err)
		err = xml.Unmarshal([]byte(`<a/>`), XM{})
		out("non-pointer: %v", err)
		err = xml.Unmarshal([]byte(`<a>1</a>`), &a)
		out("&any: %v %v", a, err)
	})
	// row: encoding/xml NewDecoder,(*Decoder).Token,(*Decoder).RawToken,(*Decoder).Skip,(*Decoder).Decode,(*Decoder).DecodeElement assert x.(io.ByteReader)
	// row: encoding/xml (*Decoder).Token,(*Decoder).RawToken,(*Decoder).Skip,(*Decoder).Decode,(*Decoder).DecodeElement assert x.(io.Reader)
	// row: encoding/xml (*Decoder).Token,(*Decoder).RawToken,(*Decoder).Skip,(*Decoder).Decode,(*Decoder).DecodeElement panic x.(io.Reader) without ok: *runtime.TypeAssertionError
	probe("Decoder/readers", func() {
		for i, r := range readers(latin) {
			d := xml.NewDecoder(r)
			d.CharsetReader = func(cs string, in io.Reader) (io.Reader, error) { return charset(fmt.Sprint("Token ", i, cs), in) }
			var toks []string
			for {
				t, err := d.Token()
				if err != nil {
					toks = append(toks, err.Error())
					break
				}
				toks = append(toks, fmt.Sprintf("%T", t))
			}
			out("%d Token: %v", i, toks)
		}
		for i, r := range readers(latin) {
			d := xml.NewDecoder(r)
			d.CharsetReader = func(cs string, in io.Reader) (io.Reader, error) { return charset(fmt.Sprint("RawToken ", i), in) }
			n := 0
			for {
				if _, err := d.RawToken(); err != nil {
					break
				}
				n++
			}
			out("%d RawToken: %d tokens", i, n)
		}
		for i, r := range readers(`<a><b>x</b></a><z/>`) {
			d := xml.NewDecoder(r)
			d.Token()
			err := d.Skip()
			t, _ := d.Token()
			out("%d Skip: %v %v", i, err, t)
		}
		for i, r := range readers(latin) {
			d := xml.NewDecoder(r)
			d.CharsetReader = func(cs string, in io.Reader) (io.Reader, error) { return charset(fmt.Sprint("Decode ", i), in) }
			var v struct {
				K string `xml:"k,attr"`
				B XM     `xml:"b"`
			}
			err := d.Decode(&v)
			out("%d Decode: %+v %v", i, v, err)
		}
		for i, r := range readers(latin) {
			d := xml.NewDecoder(r)
			d.CharsetReader = func(cs string, in io.Reader) (io.Reader, error) { return charset(fmt.Sprint("DecodeElement ", i), in) }
			var v XM
			err := d.DecodeElement(&v, nil)
			out("%d DecodeElement: %+v %v", i, v, err)
		}
		// without CharsetReader, the encoding declaration is an error
		d := xml.NewDecoder(br{strings.NewReader(latin)})
		_, err := d.Token()
		out("no CharsetReader: %v", err)
	})
	// row: encoding/xml Unmarshal assert x.(io.ByteReader)
	// row: encoding/xml Unmarshal assert x.(io.Reader)
	// row: encoding/xml Unmarshal panic x.(io.Reader) without ok: *runtime.TypeAssertionError
	probe("Unmarshal/reader", func() {
		var v XM
		err := xml.Unmarshal([]byte(latin), &v)
		out("latin1: %+v %v", v, err)
		err = xml.Unmarshal([]byte(`<a>plain</a>`), &v)
		out("utf8: %+v %v", v, err)
	})
}
