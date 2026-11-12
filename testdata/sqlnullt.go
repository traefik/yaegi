// database/sql.Null[T] for a T of the program that the native instances
// of Null do not cover: a struct that implements sql.Scanner and
// driver.Valuer, a named type with a Value method only, a named type with
// other methods, and a struct without them (the errors of database/sql).
package main

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
)

// point is stored as "x,y".
type point struct{ X, Y int }

func (p *point) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("point: cannot scan %T", src)
	}
	_, err := fmt.Sscanf(s, "%d,%d", &p.X, &p.Y)
	return err
}

func (p point) Value() (driver.Value, error) {
	if p.X < 0 {
		return nil, errors.New("point: negative x")
	}
	return fmt.Sprintf("%d,%d", p.X, p.Y), nil
}

// upper has a Value method only: Scan converts like for a string.
type upper string

func (u upper) Value() (driver.Value, error) { return strings.ToUpper(string(u)), nil }

// label has a method that database/sql does not look for.
type label string

func (l label) String() string { return "label:" + string(l) }

// plain is a struct without Scan or Value.
type plain struct{ N int }

func main() {
	var p sql.Null[point]
	fmt.Println("Scan point:", p.Scan("3,4"), p.V, p.Valid)
	v, err := p.Value()
	fmt.Printf("Value point: %#v %v\n", v, err)
	fmt.Println("Scan point error:", p.Scan(42), p.Valid)
	p.V.X = -1
	v, err = p.Value()
	fmt.Println("Value point error:", v, err)
	fmt.Println("Scan point nil:", p.Scan(nil), p.V, p.Valid)
	v, err = p.Value()
	fmt.Println("Value point invalid:", v, err)

	var u sql.Null[upper]
	fmt.Println("Scan upper:", u.Scan([]byte("abc")), u.V, u.Valid)
	v, err = u.Value()
	fmt.Printf("Value upper: %#v %v\n", v, err)

	var l sql.Null[label]
	fmt.Println("Scan label:", l.Scan("x"), l.V, l.Valid)
	v, err = l.Value()
	fmt.Printf("Value label: %#v %v\n", v, err)

	var s sql.Null[plain]
	fmt.Println("Scan plain:", s.Scan("x"), s.Valid)
	fmt.Println("Scan plain bytes:", s.Scan([]byte("x")))
	v, err = s.Value()
	fmt.Println("Value plain:", v, err)
	fmt.Println("Scan plain nil:", s.Scan(nil), s.V, s.Valid)
	// A value of T, or of a struct type with the same fields, is assigned
	// (convertAssign: AssignableTo, then ConvertibleTo for the same kind).
	fmt.Println("Scan plain value:", s.Scan(plain{3}), s.V, s.Valid)
	fmt.Println("Scan plain same fields:", s.Scan(other{4}), s.V, s.Valid)
	fmt.Println("Scan plain pointer:", s.Scan(&plain{5}), s.V)
}

// other has the fields of plain: a value of other converts to plain.
type other struct{ N int }
