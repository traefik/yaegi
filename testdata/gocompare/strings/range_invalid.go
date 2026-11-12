// Range over strings with valid and invalid UTF-8: the index, the rune
// (RuneError for each bad byte) and the step of each iteration.
package main

import "fmt"

func main() {
	inputs := []string{"", "abc", "héllo", "日本語", "\xff\xfe", "a\xc3", "a\xc3(b", "\xe2\x82", "\xe2\x82\xac", "\xed\xa0\x80x", "\xf0\x9f\x98\x80!", "\xf8\x88\x80\x80\x80", "\xc1\xbf", "\xf4\x90\x80\x80", "\x00\x7f\x80"}
	for _, s := range inputs {
		fmt.Printf("%q:", s)
		for i, r := range s {
			fmt.Printf(" %d:%U", i, r)
		}
		n := 0
		for range s {
			n++
		}
		last := -1
		for i := range s {
			last = i
		}
		fmt.Println(" count", n, "last", last)
	}
	s := "aé"
	for i, r := range s {
		s = "changed"
		fmt.Println(i, string(r))
	}
	fmt.Println(s)
	var bs []byte = []byte("x\xffé")
	for i, b := range bs {
		fmt.Print(i, ":", b, " ")
	}
	fmt.Println()
	for i, r := range string(bs) {
		fmt.Print(i, ":", r, " ")
	}
	fmt.Println()
	for i, r := range []rune(string(bs)) {
		fmt.Print(i, ":", r, " ")
	}
	fmt.Println()
	type MS string
	for i, r := range MS("ß\x80") {
		fmt.Print(i, r, ";")
	}
	fmt.Println()
	var r rune
	var i int
	for i, r = range "xyz" {
	}
	fmt.Println(i, r)
	for i := range 3 {
		fmt.Print(i)
	}
	fmt.Println()
}
