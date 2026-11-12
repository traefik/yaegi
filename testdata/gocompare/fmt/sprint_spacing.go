// The spacing rules of fmt.Print/Sprint (a space only between two operands
// that are not strings) and Println/Sprintln (always a space), the
// Append* functions, Fprint* return counts and FormatString.
package main

import (
	"fmt"
	"strings"
)

type S string

type St struct{ A int }

type Strg int

func (s Strg) String() string { return "strg" }

func main() {
	cases := [][]any{
		{},
		{"a"},
		{"a", "b"},
		{1, 2},
		{1, "a", 2},
		{"a", 1, "b"},
		{1, 2, "c", 3, 4},
		{S("x"), S("y")},
		{S("x"), 1},
		{1.5, true, nil},
		{nil, nil},
		{[]int{1}, []int{2}},
		{"", ""},
		{"", 1, ""},
		{St{1}, St{2}},
		{&St{1}, "s", &St{2}},
		{Strg(1), Strg(2)},
		{'a', 'b'},
		{[]byte("x"), []byte("y")},
		{"x", []string{"y"}},
		{error(nil), 3},
		{true, false},
	}
	for _, c := range cases {
		fmt.Printf("%q %q\n", fmt.Sprint(c...), fmt.Sprintln(c...))
		fmt.Print(c...)
		fmt.Print("|")
		fmt.Println(c...)
	}
	b := fmt.Append(nil, 1, 2, "x", 3)
	b = fmt.Appendln(b, "y", 4)
	b = fmt.Appendf(b, "%03d|%v", 7, []int{1})
	fmt.Printf("%q\n", b)
	var sb strings.Builder
	n, err := fmt.Fprint(&sb, 1, 2, "é")
	fmt.Println(n, err)
	n, err = fmt.Fprintln(&sb, "a", 2)
	fmt.Println(n, err)
	n, err = fmt.Fprintf(&sb, "%5s|%d", "日本", 3)
	fmt.Println(n, err)
	fmt.Printf("%q\n", sb.String())
	n, err = fmt.Println("count", "me")
	fmt.Println(n, err)
	n, err = fmt.Printf("%d%%\n", 50)
	fmt.Println(n, err)
}
