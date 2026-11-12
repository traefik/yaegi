// The iterator functions of strings and bytes: Lines, SplitSeq,
// SplitAfterSeq, FieldsSeq and FieldsFuncSeq, with early break.
package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

func main() {
	for _, s := range []string{"", "one", "a\nb\nc", "a\nb\n", "\n\n", "x\r\ny", "no newline\nlast"} {
		fmt.Printf("%q lines:", s)
		for l := range strings.Lines(s) {
			fmt.Printf(" %q", l)
		}
		fmt.Print(" | blines:")
		for l := range bytes.Lines([]byte(s)) {
			fmt.Printf(" %q", l)
		}
		fmt.Println()
	}
	for _, c := range [][2]string{{"a,b,c", ","}, {"", ","}, {"abc", ""}, {",a,", ","}, {"héllo", ""}, {"a\xffb", ""}} {
		fmt.Printf("%q/%q:", c[0], c[1])
		for p := range strings.SplitSeq(c[0], c[1]) {
			fmt.Printf(" %q", p)
		}
		fmt.Print(" |")
		for p := range strings.SplitAfterSeq(c[0], c[1]) {
			fmt.Printf(" %q", p)
		}
		fmt.Print(" |")
		for p := range bytes.SplitSeq([]byte(c[0]), []byte(c[1])) {
			fmt.Printf(" %q", p)
		}
		fmt.Println()
	}
	for _, s := range []string{"", "  a  b c ", "x\u00a0y\u3000z", "\xff \xfe"} {
		fmt.Printf("%q:", s)
		for f := range strings.FieldsSeq(s) {
			fmt.Printf(" %q", f)
		}
		fmt.Print(" |")
		for f := range strings.FieldsFuncSeq(s, func(r rune) bool { return unicode.IsSpace(r) || r == 'b' }) {
			fmt.Printf(" %q", f)
		}
		fmt.Print(" |")
		for f := range bytes.FieldsSeq([]byte(s)) {
			fmt.Printf(" %q", f)
		}
		fmt.Println()
	}
	n := 0
	for p := range strings.SplitSeq("1,2,3,4,5,6", ",") {
		if p == "4" {
			break
		}
		n++
	}
	fmt.Println("before 4:", n)
	for l := range strings.Lines("a\nb\nc\n") {
		if l == "b\n" {
			continue
		}
		fmt.Printf("%q ", l)
	}
	fmt.Println()
	next := strings.FieldsSeq(" p q r ")
	got := []string{}
	next(func(s string) bool {
		got = append(got, s)
		return len(got) < 2
	})
	fmt.Println(got)
}
