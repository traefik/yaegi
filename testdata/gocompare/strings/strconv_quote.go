// strconv quoting: Quote, QuoteToASCII, QuoteToGraphic, QuoteRune*,
// Append* forms, Unquote and UnquoteChar on valid and invalid literals,
// CanBackquote, IsPrint, IsGraphic and QuotedPrefix.
package main

import (
	"fmt"
	"strconv"
)

func main() {
	ins := []string{"", "hello", "héllo\n", "tab\there", "\x00\x01\x7f", "\xff\xfe", "\u00a0\u2028\ufeff", "日本", "😀", `back\slash "q"`, "'single'", "\a\b\f\r\v", "\U0010ffff", "\u0300"}
	for _, s := range ins {
		q := strconv.Quote(s)
		u, err := strconv.Unquote(q)
		fmt.Printf("%s %s %s bq=%v rt=%v %v\n", q, strconv.QuoteToASCII(s), strconv.QuoteToGraphic(s), strconv.CanBackquote(s), u == s, err)
	}
	for _, r := range []rune{'a', '\'', '"', '\n', 'é', 0x7f, 0xd800, 0x110000, -1, '😀', '\u00a0', 0xfffd} {
		fmt.Printf("%s %s %s print=%v graphic=%v\n", strconv.QuoteRune(r), strconv.QuoteRuneToASCII(r), strconv.QuoteRuneToGraphic(r), strconv.IsPrint(r), strconv.IsGraphic(r))
	}
	b := []byte("x:")
	b = strconv.AppendQuote(b, "a\"b")
	b = strconv.AppendQuoteToASCII(b, "é")
	b = strconv.AppendQuoteRune(b, 'ü')
	b = strconv.AppendQuoteRuneToASCII(b, 'ü')
	b = strconv.AppendQuoteToGraphic(b, "\u00a0")
	b = strconv.AppendQuoteRuneToGraphic(b, '\t')
	fmt.Println(string(b))
	lits := []string{`"abc"`, "`raw\\n`", `'a'`, `'ab'`, `''`, `"\x41\101é\U0001F600"`, `"\q"`, `"unterminated`, `"a"b"`, `abc`, `"\xff"`, `'\xff'`, `"\ud800"`, `"\400"`, "`a`b`", `"new
line"`, `'\''`, `"\'"`, `'\"'`, `"é"`, `""`, "``", `"`, "\"\x00\"", "`\r`"}
	for _, l := range lits {
		u, err := strconv.Unquote(l)
		p, perr := strconv.QuotedPrefix(l + "rest")
		fmt.Printf("%q -> %q %v | %q %v\n", l, u, err, p, perr)
	}
	for _, c := range []struct {
		s string
		q byte
	}{{`\n`, '"'}, {`\'`, '\''}, {`\'`, '"'}, {`\"`, '"'}, {`\x4`, '"'}, {`ébc`, '"'}, {`éx`, 0}, {`"`, '"'}, {`\777`, 0}, {"", '"'}} {
		v, mb, tail, err := strconv.UnquoteChar(c.s, c.q)
		fmt.Printf("%q %c -> %q %v %q %v\n", c.s, c.q, v, mb, tail, err)
	}
}
