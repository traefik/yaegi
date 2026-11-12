// AOT test program: encoding/xml on interpreted types.
//
//aot:lib encoding/xml
package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Color int

func (c Color) MarshalText() ([]byte, error) { return []byte([...]string{"red", "green"}[c]), nil }
func (c *Color) UnmarshalText(b []byte) error {
	switch string(b) {
	case "red":
		*c = 0
	case "green":
		*c = 1
	default:
		return fmt.Errorf("bad color %q", b)
	}
	return nil
}

type Money struct{ Cents int }

func (m Money) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(fmt.Sprintf("%d.%02d", m.Cents/100, m.Cents%100), start)
}

func (m *Money) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	a, b, _ := strings.Cut(s, ".")
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	m.Cents = x*100 + y
	return nil
}

type Address struct {
	City string `xml:"city"`
	Zip  string `xml:"zip,attr"`
}

type Item struct {
	SKU   string `xml:"sku,attr"`
	Name  string `xml:"name"`
	Price Money  `xml:"price"`
	Color Color  `xml:"color,attr"`
}

type Order struct {
	XMLName xml.Name `xml:"order"`
	ID      int      `xml:"id,attr"`
	Address
	Items   []Item   `xml:"items>item"`
	Note    string   `xml:",comment"`
	Tags    []string `xml:"tag"`
	Gift    *bool    `xml:"gift,omitempty"`
	Comment string   `xml:",chardata"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	yes := true
	o := Order{ID: 9, Address: Address{"Lyon", "69000"}, Note: " fragile ",
		Items: []Item{{"A1", "pen", Money{150}, 1}, {"B2", "ink", Money{1205}, 0}},
		Tags:  []string{"x", "y"}, Gift: &yes, Comment: "thanks"}
	b, err := xml.MarshalIndent(o, "", "  ")
	fmt.Println(string(b), err)
	var p Order
	err = xml.Unmarshal(b, &p)
	fmt.Println(err, p.ID, p.City, p.Zip, len(p.Items), p.Items[1].Price.Cents, p.Items[0].Color, *p.Gift, p.Tags, strings.TrimSpace(p.Comment))
	fmt.Printf("%+v\n", p.Items[0])
	err = xml.Unmarshal([]byte(`<order id="x"></order>`), &p)
	fmt.Println(err)
	err = xml.Unmarshal([]byte(`<order><items><item color="blue"/></items></order>`), &p)
	fmt.Println(err)
	d := xml.NewDecoder(strings.NewReader(`<a x="1"><b>t</b><!-- c --></a>`))
	for {
		tok, err := d.Token()
		if err != nil {
			fmt.Println("end:", err)
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			fmt.Println("start", t.Name.Local, len(t.Attr))
		case xml.EndElement:
			fmt.Println("end", t.Name.Local)
		case xml.CharData:
			fmt.Printf("chardata %q\n", string(t))
		case xml.Comment:
			fmt.Printf("comment %q\n", string(t))
		}
	}
	e := xml.NewEncoder(os.Stdout)
	e.Encode(struct {
		XMLName xml.Name `xml:"pair"`
		K       Color    `xml:"k"`
		V       []Money  `xml:"v"`
	}{K: 1, V: []Money{{1}, {250}}})
	fmt.Println()
}

var benchDoc = []byte(`<order id="9" zip="69000"><city>Lyon</city><items><item sku="A1" color="green"><name>pen</name><price>1.50</price></item><item sku="B2" color="red"><name>ink</name><price>12.05</price></item></items><tag>x</tag></order>`)

// benchOp: unmarshal then marshal an order.
func benchOp() {
	var o Order
	if err := xml.Unmarshal(benchDoc, &o); err != nil {
		panic(err)
	}
	if _, err := xml.Marshal(o); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
