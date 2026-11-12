// Slices of interfaces, funcs, pointers and channels: mixed dynamic types,
// == on elements, nil elements, type switches over elements, conversion of
// []T to []any by a loop, and %v / %#v / %T printing of mixed slices.
package main

import (
	"errors"
	"fmt"
	"strings"
)

type Shape interface{ Area() float64 }

type Sq struct{ S float64 }
type Rect struct{ W, H float64 }

func (s Sq) Area() float64    { return s.S * s.S }
func (r *Rect) Area() float64 { return r.W * r.H }

func main() {
	mixed := []any{1, "two", 3.0, []int{4}, map[string]int{"five": 5}, nil, Sq{2}, &Rect{1, 2}, errors.New("e"), true, 'x', byte(7)}
	for i, v := range mixed {
		switch t := v.(type) {
		case nil:
			fmt.Print(i, ":nil ")
		case int, float64:
			fmt.Printf("%d:num(%v) ", i, t)
		case string:
			fmt.Printf("%d:str(%d) ", i, len(t))
		case Shape:
			fmt.Printf("%d:shape(%v) ", i, t.Area())
		case error:
			fmt.Printf("%d:err(%v) ", i, t)
		default:
			fmt.Printf("%d:%T ", i, t)
		}
	}
	fmt.Println()
	fmt.Printf("%v\n", mixed[:3])
	fmt.Printf("%#v\n", mixed[:3])
	fmt.Println(mixed[0] == 1, mixed[2] == 3.0, mixed[2] == 3, mixed[5] == nil, mixed[6] == Sq{2})
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(mixed[3] == mixed[3])
	}()

	shapes := []Shape{Sq{1}, &Rect{2, 3}, Sq{3}}
	total := 0.0
	for _, s := range shapes {
		total += s.Area()
	}
	fmt.Println(total, shapes[0] == Sq{1}, shapes[1] == shapes[1])
	var nilShape Shape
	shapes = append(shapes, nilShape)
	fmt.Println(len(shapes), shapes[3] == nil)
	var rp *Rect
	shapes = append(shapes, rp)
	fmt.Println(shapes[4] == nil, shapes[4] != nil)

	// []string to []any.
	words := strings.Fields("a b c")
	anyWords := make([]any, len(words))
	for i, w := range words {
		anyWords[i] = w
	}
	fmt.Println(anyWords...)
	fmt.Println(fmt.Sprint(anyWords...), fmt.Sprintln(anyWords...) == "a b c\n")

	// Slice of funcs.
	var fns []func(int) int
	for i := range 4 {
		fns = append(fns, func(x int) int { return x*10 + i })
	}
	acc := 0
	for _, f := range fns {
		acc = f(acc)
	}
	fmt.Println(acc, fns[0] != nil)

	// Slice of pointers shares the targets.
	vals := []int{1, 2, 3}
	ptrs := make([]*int, len(vals))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	*ptrs[1] = 20
	fmt.Println(vals, *ptrs[1], ptrs[0] == &vals[0])
	vals = append(vals, 4)
	*ptrs[2] = 30
	fmt.Println(*ptrs[2])

	// Slice of channels.
	chans := make([]chan string, 3)
	for i := range chans {
		chans[i] = make(chan string, 1)
		chans[i] <- fmt.Sprint("c", i)
	}
	for _, c := range chans {
		fmt.Print(<-c, " ")
	}
	fmt.Println(chans[0] == chans[0], chans[0] == chans[1])

	// Slice of errors with errors.Join and Is.
	base := errors.New("base")
	errs := []error{fmt.Errorf("wrap: %w", base), nil, errors.New("other")}
	joined := errors.Join(errs...)
	fmt.Println(errors.Is(joined, base), strings.ReplaceAll(joined.Error(), "\n", "|"))

	// Slice of structs with an embedded interface.
	type named struct {
		Shape
		Name string
	}
	ns := []named{{Sq{2}, "sq"}, {&Rect{1, 5}, "rect"}}
	for _, n := range ns {
		fmt.Print(n.Name, "=", n.Area(), " ")
	}
	fmt.Println()
}
