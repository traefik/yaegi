// strings.ToUpper, ToLower, ToTitle, ToValidUTF8, the *Special variants with
// Turkish case, EqualFold with Unicode folding, and Compare.
package main

import (
	"fmt"
	"strings"
	"unicode"
)

func main() {
	ins := []string{"", "hello", "HELLO", "Hello World", "héllo ÉCOLE", "straße", "ǆemal ǅ", "İstanbul ıi", "ﬀ ﬁ", "σς Σ", "\xffabc\xfe", "1234!?", "ΑΒΓ αβγ", "ꙋ", "ᾳ ᾼ", "K Å"}
	for _, s := range ins {
		fmt.Printf("%q U%q L%q T%q TU%q TL%q TT%q V%q\n", s, strings.ToUpper(s), strings.ToLower(s), strings.ToTitle(s),
			strings.ToUpperSpecial(unicode.TurkishCase, s), strings.ToLowerSpecial(unicode.TurkishCase, s),
			strings.ToTitleSpecial(unicode.TurkishCase, s), strings.ToValidUTF8(s, "?"))
	}
	fmt.Printf("%q %q %q\n", strings.ToValidUTF8("a\xff\xfeb\xc3", "<bad>"), strings.ToValidUTF8("\xed\xa0\x80", ""), strings.ToValidUTF8("ok", "x"))
	pairs := [][2]string{{"Go", "GO"}, {"go", "gO"}, {"ß", "SS"}, {"K", "k"}, {"K", "K"}, {"σ", "ς"}, {"Σ", "ς"}, {"İ", "i"}, {"ı", "I"}, {"é", "É"}, {"a\xff", "A\xff"}, {"abc", "abcd"}, {"", ""}, {"ǅ", "ǆ"}, {"ſ", "S"}, {"µ", "Μ"}}
	for _, p := range pairs {
		fmt.Printf("%q~%q %v cmp=%d\n", p[0], p[1], strings.EqualFold(p[0], p[1]), strings.Compare(p[0], p[1]))
	}
	long := strings.Repeat("aBc", 100)
	fmt.Println(len(strings.ToUpper(long)), strings.ToLower(long)[:9], strings.EqualFold(long, strings.ToUpper(long)))
}
