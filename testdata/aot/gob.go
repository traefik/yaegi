// AOT test program: encoding/gob on interpreted types.
//
//aot:lib encoding/gob
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Shape interface{ Area() float64 }

type Rect struct{ W, H float64 }

func (r Rect) Area() float64 { return r.W * r.H }

type Circle struct{ R float64 }

func (c *Circle) Area() float64 { return 3 * c.R * c.R }

type Level uint8

// Version uses a custom gob encoding.
type Version struct{ Major, Minor int }

func (v Version) GobEncode() ([]byte, error) { return []byte(fmt.Sprintf("%d.%d", v.Major, v.Minor)), nil }
func (v *Version) GobDecode(b []byte) error {
	_, err := fmt.Sscanf(string(b), "%d.%d", &v.Major, &v.Minor)
	return err
}

type Doc struct {
	Title  string
	Level  Level
	Tags   []string
	Attrs  map[string]int
	Shapes []Shape
	Ver    Version
	Next   *Doc
	secret int
}

func init() {
	gob.Register(Rect{})
	gob.Register(&Circle{})
}

func main() {
	if benchMain(benchOp) {
		return
	}
	d := Doc{Title: "t", Level: 3, Tags: []string{"a", "b"}, Attrs: map[string]int{"x": 1},
		Shapes: []Shape{Rect{2, 3}, &Circle{1}}, Ver: Version{1, 22}, Next: &Doc{Title: "child"}, secret: 5}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(d); err != nil {
		fmt.Println("encode:", err)
		return
	}
	if err := enc.Encode(Level(7)); err != nil {
		fmt.Println("encode2:", err)
	}
	dec := gob.NewDecoder(&buf)
	var e Doc
	err := dec.Decode(&e)
	fmt.Println(err, e.Title, e.Level, e.Tags, e.Attrs, len(e.Shapes), e.Ver, e.Next.Title, e.secret)
	for _, s := range e.Shapes {
		fmt.Printf("%T %v\n", s, s.Area())
	}
	var l Level
	fmt.Println(dec.Decode(&l), l)
	var bad struct{ Title int }
	var buf2 bytes.Buffer
	gob.NewEncoder(&buf2).Encode(d)
	fmt.Println(gob.NewDecoder(&buf2).Decode(&bad))
}

// benchOp: encode and decode a document.
func benchOp() {
	var buf bytes.Buffer
	d := Doc{Title: "t", Level: 3, Tags: []string{"a", "b"}, Attrs: map[string]int{"x": 1}, Shapes: []Shape{Rect{2, 3}}}
	if err := gob.NewEncoder(&buf).Encode(d); err != nil {
		panic(err)
	}
	var e Doc
	if err := gob.NewDecoder(&buf).Decode(&e); err != nil {
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
