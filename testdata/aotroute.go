package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"time"
)

// Native fmt and encoding/xml called with interpreted types in default
// mode: %T, %#v and nested String methods see the interpreted type names
// and methods, and xml accepts an interpreted (generic) struct.

type Int int

func (i Int) String() string { return fmt.Sprintf("Int=%d", int(i)) }

type RWS struct{}

func (*RWS) Read(p []byte) (int, error) { return 0, nil }
func (*RWS) String() string             { return "rws" }

type Point struct {
	X, Y int
	Tag  Int
}

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

type Err struct{ code int }

func (e *Err) Error() string { return fmt.Sprintf("err %d", e.code) }

type A[T, U any] struct {
	Name T `xml:"name"`
	Data U `xml:"data"`
}

func main() {
	fmt.Printf("%T %v\n", Int(2), Int(2))
	fmt.Printf("%T %v %#v\n", &RWS{}, &RWS{}, &RWS{})
	var s fmt.Stringer = (*RWS)(nil)
	fmt.Printf("%T %#v\n", s, s)
	fmt.Printf("%T %#v\n", Pair[string, Int]{}, Pair[string, Int]{"a", 1})
	fmt.Printf("%v %+v\n", Point{1, 2, 3}, []Point{{4, 5, 6}})
	fmt.Println([]Int{1, 2}, map[string]Int{"k": 7})
	fmt.Println(fmt.Sprint(Int(8), " ", 9, " ", "x"))
	fmt.Fprintf(os.Stdout, "%T|%v\n", &Err{3}, &Err{3})
	var buf bytes.Buffer
	fmt.Fprintln(&buf, Point{7, 8, 9}, time.Duration(0))
	fmt.Print(buf.String())
	b := fmt.Appendf(nil, "%T", Int(0))
	fmt.Println(string(b))
	// basic and native values only: native fmt
	fmt.Printf("%T %T %v %d\n", 1, "s", time.Second, []int{1})

	err := fmt.Errorf("wrap: %w", &Err{4})
	fmt.Println(err, errors.Unwrap(err))

	src := &A[string, int]{Name: "name", Data: 1}
	data, err := xml.Marshal(src)
	fmt.Println(string(data), err)
	dst := &A[string, int]{}
	err = xml.Unmarshal(data, dst)
	fmt.Println(*dst == *src, err)
	out, _ := xml.MarshalIndent(Point{1, 2, 3}, "", " ")
	fmt.Println(string(out))
}
