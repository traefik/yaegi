// Conversions between string, []byte and []rune, with invalid UTF-8,
// surrogate halves and out-of-range runes coming from variables.
package main

import "fmt"

func main() {
	inputs := []string{"", "abc", "héllo", "世界", "\xff", "a\xffb", "\xe4\xb8", "\xed\xa0\x80", "\xf4\x90\x80\x80", "\xc0\xaf", "\U0010FFFF", "\x00z"}
	for _, s := range inputs {
		rs := []rune(s)
		bs := []byte(s)
		fmt.Printf("%q len=%d runes=%d %v %v back=%q bytesback=%q\n", s, len(s), len(rs), rs, bs, string(rs), string(bs))
	}
	vals := []int64{-1, 0, 65, 0x7f, 0x80, 0xe9, 0xd7ff, 0xd800, 0xdbff, 0xdc00, 0xdfff, 0xe000, 0xfffd, 0xffff, 0x10ffff, 0x110000, 1 << 31, -1 << 31}
	for _, v := range vals {
		r := rune(v)
		s := string(r)
		fmt.Printf("%d %q %v %d\n", v, s, []byte(s), len(s))
	}
	for _, v := range []int{65, 0x10000, -5} {
		fmt.Printf("%q ", string(rune(v)))
	}
	fmt.Println()
	var u8 uint8 = 200
	var u16 uint16 = 0xd801
	var u32 uint32 = 0xffffffff
	var i64 int64 = 0x1F600
	fmt.Printf("%q %q %q %q\n", string(rune(u8)), string(rune(u16)), string(rune(u32)), string(rune(i64)))
	rs := []rune{'a', -1, 0xd800, 0x110000, 'é', 0x10ffff}
	s := string(rs)
	fmt.Printf("%q %d %v\n", s, len(s), []rune(s))
	b := []byte("hello")
	s2 := string(b)
	b[0] = 'J'
	fmt.Println(s2, string(b))
	var nb []byte
	var nr []rune
	fmt.Printf("%q %q %v %v\n", string(nb), string(nr), []byte(""), []rune("") == nil)
	fmt.Println([]byte("") == nil, nb == nil, len([]byte(nil)))
	type B []byte
	type R []rune
	type S string
	fmt.Printf("%v %v %q %q\n", B("xy"), R("xé"), S(B("ab")), S(R{'z', 'é'}))
	bb := append([]byte("ab"), "cdé"...)
	fmt.Println(bb, string(bb))
	cp := make([]byte, 3)
	n := copy(cp, "hello")
	fmt.Println(n, string(cp))
	for _, x := range []byte("\x80\xff") {
		fmt.Print(x, " ", string(rune(x)), " ")
	}
	fmt.Println()
}
