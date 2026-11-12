// text/scanner: tokens of a Go-like source (identifiers, numbers, strings,
// raw strings, chars, comments), Mode and Whitespace settings, positions,
// Peek and Next, the error handler and custom IsIdentRune.
package main

import (
	"fmt"
	"strings"
	"text/scanner"
)

const src = `// comment
package main
x := 3.14e+2 + 0x1F - 0b101 * 'c' /* block
comment */ y = "str\n" + ` + "`raw`" + ` + 1_000 .5 07 0o17 1e 'ab' "bad
z_é := a.b; ! # @
`

func main() {
	var s scanner.Scanner
	s.Init(strings.NewReader(src))
	s.Filename = "input"
	var errs []string
	s.Error = func(s *scanner.Scanner, msg string) { errs = append(errs, fmt.Sprintf("%s: %s", s.Pos(), msg)) }
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		fmt.Printf("%s %s %q\n", s.Position, scanner.TokenString(tok), s.TokenText())
	}
	fmt.Println(s.ErrorCount, errs)

	s.Init(strings.NewReader(src))
	s.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanComments
	s.Error = func(*scanner.Scanner, string) {}
	n := 0
	for tok := s.Scan(); tok != scanner.EOF && n < 25; tok = s.Scan() {
		fmt.Printf("%s:%q ", scanner.TokenString(tok), s.TokenText())
		n++
	}
	fmt.Println()

	s.Init(strings.NewReader("a b\nc\td"))
	s.Whitespace ^= 1<<'\t' | 1<<'\n'
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		fmt.Printf("%q ", s.TokenText())
	}
	fmt.Println()

	s.Init(strings.NewReader("ab€c"))
	fmt.Printf("%q %q %q %q %q %q\n", s.Peek(), s.Next(), s.Peek(), s.Next(), s.Next(), s.Next())
	fmt.Println(s.Next() == scanner.EOF, s.Pos())

	s.Init(strings.NewReader("$var-name x-1 $"))
	s.IsIdentRune = func(ch rune, i int) bool {
		return ch == '$' && i == 0 || ch == '-' && i > 0 || 'a' <= ch && ch <= 'z' || '0' <= ch && ch <= '9' && i > 0
	}
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		fmt.Printf("%s:%q ", scanner.TokenString(tok), s.TokenText())
	}
	fmt.Println()
	for _, t := range []rune{scanner.EOF, scanner.Ident, scanner.Int, scanner.Float, scanner.Char, scanner.String, scanner.RawString, scanner.Comment, 'x', '\n', 0} {
		fmt.Print(scanner.TokenString(t), " ")
	}
	fmt.Println()
	var p scanner.Position
	fmt.Println(p.IsValid(), p.String())
	p = scanner.Position{Filename: "f", Line: 2, Column: 3}
	fmt.Println(p.IsValid(), p)
}
