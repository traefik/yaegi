// strings.Split, SplitN, SplitAfter, SplitAfterN, Fields and FieldsFunc
// with empty separators, empty input, negative and zero counts, and
// invalid UTF-8.
package main

import (
	"fmt"
	"strings"
	"unicode"
)

func main() {
	ins := []string{"", "a", "a,b,c", ",a,,b,", ",,,", "a, b ,c", "héllo,wörld", "\xff,\xfe", "abc"}
	seps := []string{",", "", ",,", "b", "xyz"}
	for _, s := range ins {
		for _, sep := range seps {
			fmt.Printf("%q|%q: %q %q", s, sep, strings.Split(s, sep), strings.SplitAfter(s, sep))
			for _, n := range []int{-1, 0, 1, 2, 3} {
				fmt.Printf(" %d%q%q", n, strings.SplitN(s, sep, n), strings.SplitAfterN(s, sep, n))
			}
			fmt.Println()
		}
	}
	fmt.Println(strings.Split("", "") == nil, len(strings.Split("", "")), strings.SplitN("a", ",", 0) == nil)
	fs := []string{"", "   ", " a b  c ", "\ta\nb\vc\fd\re\u0085f\u00a0g\u2000h", "one", "x\u3000y", "\xff \xfe"}
	for _, s := range fs {
		fmt.Printf("%q -> %q %q\n", s, strings.Fields(s), strings.FieldsFunc(s, unicode.IsSpace))
	}
	f := func(r rune) bool { return r == ',' || r == ';' || unicode.IsDigit(r) }
	for _, s := range []string{"a,b;c1d", "1,2;3", "", "x"} {
		fmt.Printf("%q -> %q\n", s, strings.FieldsFunc(s, f))
	}
	fmt.Printf("%q\n", strings.FieldsFunc("aXbXXc", func(r rune) bool { return r == 'X' }))
	big := strings.Repeat("ab,", 1000)
	parts := strings.Split(big, ",")
	fmt.Println(len(parts), parts[0], parts[999], parts[1000] == "")
	fmt.Println(strings.Join(parts[:5], "+"), strings.Join(nil, ","), strings.Join([]string{"x"}, ","), strings.Join([]string{"", ""}, "-"))
}
