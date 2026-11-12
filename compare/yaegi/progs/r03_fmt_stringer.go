package main

import (
	"errors"
	"fmt"
)

type Color int

func (c Color) String() string { return [...]string{"red", "green"}[c] }

type Pt struct {
	X, Y int
	C    Color
}

type Wrap struct {
	P   *Pt
	Err error
}

type MyErr struct{ Code int }

func (e *MyErr) Error() string { return fmt.Sprintf("code %d", e.Code) }

func main() {
	p := Pt{1, 2, 1}
	fmt.Printf("%v|%+v|%s|%d\n", p, p, Color(0), Color(1))
	fmt.Printf("%v %+v\n", []Color{0, 1}, map[Color]int{1: 2})
	w := Wrap{&p, &MyErr{4}}
	fmt.Printf("%+v %v\n", w.Err, w.Err)
	fmt.Printf("%T %T\n", p, w.Err)
	fmt.Println(fmt.Errorf("wrap: %w", &MyErr{5}), errors.New("x"))
	fmt.Printf("%#v\n", Pt{3, 4, 0})
}
