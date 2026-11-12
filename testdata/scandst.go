// Native functions that fill their pointer arguments (fmt.Sscan family,
// binary.Read, asn1.Unmarshal) with values whose type has methods: String,
// Error, a pointer String, and fmt.Scanner. The pointer
// must reach native code as a pointer, not as a hand wrapper
// ("type not a pointer: vm.stringerW"), and fmt calls the Scan method.
// Encoders that walk the value with reflect (binary.Write, asn1.Marshal)
// must see its fields, not the wrapper. A nil pointer gives the error of
// go run, and binary.Read fills a slice passed directly, also with -opaque.
package main

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

type Color int

func (c Color) String() string { return [...]string{"Red", "Green", "Blue"}[c] }

type Code int

func (c Code) Error() string { return fmt.Sprintf("code %d", int(c)) }

type Both int

func (b Both) Error() string  { return "err" }
func (b Both) String() string { return fmt.Sprintf("both %d", int(b)) }

type Name string

func (n *Name) String() string { return "name:" + string(*n) }

// Upper has a Scan method only: fmt must call it.
type Upper string

func (u *Upper) Scan(st fmt.ScanState, verb rune) error {
	tok, err := st.Token(true, nil)
	if err != nil {
		return err
	}
	*u = Upper(strings.ToUpper(string(tok)))
	return nil
}

// Tagged has a Scan method and a String method.
type Tagged string

func (t Tagged) String() string { return "tag(" + string(t) + ")" }

func (t *Tagged) Scan(st fmt.ScanState, verb rune) error {
	tok, err := st.Token(true, nil)
	if err != nil {
		return err
	}
	*t = Tagged(string(tok) + "/" + string(verb))
	return nil
}

// Picky refuses some input.
type Picky int

func (p *Picky) Scan(st fmt.ScanState, verb rune) error {
	tok, err := st.Token(true, nil)
	if err != nil {
		return err
	}
	if string(tok) != "ok" {
		return errors.New("picky: " + string(tok))
	}
	*p = 1
	return nil
}

type Pair struct{ A, B int16 }

func (p Pair) String() string { return fmt.Sprintf("<%d,%d>", p.A, p.B) }

type Point struct{ X, Y int }

func (p Point) String() string { return fmt.Sprintf("(%d;%d)", p.X, p.Y) }

// Count has no methods.
type Count int

func try(name string, f func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	f()
}

func main() {
	var c Color
	n, err := fmt.Sscan("2", &c)
	fmt.Println("Sscan:", n, err, c)

	var c2 Color
	var e Code
	var name string
	var f float64
	n, err = fmt.Sscanf("1 7 bob 1.5", "%d %d %s %g", &c2, &e, &name, &f)
	fmt.Println("Sscanf:", n, err, c2, int(e), name, f)

	var b Both
	var nm Name
	n, err = fmt.Sscanln("3 bob\n", &b, &nm)
	fmt.Println("Sscanln:", n, err, b, nm.String())

	var c3 Color
	var x int
	n, err = fmt.Fscan(strings.NewReader("1 9"), &c3, &x)
	fmt.Println("Fscan:", n, err, c3, x)
	n, err = fmt.Fscanf(strings.NewReader("0"), "%d", &c3)
	fmt.Println("Fscanf:", n, err, c3)
	n, err = fmt.Fscanln(strings.NewReader("2\n"), &c3)
	fmt.Println("Fscanln:", n, err, c3)

	// a parse error leaves the value as it was
	c3 = 1
	n, err = fmt.Sscan("x", &c3)
	fmt.Println("bad:", n, err != nil, c3)

	// fmt.Scanner methods
	var u Upper
	var t Tagged
	var i int
	n, err = fmt.Sscan("abc def 3", &u, &t, &i)
	fmt.Println("Scanner:", n, err, u, t, i)
	n, err = fmt.Sscanf("ghi", "%v", &t)
	fmt.Println("Scanner Sscanf:", n, err, t)
	var p Picky
	n, err = fmt.Sscan("ok", &p)
	fmt.Println("Picky ok:", n, err, p)
	n, err = fmt.Sscan("no", &p)
	fmt.Println("Picky no:", n, err)

	// the same pointer held by an interface variable
	var dst any = &c
	n, err = fmt.Sscan("0", dst)
	fmt.Println("iface:", n, err, c)
	args := []any{&c, &i}
	n, err = fmt.Sscan("1 4", args...)
	fmt.Println("spread:", n, err, c, i)

	// binary.Read and binary.Decode
	var pr Pair
	err = binary.Read(bytes.NewReader([]byte{1, 0, 2, 0}), binary.LittleEndian, &pr)
	fmt.Println("binary.Read:", err, pr)
	var pr2 Pair
	n, err = binary.Decode([]byte{0, 3, 0, 4}, binary.BigEndian, &pr2)
	fmt.Println("binary.Decode:", n, err, pr2)

	// encoders that only read the value with reflect
	var wbuf bytes.Buffer
	err = binary.Write(&wbuf, binary.LittleEndian, Pair{5, 6})
	fmt.Println("binary.Write:", err, wbuf.Bytes(), binary.Size(Pair{}), binary.Size(&pr))
	enc, err := binary.Append(nil, binary.BigEndian, &pr2)
	fmt.Println("binary.Append:", err, enc)

	// asn1.Marshal and Unmarshal
	der, err := asn1.Marshal(Point{5, 6})
	fmt.Println("asn1.Marshal:", err, len(der))
	var pt Point
	rest, err := asn1.Unmarshal(der, &pt)
	fmt.Println("asn1.Unmarshal:", len(rest), err, pt)

	// nil pointers: go run says "can't scan type", the native copy (a nil
	// *int) must not reach fmt's fast path. A nil pointer with a Scan
	// method panics in the method, like go run.
	try("nil Color", func() {
		var nc *Color
		n, err := fmt.Sscan("5", nc)
		fmt.Println("nil Color:", n, err)
	})
	try("nil Count", func() {
		var nk *Count
		n, err := fmt.Sscanf("5", "%d", nk)
		fmt.Println("nil Count:", n, err)
	})
	try("nil Point", func() {
		var np *Point
		n, err := fmt.Sscan("5", np)
		fmt.Println("nil Point:", n, err)
	})
	try("nil Upper", func() {
		var nu *Upper
		n, err := fmt.Sscan("x", nu)
		fmt.Println("nil Upper:", n, err)
	})

	// binary.Read and binary.Decode into a slice passed directly
	us := make([]uint16, 2)
	err = binary.Read(bytes.NewReader([]byte{1, 0, 2, 0}), binary.LittleEndian, us)
	fmt.Println("binary.Read slice:", err, us)
	ps := make([]Pair, 2)
	err = binary.Read(bytes.NewReader([]byte{1, 0, 2, 0, 3, 0, 4, 0}), binary.LittleEndian, ps)
	fmt.Println("binary.Read []Pair:", err, ps)
	n, err = binary.Decode([]byte{9, 0, 8, 0, 7, 0, 6, 0}, binary.LittleEndian, ps)
	fmt.Println("binary.Decode []Pair:", n, err, ps)

	// printing still uses the methods
	fmt.Println("print:", c, &nm, e, b, t)
	fmt.Printf("%v %s %d\n", c, &nm, c)
}
