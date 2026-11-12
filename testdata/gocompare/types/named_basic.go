// Named types of basic kinds with methods: Stringer on named ints
// (enum-like iota), named strings, floats, bools and bytes; arithmetic keeps
// the named type; conversions between named types; fmt verbs with Stringer.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
)

var dayNames = [...]string{"Sun", "Mon", "Tue", "Wed"}

func (d Weekday) String() string {
	if d < 0 || int(d) >= len(dayNames) {
		return fmt.Sprintf("Weekday(%d)", int(d))
	}
	return dayNames[d]
}

func (d Weekday) Next() Weekday { return (d + 1) % 4 }

type Flags uint8

const (
	FRead Flags = 1 << iota
	FWrite
	FExec
)

func (f Flags) String() string {
	var parts []string
	if f&FRead != 0 {
		parts = append(parts, "r")
	}
	if f&FWrite != 0 {
		parts = append(parts, "w")
	}
	if f&FExec != 0 {
		parts = append(parts, "x")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "")
}

type Name string

func (n Name) Upper() Name { return Name(strings.ToUpper(string(n))) }
func (n Name) Len() int    { return len(n) }

type Meters float64
type Feet float64

func (m Meters) ToFeet() Feet { return Feet(m * 3.28084) }
func (f Feet) String() string { return fmt.Sprintf("%.2fft", float64(f)) }

type On bool

func (o On) String() string {
	if o {
		return "ON"
	}
	return "OFF"
}

type Byte byte

func (b Byte) Hex() string { return fmt.Sprintf("%02x", byte(b)) }

type Days []Weekday

func (d Days) Len() int           { return len(d) }
func (d Days) Less(i, j int) bool { return d[i] < d[j] }
func (d Days) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func main() {
	fmt.Println(Sunday, Monday, Wednesday, Weekday(9), Weekday(-1))
	fmt.Printf("%v %s %d %q %x %T\n", Tuesday, Tuesday, Tuesday, Tuesday, Tuesday, Tuesday)
	fmt.Println(Wednesday.Next(), Monday+2, Wednesday-Monday)
	d := Monday
	d++
	d *= 1
	fmt.Println(d, []Weekday{Sunday, Tuesday}, map[Weekday]int{Monday: 1})
	fmt.Printf("%v %+v\n", struct{ D Weekday }{Tuesday}, struct{ D Weekday }{Tuesday})

	f := FRead | FExec
	fmt.Println(f, f|FWrite, f&^FRead, Flags(0), uint8(f))
	fmt.Printf("%v %d %08b\n", f, f, f)

	n := Name("gopher")
	fmt.Println(n.Upper(), n.Len(), n+"!", n[0], n[1:3], len(n))
	fmt.Printf("%T %T\n", n+"x", n[1:])
	fmt.Println(n < "z", n == "gopher", strings.Contains(string(n), "ph"))
	for i, r := range Name("hé") {
		fmt.Print(i, ":", string(r), " ")
	}
	fmt.Println()

	m := Meters(100)
	fmt.Println(m.ToFeet(), Feet(m), Meters(Feet(3)))
	fmt.Printf("%.3f %T\n", float64(m*2), m*2)

	var on On = true
	fmt.Println(on, !on, On(false), on == true)
	fmt.Printf("%v %t\n", on, on)

	bs := []Byte{1, 255, 16}
	for _, b := range bs {
		fmt.Print(b.Hex(), " ")
	}
	fmt.Println(bs)

	days := Days{Wednesday, Sunday, Tuesday, Monday}
	sort.Sort(days)
	fmt.Println(days)
	fmt.Println(sort.IsSorted(days))

	// Method expression and value on a named basic type.
	next := Weekday.Next
	fmt.Println(next(Tuesday), Sunday.Next())
	up := n.Upper
	n = "changed"
	fmt.Println(up())

	// Conversions between named types of the same underlying type.
	type Level int
	type Rank int
	l := Level(3)
	r := Rank(l) + 1
	w := Weekday(r)
	fmt.Println(l, r, w, int(w))
	fmt.Printf("%T %T %T\n", l, r, w)
}
