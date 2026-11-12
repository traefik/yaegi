// The unicode/utf16 functions: Encode, Decode, EncodeRune, DecodeRune,
// IsSurrogate, AppendRune and RuneLen, with lone and swapped surrogates.
package main

import (
	"fmt"
	"unicode/utf16"
)

func main() {
	runes := []rune{'a', 'é', 0xffff, 0x10000, 0x1F600, 0x10ffff, 0xd800, 0xdc00, 0x110000, -1}
	for _, r := range runes {
		r1, r2 := utf16.EncodeRune(r)
		fmt.Printf("%d: pair %x %x surrogate=%v len=%d app=%x\n", r, r1, r2, utf16.IsSurrogate(r), utf16.RuneLen(r), utf16.AppendRune(nil, r))
	}
	enc := utf16.Encode([]rune("a😀é\U0010FFFF"))
	fmt.Printf("%x %d\n", enc, len(enc))
	fmt.Println(string(utf16.Decode(enc)))
	bad := [][]uint16{
		{0xd800},
		{0xdc00, 0xd800},
		{0xd83d, 0xde00},
		{0xd83d, 'x'},
		{'a', 0xdfff, 'b'},
		{0xd800, 0xd800, 0xdc00},
		{},
	}
	for _, u := range bad {
		d := utf16.Decode(u)
		fmt.Printf("%x -> %U %q\n", u, d, string(d))
	}
	fmt.Printf("%U %U %U\n", utf16.DecodeRune(0xd83d, 0xde00), utf16.DecodeRune('a', 'b'), utf16.DecodeRune(0xdc00, 0xd800))
	fmt.Println(utf16.Encode(nil) == nil, len(utf16.Encode([]rune{})))
}
