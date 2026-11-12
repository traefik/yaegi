// strings.Cut, CutPrefix, CutSuffix, Replace, ReplaceAll (empty old string
// and invalid UTF-8), Replacer (generic, byte and single-string cases, with
// overlapping patterns), Map (rune dropping and expansion).
package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

func main() {
	for _, c := range [][2]string{{"key=value", "="}, {"novalue", "="}, {"a=b=c", "="}, {"", ""}, {"abc", ""}, {"héllo", "é"}, {"x", "x"}} {
		b, a, ok := strings.Cut(c[0], c[1])
		p, okp := strings.CutPrefix(c[0], c[1])
		q, oks := strings.CutSuffix(c[0], c[1])
		fmt.Printf("%q %q: %q %q %v | %q %v | %q %v\n", c[0], c[1], b, a, ok, p, okp, q, oks)
	}
	ins := []string{"", "aaaa", "banana", "héllo", "a\xffb", "abcabc"}
	for _, s := range ins {
		for _, o := range []string{"", "a", "an", "ana", "\xff", "é"} {
			fmt.Printf("%q/%q: %q", s, o, strings.ReplaceAll(s, o, "<>"))
			for _, n := range []int{-1, 0, 1, 2} {
				fmt.Printf(" %q", strings.Replace(s, o, "_", n))
			}
			fmt.Println()
		}
	}
	reps := []*strings.Replacer{
		strings.NewReplacer("a", "1", "b", "2"),
		strings.NewReplacer("a", "A"),
		strings.NewReplacer("aaa", "3", "aa", "2", "a", "1"),
		strings.NewReplacer("a", "1", "aa", "2", "aaa", "3"),
		strings.NewReplacer("", "X"),
		strings.NewReplacer("", "X", "o", "0"),
		strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;"),
		strings.NewReplacer("hello", "bye"),
		strings.NewReplacer("é", "e", "ü", "u"),
		strings.NewReplacer("a", "b", "b", "a"),
		strings.NewReplacer(),
		strings.NewReplacer("ab", "", "c", ""),
	}
	for i, r := range reps {
		for _, s := range []string{"", "aaaaa", "abba", "hello world", "<a href=\"x\">&</a>", "héllo über", "cabcab"} {
			fmt.Printf("%d %q -> %q\n", i, s, r.Replace(s))
		}
		var buf bytes.Buffer
		n, err := r.WriteString(&buf, "a<b>aa")
		fmt.Println(i, n, err, buf.String())
	}
	func() {
		defer func() { fmt.Println("odd:", recover()) }()
		strings.NewReplacer("a")
	}()
	fmt.Println(strings.Map(unicode.ToUpper, "héllo ß"))
	fmt.Println(strings.Map(func(r rune) rune {
		if r == 'l' {
			return -1
		}
		if r == 'o' {
			return '😀'
		}
		return r
	}, "hello world"))
	fmt.Printf("%q\n", strings.Map(func(r rune) rune { return r }, "a\xffb"))
	fmt.Printf("%q\n", strings.Map(func(r rune) rune { return r + 1 }, "abc\xff"))
	fmt.Printf("%q\n", strings.Map(func(r rune) rune { return 0xd800 }, "ab"))
	fmt.Printf("%q\n", strings.Map(func(r rune) rune { return -1 }, "abc"))
}
