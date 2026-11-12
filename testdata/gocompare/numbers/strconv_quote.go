// strconv quoting and unquoting of numbers, runes and strings: Quote,
// QuoteToASCII, QuoteToGraphic, QuoteRune, Unquote, UnquoteChar,
// IsPrint, CanBackquote, with escapes of control and non-ASCII runes.
package main

import (
	"fmt"
	"strconv"
)

func main() {
	strs := []string{"", "abc", "a\"b", "tab\there", "nl\n", "\x00\x7f", "é☺", " ", " ", "\xff\xfe", "`back`", "1234567890", "\U0001F600", "\\"}
	for _, s := range strs {
		fmt.Println(strconv.Quote(s), strconv.QuoteToASCII(s), strconv.QuoteToGraphic(s), strconv.CanBackquote(s))
	}
	for _, r := range []rune{'a', '\'', '"', '\n', 0, 0x7f, 'é', '☺', 0x10FFFF, -1, 0xD800, 0x1F600, 0xa0} {
		fmt.Println(r, strconv.QuoteRune(r), strconv.QuoteRuneToASCII(r), strconv.QuoteRuneToGraphic(r), strconv.IsPrint(r), strconv.IsGraphic(r))
	}
	for _, q := range []string{`"abc"`, `'a'`, "`raw\\n`", `"\x41\101A\U00000041"`, `"\a\b\f\n\r\t\v\\\""`, `'\''`,
		`"unterminated`, `'ab'`, `"\400"`, `"\xZZ"`, `"\uD800"`, `abc`, `""`, `''`, `"é"`, `"\'"`, `'\"'`} {
		s, err := strconv.Unquote(q)
		fmt.Printf("%s -> %q %v\n", q, s, err)
	}
	s := `\x41☺rest`
	for len(s) > 0 {
		v, mb, tail, err := strconv.UnquoteChar(s, '"')
		if err != nil {
			fmt.Println("err", err)
			break
		}
		fmt.Println(v, mb, tail)
		s = tail
	}
	_, _, _, err := strconv.UnquoteChar(`\'`, '"')
	fmt.Println(err)
	pref, err := strconv.QuotedPrefix(`"abc\"d" tail`)
	fmt.Println(pref, err)
	_, err = strconv.QuotedPrefix(`abc`)
	fmt.Println(err)
}
