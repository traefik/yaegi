// The unicode package: Is* class predicates, To/ToUpper/ToLower/ToTitle,
// SimpleFold orbits, In and Is with range tables, special cases (Turkish).
package main

import (
	"fmt"
	"unicode"
)

func main() {
	rs := []rune{'a', 'Z', '5', '٣', ' ', '\t', '\n', '\u00a0', '\u2028', '_', '!', '€', 'é', 'ß', 'ǅ', 'ǆ', 'Ǆ', 'İ', 'ı', 'K', 'Ω', 'ω', '世', '\u0300', '½', 'Ⅻ', '\x00', '\x7f', 0x10ffff, -1, 0x110000, '😀', 'ﬀ', 'ſ', 'µ'}
	for _, r := range rs {
		fmt.Printf("%U L%v U%v l%v T%v D%v N%v S%v P%v M%v C%v G%v Pr%v Sy%v | %U %U %U |",
			r, b(unicode.IsLetter(r)), b(unicode.IsUpper(r)), b(unicode.IsLower(r)), b(unicode.IsTitle(r)),
			b(unicode.IsDigit(r)), b(unicode.IsNumber(r)), b(unicode.IsSpace(r)), b(unicode.IsPunct(r)),
			b(unicode.IsMark(r)), b(unicode.IsControl(r)), b(unicode.IsGraphic(r)), b(unicode.IsPrint(r)), b(unicode.IsSymbol(r)),
			unicode.ToUpper(r), unicode.ToLower(r), unicode.ToTitle(r))
		f := unicode.SimpleFold(r)
		for i := 0; i < 5 && f != r; i++ {
			fmt.Printf(" %U", f)
			f = unicode.SimpleFold(f)
		}
		fmt.Println()
	}
	fmt.Printf("%U %U %U\n", unicode.To(unicode.UpperCase, 'q'), unicode.To(unicode.LowerCase, 'Q'), unicode.To(unicode.TitleCase, 'ǆ'))
	fmt.Printf("%U %U %U %U\n", unicode.TurkishCase.ToUpper('i'), unicode.TurkishCase.ToLower('I'), unicode.TurkishCase.ToLower('İ'), unicode.TurkishCase.ToTitle('i'))
	fmt.Println(unicode.In('世', unicode.Han, unicode.Latin), unicode.In('a', unicode.Han), unicode.Is(unicode.Greek, 'Ω'), unicode.Is(unicode.Cyrillic, 'Ω'))
	fmt.Println(unicode.IsOneOf([]*unicode.RangeTable{unicode.Digit, unicode.Letter}, '7'), unicode.IsOneOf(nil, '7'))
	fmt.Println(unicode.MaxRune, unicode.ReplacementChar, unicode.MaxASCII, unicode.MaxLatin1, unicode.UpperCase, unicode.LowerCase, unicode.TitleCase, unicode.MaxCase)
	n := 0
	for r := rune(0); r < 0x3000; r++ {
		if unicode.IsLetter(r) {
			n++
		}
	}
	fmt.Println("letters below 0x3000:", n)
}

func b(v bool) int {
	if v {
		return 1
	}
	return 0
}
