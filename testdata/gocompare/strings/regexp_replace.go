// regexp replacement: ReplaceAllString with $1, ${name}, $$ and unknown
// groups, ReplaceAllLiteralString, ReplaceAllStringFunc, the []byte forms,
// Expand and ExpandString with templates, on empty matches too.
package main

import (
	"fmt"
	"regexp"
	"strings"
)

func main() {
	re := regexp.MustCompile(`(?P<first>\w+)\s(?P<last>\w+)`)
	in := "John Smith, Jane Doe, x"
	tmpls := []string{"$last $first", "${last}_$first", "$1x", "${1}x", "$$1", "$9", "$name", "${first", "$", "$last.$first", "[$0]", "${2}${1}"}
	for _, t := range tmpls {
		fmt.Printf("%q -> %q | %q\n", t, re.ReplaceAllString(in, t), re.ReplaceAllLiteralString(in, t))
	}
	fmt.Println(re.ReplaceAllStringFunc(in, strings.ToUpper))
	fmt.Printf("%q\n", re.ReplaceAll([]byte(in), []byte("<$2>")))
	fmt.Printf("%q\n", re.ReplaceAllLiteral([]byte(in), []byte("<$2>")))
	fmt.Printf("%q\n", re.ReplaceAllFunc([]byte(in), func(b []byte) []byte { return []byte(fmt.Sprint(len(b))) }))
	var dst []byte
	for _, sm := range re.FindAllStringSubmatchIndex(in, -1) {
		dst = re.ExpandString(dst, "$last:${first};", in, sm)
	}
	fmt.Println(string(dst))
	dst = re.Expand([]byte(">"), []byte("$2 $1 $3"), []byte(in), re.FindSubmatchIndex([]byte(in)))
	fmt.Println(string(dst))
	e := regexp.MustCompile(`a*`)
	fmt.Printf("%q %q\n", e.ReplaceAllString("baaac", "-"), e.ReplaceAllString("", "-"))
	fmt.Printf("%q\n", regexp.MustCompile(``).ReplaceAllString("héllo", "|"))
	fmt.Printf("%q\n", regexp.MustCompile(`.`).ReplaceAllString("a\xffé", "[$0]"))
	fmt.Printf("%q\n", regexp.MustCompile(`(a)|(b)`).ReplaceAllString("ab", "<$1|$2>"))
	n := 0
	out := regexp.MustCompile(`\d+`).ReplaceAllStringFunc("a1b22c333", func(s string) string {
		n++
		return strings.Repeat("#", len(s)) + fmt.Sprint(n)
	})
	fmt.Println(out, n)
	ws := regexp.MustCompile(`\s+`)
	fmt.Printf("%q\n", ws.ReplaceAllString("  many   spaces\there\n", " "))
}
