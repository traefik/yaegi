// fmt.Formatter on program types: the fmt.State seen by Format (Width,
// Precision, Flag, Write), fmt.FormatString rebuilding the directive,
// pointer and value receivers, Format called for every verb (even %d and
// %T is not), a Format that panics, and width padding counted in runes.
package main

import (
	"fmt"
	"strings"
)

type Money struct {
	Cents int64
	Cur   string
}

func (m Money) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v', 's':
		s := fmt.Sprintf("%d.%02d %s", m.Cents/100, m.Cents%100, m.Cur)
		if f.Flag('+') {
			s = "+" + s
		}
		if w, ok := f.Width(); ok && len(s) < w {
			if f.Flag('-') {
				s += strings.Repeat(" ", w-len(s))
			} else {
				s = strings.Repeat(" ", w-len(s)) + s
			}
		}
		f.Write([]byte(s))
	case 'd':
		fmt.Fprintf(f, "%d", m.Cents)
	default:
		fmt.Fprintf(f, "%%!%c(Money)", verb)
	}
}

type Echo struct{}

func (*Echo) Format(f fmt.State, verb rune) {
	fmt.Fprintf(f, "<%s>", fmt.FormatString(f, verb))
}

type Boom struct{}

func (Boom) Format(fmt.State, rune) { panic("format exploded") }

type Inner struct{ N int }

func (i Inner) Format(f fmt.State, verb rune) { fmt.Fprintf(f, "I%d/%c", i.N, verb) }

type Outer struct {
	In  Inner
	Ptr *Inner
	Tag string
}

func main() {
	m := Money{12345, "EUR"}
	fmt.Printf("%v|%s|%+v|%15v|%-15v|%d|%x|%q|%T\n", m, m, m, m, m, m, m, m, m)
	fmt.Println(m, Money{-5, "USD"})
	fmt.Print(m, m, "\n")
	fmt.Println(fmt.Sprint(m), fmt.Sprintf("%10.3v|", m))
	e := &Echo{}
	for _, f := range []string{"%v", "%+v", "%#v", "%-10.3x", "%08d", "% q", "%+#-0 12.4e", "%*d", "%.*s", "%[1]v", "%s", "%5.f", "%.d", "%c", "%U"} {
		fmt.Printf(f+" ", e, e)
		fmt.Println()
	}
	var ev Echo
	fmt.Printf("%v %s\n", ev, &ev)
	fmt.Println(Boom{})
	fmt.Printf("%d|%s\n", Boom{}, Boom{})
	o := Outer{In: Inner{1}, Tag: "t"}
	fmt.Printf("%v|%+v|%d|%s\n", o, o, o, o)
	fmt.Println(o, []Inner{{2}, {3}}, map[string]Inner{"k": {4}}, [1]Inner{{5}})
	fmt.Printf("%x %q\n", []Inner{{6}}, map[Inner]int{{7}: 1})
	op := &Outer{In: Inner{8}}
	fmt.Printf("%v %+v\n", op, op)
	fmt.Printf("[%5s][%-5s][%5q][%.2s][%5.1s]\n", "日本", "é", "ü", "日本語", "日本")
	fmt.Printf("[%6x][%-8X][%.1x][% .2x]\n", "日", "é", "日本", "abc")
	fmt.Printf("[%3c][%-3c][%03c][%05s][%-05d]\n", '日', 'é', 'x', "ab", 3)
	fmt.Printf("[%6v][%-6v][%06v]\n", []string{"a", "日"}, map[string]int{"é": 1}, []int{1, -2})
}
