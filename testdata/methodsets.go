// Embedding and method sets: promoted methods and fields through value and
// pointer embedding, shadowing of promoted methods, interface embedding,
// method values and method expressions, interface satisfaction by T and *T,
// type switches with several types per case, interface to interface
// assertions, and comparisons of interface values.
package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Animal interface {
	Name() string
	Sound() string
}

type Walker interface{ Walk() string }

type WalkingAnimal interface {
	Animal
	Walker
}

type Base struct{ name string }

func (b Base) Name() string     { return b.name }
func (b *Base) Rename(n string) { b.name = n }
func (b Base) Sound() string    { return "..." }

type Dog struct {
	Base
	legs int
}

func (d Dog) Sound() string { return "woof" } // shadows Base.Sound
func (d *Dog) Walk() string { return fmt.Sprintf("%s walks on %d legs", d.name, d.legs) }

type Robot struct {
	*Base // pointer embedding
	model string
}

type Logger struct {
	strings.Builder // embedded native type: its methods are promoted
	prefix          string
}

func (l *Logger) Log(s string) { l.WriteString(l.prefix + s + "\n") }

type ReadCounter struct {
	io.Reader // embedded interface
	n         int
}

func (r *ReadCounter) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n += n
	return n, err
}

type Inner struct{ X int }

func (i Inner) Get() int    { return i.X }
func (i *Inner) Inc()       { i.X++ }
func (i Inner) Who() string { return "inner" }

type Middle struct {
	Inner
	Y int
}

func (m Middle) Who() string { return "middle" }

type Outer struct {
	Middle
	Z int
}

type Shape interface{ Area() float64 }
type Square struct{ s float64 }
type Circle struct{ r float64 }

func (s Square) Area() float64  { return s.s * s.s }
func (c *Circle) Area() float64 { return 3 * c.r * c.r }

type MyErr struct{ msg string }

func (e MyErr) Error() string { return e.msg }

func classify(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case int, int64:
		return fmt.Sprintf("integer %v (%T)", x, x)
	case error:
		return "error " + x.Error()
	case fmt.Stringer:
		return "stringer " + x.String()
	case WalkingAnimal:
		return "walking " + x.Name()
	case Animal:
		return "animal " + x.Name()
	case []any:
		return fmt.Sprint("slice of ", len(x))
	case func() int:
		return fmt.Sprint("func ", x())
	default:
		return fmt.Sprintf("default %T", x)
	}
}

type strer string

func (s strer) String() string { return "strer(" + string(s) + ")" }

func main() {
	d := Dog{Base{"rex"}, 4}
	fmt.Println(d.Name(), d.Sound(), d.Base.Sound(), d.Walk())
	d.Rename("max") // addressable: (&d.Base).Rename
	fmt.Println(d.name)

	var a Animal = d // Dog value has Name and Sound
	fmt.Println(a.Name(), a.Sound())
	_, isWalker := a.(Walker) // Walk has a pointer receiver: Dog is not a Walker
	fmt.Println("Dog value is Walker:", isWalker)
	a = &d
	w, isWalker := a.(Walker)
	fmt.Println("*Dog is Walker:", isWalker, w.Walk())
	wa, ok := a.(WalkingAnimal)
	fmt.Println(ok, wa.Sound())

	r := Robot{&Base{"r2"}, "astromech"}
	r.Rename("r2d2")  // through the embedded pointer
	var a2 Animal = r // value Robot has *Base methods promoted
	fmt.Println(a2.Name(), a2.Sound(), r.Base.name)

	var lg Logger
	lg.prefix = "> "
	lg.Log("one")
	lg.Log("two")
	fmt.Print(lg.String())
	fmt.Println(lg.Len())

	rc := &ReadCounter{Reader: strings.NewReader("hello world")}
	b, _ := io.ReadAll(rc)
	fmt.Println(string(b), rc.n)

	o := Outer{Middle{Inner{1}, 2}, 3}
	o.Inc()
	fmt.Println(o.Get(), o.X, o.Who(), o.Inner.Who(), o.Middle.Inner.X)

	// Method values bind the receiver at evaluation time.
	in := Inner{10}
	get := in.Get // copies in
	inc := in.Inc // takes &in
	in.X = 20
	inc()
	fmt.Println(get(), in.X)

	// Method expressions.
	fget := Inner.Get
	finc := (*Inner).Inc
	fgetp := (*Inner).Get
	finc(&in)
	fmt.Println(fget(in), fgetp(&in))
	fsound := Animal.Sound
	fmt.Println(fsound(d), fsound(Base{"b"}))

	// Interface values with pointer and value receivers.
	shapes := []Shape{Square{2}, &Circle{1}, Square{1}}
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].Area() < shapes[j].Area() })
	for _, s := range shapes {
		fmt.Printf("%T %.1f; ", s, s.Area())
	}
	fmt.Println()

	// Interface comparison.
	var e1, e2 error = MyErr{"x"}, MyErr{"x"}
	fmt.Println(e1 == e2, e1 == MyErr{"y"}, errors.Is(e1, MyErr{"x"}))
	var s1, s2 Shape = &Circle{1}, &Circle{1}
	fmt.Println(s1 == s2, s1 == s1)
	var nilShape Shape
	var nilCircle *Circle
	fmt.Println(nilShape == nil, Shape(nilCircle) == nil, Shape(nilCircle) != nilShape)

	// Comparing interfaces that hold uncomparable values panics.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var x, y any = []int{1}, []int{1}
		fmt.Println(x == y)
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		m := map[any]int{}
		m[map[string]int{}] = 1
	}()

	for _, v := range []any{nil, 3, int64(4), MyErr{"bad"}, strer("s"), &d, Base{"plain"}, []any{1, 2}, func() int { return 9 }, 2.5, uint8(1)} {
		fmt.Println(classify(v))
	}

	// Assertion to a non-interface type that does not match.
	var anyv any = d
	_, ok = anyv.(*Dog)
	_, ok2 := anyv.(Dog)
	fmt.Println(ok, ok2)

	// Anonymous struct with embedded field and interface field.
	x := struct {
		Inner
		fmt.Stringer
	}{Inner{5}, strer("emb")}
	fmt.Println(x.Get(), x.String())
	var st fmt.Stringer = x // promoted through the embedded interface
	fmt.Println(st.String())
}
