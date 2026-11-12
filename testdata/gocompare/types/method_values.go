// Method values (receiver bound when the method value is evaluated, value
// receiver copied, pointer receiver shared), method expressions T.M and
// (*T).M, method values of interfaces, and deferred method calls.
package main

import "fmt"

type T struct{ v int }

func (t T) Val() int             { return t.v }
func (t *T) Ptr() int            { return t.v }
func (t *T) Set(v int)           { t.v = v }
func (t T) Add(d int) int        { return t.v + d }
func (t T) Show(p string) string { return fmt.Sprintf("%s%d", p, t.v) }

type I interface{ Val() int }

func apply(f func() int) int { return f() }

func main() {
	t := T{1}
	fv := t.Val // copies t now
	fp := t.Ptr // &t now
	fs := (&t).Set
	t.v = 2
	fmt.Println(fv(), fp(), apply(fv), apply(fp))
	fs(42)
	fmt.Println(t.v, fv(), fp())

	pt := &T{10}
	pv := pt.Val // copies *pt now
	pt.v = 20
	fmt.Println(pv(), pt.Val())
	pt = &T{30} // the bound receiver does not change
	fmt.Println(pv())

	// Method expressions.
	val := T.Val
	ptr := (*T).Ptr
	vOnPtr := (*T).Val
	set := (*T).Set
	add := T.Add
	fmt.Println(val(T{3}), ptr(&T{4}), vOnPtr(&T{5}), add(T{6}, 7))
	x := T{}
	set(&x, 8)
	fmt.Println(x.v)
	show := T.Show
	fmt.Println(show(T{9}, "n="))

	// Method values of an interface: the dynamic value at evaluation.
	var i I = T{100}
	iv := i.Val
	i = T{200}
	fmt.Println(iv(), i.Val())
	ie := I.Val
	fmt.Println(ie(T{300}))

	// Method values in a slice and a map.
	a, b := T{1}, T{2}
	fs2 := []func() int{a.Val, b.Val, (&a).Ptr}
	a.v, b.v = 11, 22
	for _, f := range fs2 {
		fmt.Print(f(), " ")
	}
	fmt.Println()
	ops := map[string]func(int) int{"a": a.Add, "b": b.Add}
	fmt.Println(ops["a"](1), ops["b"](1))

	// Deferred method calls evaluate the receiver at defer time.
	func() {
		d := T{1}
		defer fmt.Println("deferred value:", d.Val())
		defer func() { fmt.Println("closure sees:", d.Val()) }()
		dv := d.Val
		defer func() { fmt.Println("method value:", dv()) }()
		dp := &d
		defer func() { fmt.Println("pointer:", dp.Ptr()) }()
		d.v = 2
	}()

	// Nil pointer method value of a value method panics at evaluation.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var np *T
		f := np.Val
		fmt.Println("not reached", f())
	}()
	// Nil pointer method value of a pointer method is fine until the call.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var np *T
		f := np.Ptr
		fmt.Println("got method value")
		fmt.Println(f())
	}()
}
