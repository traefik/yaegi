// The unicode/utf8 functions on valid and invalid input: DecodeRune,
// DecodeLastRune, EncodeRune, AppendRune, RuneLen, Valid, FullRune,
// RuneCount, RuneStart, ValidRune, in string and []byte forms.
package main

import (
	"fmt"
	"unicode/utf8"
)

func main() {
	inputs := []string{"", "a", "é", "€", "😀", "\xff", "\xc3", "\xe2\x82", "\xf0\x9f\x98", "\xed\xa0\x80", "\xc0\x80", "a\xffb€", "\xf4\x8f\xbf\xbf", "\xf4\x90\x80\x80"}
	for _, s := range inputs {
		r, n := utf8.DecodeRuneInString(s)
		lr, ln := utf8.DecodeLastRuneInString(s)
		br, bn := utf8.DecodeRune([]byte(s))
		blr, bln := utf8.DecodeLastRune([]byte(s))
		fmt.Printf("%q: first %U/%d last %U/%d bytes %U/%d %U/%d valid=%v/%v full=%v/%v count=%d/%d start=%v\n",
			s, r, n, lr, ln, br, bn, blr, bln, utf8.ValidString(s), utf8.Valid([]byte(s)),
			utf8.FullRuneInString(s), utf8.FullRune([]byte(s)), utf8.RuneCountInString(s), utf8.RuneCount([]byte(s)),
			len(s) > 0 && utf8.RuneStart(s[len(s)-1]))
	}
	runes := []rune{0, 'a', 0x7f, 0x80, 0x7ff, 0x800, 0xd7ff, 0xd800, 0xdfff, 0xe000, 0xffff, 0x10000, 0x10ffff, 0x110000, -1, utf8.RuneError, utf8.MaxRune}
	for _, r := range runes {
		buf := make([]byte, 4)
		n := utf8.EncodeRune(buf, r)
		fmt.Printf("%d len=%d valid=%v enc=% x app=% x\n", r, utf8.RuneLen(r), utf8.ValidRune(r), buf[:n], utf8.AppendRune([]byte("<"), r))
	}
	fmt.Println(utf8.UTFMax, utf8.RuneSelf, utf8.MaxRune, utf8.RuneError)
	func() {
		defer func() { fmt.Println("recovered:", recover() != nil) }()
		var small []byte = make([]byte, 1)
		utf8.EncodeRune(small, '€')
	}()
	s := "héllo wörld €100 \xff end"
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		fmt.Printf("%c%d ", r, size)
		s = s[size:]
	}
	fmt.Println()
	b := []byte("añb€")
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		fmt.Printf("%c%d ", r, size)
		b = b[:len(b)-size]
	}
	fmt.Println()
}
