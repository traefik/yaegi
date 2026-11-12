// AOT test program: fmt on interpreted types.
//
//aot:lib fmt
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Level int

func (l Level) String() string { return [...]string{"debug", "info", "warn"}[l] }

type Code uint8

type Celsius float64

func (c Celsius) Format(f fmt.State, verb rune) {
	prec, ok := f.Precision()
	if !ok {
		prec = 1
	}
	fmt.Fprintf(f, "%.*f°C(%c)", prec, float64(c), verb)
}

type Secret string

func (Secret) GoString() string { return `Secret("***")` }

type Point struct {
	X, Y int
}

func (p *Point) String() string { return fmt.Sprintf("P(%d,%d)", p.X, p.Y) }

type MyErr struct{ Op string }

func (e *MyErr) Error() string { return "myerr: " + e.Op }

type Inner struct {
	Name string
	lvl  Level
}

type Outer struct {
	Inner
	L     Level
	C     Code
	P     *Point
	Q     Point
	M     map[string]Level
	S     []Celsius
	I     any
	E     error
	F     func()
	Ch    chan int
	Arr   [2]Secret
	unexp int
}

type Words []string

func (w Words) String() string { return strings.Join(w, "+") }

func main() {
	if benchMain(benchOp) {
		return
	}
	l := Level(2)
	fmt.Printf("%v %s %d %T %q %x %08b\n", l, l, l, l, l, l, l)
	fmt.Printf("%v %d %T %#v\n", Code(7), Code(7), Code(7), Code(7))
	fmt.Printf("%v|%.3v|%s|%T\n", Celsius(21.5), Celsius(21.5), Celsius(1), Celsius(0))
	fmt.Printf("%v %#v %T\n", Secret("pw"), Secret("pw"), Secret(""))
	p := &Point{1, 2}
	fmt.Printf("%v %s %+v %T\n", p, p, *p, p)
	fmt.Println(Point{3, 4}, &Point{5, 6})
	var e error = &MyErr{"open"}
	fmt.Printf("%v|%s|%q|%T\n", e, e, e, e)
	o := Outer{
		Inner: Inner{Name: "in", lvl: 1},
		L:     1, C: 3, P: p, Q: Point{7, 8},
		M:     map[string]Level{"b": 1, "a": 2},
		S:     []Celsius{1, 2.25},
		I:     Words{"x", "y"},
		E:     e,
		Arr:   [2]Secret{"a", "b"},
		unexp: 9,
	}
	fmt.Printf("%v\n%+v\n", o, o)
	fmt.Printf("%#v\n", o.Inner)
	fmt.Printf("%v %v %d\n", o.M, o.S, o.S)
	fmt.Println(Words{"a"}, []Level{0, 1}, map[Level]Code{2: 1, 0: 9})
	fmt.Printf("%v %v\n", []any{l, Code(1), "s", 3, nil, e}, [3]Level{})
	fmt.Printf("%5s|%-6v|%+d|%x|%X|% x\n", l, Code(2), Level(1), "hi", []byte("hi"), Words{"ab"})
	fmt.Printf("%t %v %08.3f %e %g\n", true, 1.5, 3.14159, 1e6, Celsius(2))
	fmt.Printf("%c %U %q\n", 'é', 'é', 'é')
	fmt.Printf("%v %v\n", struct{ A, B any }{l, nil}, &struct{ X Level }{1})
	fmt.Printf("%!v %z %d\n", l, l)
	fmt.Printf("%d %d\n", 1)
	fmt.Println(fmt.Sprint(l, Code(1), "x", 2, 3), fmt.Sprintln(l, "y"))
	w := fmt.Errorf("ctx %d: %w", 3, e)
	fmt.Println(w, errors.Unwrap(w) == e)
	var me *MyErr
	fmt.Println(errors.As(w, &me), me.Op)
	w2 := fmt.Errorf("multi: %w + %w", e, os.ErrNotExist)
	fmt.Println(w2, errors.Is(w2, os.ErrNotExist))
	fmt.Fprintln(os.Stdout, "fprint", l)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%v-%v", l, p)
	fmt.Println(sb.String())

	var lv Level
	var name string
	var c Celsius
	n, err := fmt.Sscanf("2 bob 1.5", "%d %s %g", &lv, &name, &c)
	fmt.Println(n, err, lv, name, float64(c))
	var x, y int
	n, err = fmt.Sscan("10 20", &x, &y)
	fmt.Println(n, err, x+y)
	fmt.Println(fmt.Sprintf("%v", time.Duration(1500)*time.Millisecond))
	fmt.Println(fmt.Append([]byte("app:"), l, 1))
}

// benchOp: format a struct with Stringers and verbs.
func benchOp() {
	_ = fmt.Sprintf("%v %d %s %+v", Level(1), Code(2), Words{"a", "b"}, Point{1, 2})
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
