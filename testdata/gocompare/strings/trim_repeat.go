// strings.Trim, TrimLeft, TrimRight, TrimSpace, TrimPrefix, TrimSuffix,
// TrimFunc and friends with ASCII and Unicode cutsets, and Repeat with
// zero, large and negative counts (the panic is recovered).
package main

import (
	"fmt"
	"strings"
	"unicode"
)

func main() {
	ins := []string{"", "  hello  ", "\t\n héllo \u00a0\u3000", "xxhixx", "ééaéé", "\xff\xffa\xff", "abcba", "   ", "a b"}
	cuts := []string{"", " ", "x", "é", "ab", "\xff", " \t\n\u00a0\u3000"}
	for _, s := range ins {
		fmt.Printf("%q TS%q", s, strings.TrimSpace(s))
		for _, c := range cuts {
			fmt.Printf(" [%q %q %q]", strings.Trim(s, c), strings.TrimLeft(s, c), strings.TrimRight(s, c))
		}
		fmt.Printf(" P%q S%q", strings.TrimPrefix(s, "xx"), strings.TrimSuffix(s, "xx"))
		fmt.Printf(" F%q %q %q\n", strings.TrimFunc(s, unicode.IsSpace), strings.TrimLeftFunc(s, unicode.IsLetter), strings.TrimRightFunc(s, func(r rune) bool { return r == 0xfffd || r == 'a' }))
	}
	fmt.Printf("%q %q %q %q\n", strings.Repeat("ab", 0), strings.Repeat("", 5), strings.Repeat("é-", 3), strings.Repeat("x", 1))
	fmt.Println(len(strings.Repeat("abc", 10000)))
	for _, n := range []int{-1, -100} {
		func() {
			defer func() { fmt.Println("repeat", n, "->", recover()) }()
			fmt.Println(strings.Repeat("a", n))
		}()
	}
	fmt.Println(strings.Clone("clone") == "clone", strings.Clone("") == "")
}
