// strings.Index, IndexByte, IndexRune (with RuneError and invalid runes),
// IndexAny, IndexFunc, LastIndex*, Contains*, ContainsFunc, HasPrefix,
// HasSuffix and Count, with empty and invalid UTF-8 input.
package main

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func main() {
	hay := []string{"", "a", "chicken", "héllo héllo", "a\xffb\xff", "aaaaaa", "日本語日本", strings.Repeat("x", 100) + "needle" + strings.Repeat("y", 50)}
	subs := []string{"", "a", "ken", "é", "llo", "\xff", "aa", "本", "needle", "zz"}
	for _, h := range hay {
		fmt.Printf("%.20q:", h)
		for _, s := range subs {
			fmt.Printf(" %d/%d/%d/%v", strings.Index(h, s), strings.LastIndex(h, s), strings.Count(h, s), strings.Contains(h, s))
		}
		fmt.Println()
		fmt.Printf("  any: %d %d %d %d %d | %v %v\n", strings.IndexAny(h, "éx"), strings.LastIndexAny(h, "éx"), strings.IndexAny(h, ""), strings.IndexAny(h, "\xff"), strings.LastIndexAny(h, "\xffa"), strings.ContainsAny(h, "本z"), strings.ContainsAny(h, ""))
		fmt.Printf("  rune: %d %d %d %d %d %d | byte %d %d | %v\n", strings.IndexRune(h, 'é'), strings.IndexRune(h, utf8.RuneError), strings.IndexRune(h, -1), strings.IndexRune(h, 0xd800), strings.IndexRune(h, '本'), strings.IndexRune(h, 'a'),
			strings.IndexByte(h, 'l'), strings.LastIndexByte(h, 'l'), strings.ContainsRune(h, '語'))
		fmt.Printf("  func: %d %d %d %d %v\n", strings.IndexFunc(h, unicode.IsUpper), strings.IndexFunc(h, func(r rune) bool { return r > 127 }), strings.LastIndexFunc(h, unicode.IsLetter), strings.LastIndexFunc(h, func(r rune) bool { return r == utf8.RuneError }), strings.ContainsFunc(h, unicode.IsDigit))
		fmt.Printf("  pre/suf: %v %v %v %v\n", strings.HasPrefix(h, "a"), strings.HasSuffix(h, "y"), strings.HasPrefix(h, ""), strings.HasSuffix(h, "\xff"))
	}
	fmt.Println(strings.Count("cheese", "e"), strings.Count("five", ""), strings.Count("", ""), strings.Count("日本", ""))
}
