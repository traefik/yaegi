// strings, bytes and unicode with program types: strings.NewReplacer,
// Builder through program Writers, Map and FieldsFunc with program funcs,
// EqualFold, Title-like casing, unicode tables, utf8 and utf16 decoding.
package main

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

type Slug string

func (s Slug) Normalize() Slug {
	return Slug(strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			return unicode.ToLower(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			return '-'
		}
		return -1
	}, string(s)))
}

type Censor struct{ words []string }

func (c Censor) Replacer() *strings.Replacer {
	var pairs []string
	for _, w := range c.words {
		pairs = append(pairs, w, strings.Repeat("*", utf8.RuneCountInString(w)))
	}
	return strings.NewReplacer(pairs...)
}

type classifier func(rune) bool

func (c classifier) count(s string) (n int) {
	for _, r := range s {
		if c(r) {
			n++
		}
	}
	return
}

func main() {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;")
	fmt.Println(r.Replace("<a href='x'>&</a>"))
	r2 := strings.NewReplacer("a", "1", "aa", "2", "aaa", "3")
	fmt.Println(r2.Replace("aaaa aaa aa a"))
	r3 := strings.NewReplacer("", "-")
	fmt.Println(r3.Replace("abc"))
	r4 := strings.NewReplacer("hello", "bye", "he", "she", "h", "H")
	fmt.Println(r4.Replace("hello hell he h"))
	var sb strings.Builder
	n, err := r.WriteString(&sb, "x<y & y>z")
	fmt.Println(n, err, sb.String())
	c := Censor{[]string{"darn", "héck", "dang"}}
	fmt.Println(c.Replacer().Replace("darn it, héck, dangling darn"))
	for _, s := range []Slug{"Hello World!", "  Go_VM -- 1.27 ", "Ünïcödé Straße", "日本語 テキスト"} {
		fmt.Printf("%q -> %q\n", s, s.Normalize())
	}
	fmt.Printf("%q\n", strings.FieldsFunc("a1b22,c;;d", func(r rune) bool { return !unicode.IsLetter(r) }))
	fmt.Println(strings.EqualFold("Straße", "STRASSE"), strings.EqualFold("Gö", "GÖ"), strings.EqualFold("k", "K"))
	fmt.Println(strings.ToUpper("straße ǆ ﬀ"), strings.ToLower("İSTANBUL ΣΑΣ"), strings.ToTitle("ǆemal"))
	fmt.Println(strings.ToUpperSpecial(unicode.TurkishCase, "istanbul"), strings.ToLowerSpecial(unicode.TurkishCase, "ISTANBUL"))
	s := "Héllo, 世界! 123 \t x​"
	for _, cl := range []struct {
		name string
		c    classifier
	}{{"letter", unicode.IsLetter}, {"upper", unicode.IsUpper}, {"digit", unicode.IsDigit}, {"space", unicode.IsSpace}, {"punct", unicode.IsPunct}, {"han", func(r rune) bool { return unicode.Is(unicode.Han, r) }}, {"graphic", unicode.IsGraphic}, {"print", unicode.IsPrint}, {"control", unicode.IsControl}, {"latin", func(r rune) bool { return unicode.In(r, unicode.Latin) }}} {
		fmt.Print(cl.name, "=", cl.c.count(s), " ")
	}
	fmt.Println()
	fmt.Println(utf8.RuneCountInString(s), len(s), utf8.ValidString(s), utf8.ValidString("a\xffb"), utf8.RuneLen('世'), utf8.RuneLen(-1))
	for i, r := range "a\xffé\xe4\xb8" {
		fmt.Print(i, ":", r, " ")
	}
	fmt.Println()
	b := []byte("€uro")
	r0, size := utf8.DecodeRune(b)
	rl, sizel := utf8.DecodeLastRuneInString("abc€")
	fmt.Println(r0, size, rl, sizel, utf8.FullRune(b[:2]), utf8.AppendRune(nil, 0x1F600), string(utf8.AppendRune([]byte("x"), 'ÿ')))
	u16 := utf16.Encode([]rune("a😀é"))
	fmt.Println(u16, string(utf16.Decode(u16)), utf16.IsSurrogate(rune(u16[1])), utf16.RuneLen('😀'))
	fmt.Println(unicode.SimpleFold('A'), unicode.SimpleFold('k'), unicode.SimpleFold('K'), unicode.To(unicode.TitleCase, 'ǆ'), unicode.IsTitle('ǅ'))
	fmt.Println(strings.Title("hello wide world"), strings.Count("cheese", "e"), strings.Count("five", ""), strings.LastIndexAny("golang", "gn"), strings.IndexRune("chicken", 'k'))
	fmt.Println(strings.Cut("key=value=x", "="))
	fmt.Println(strings.CutPrefix("prefix-body", "prefix-"))
	fmt.Println(strings.SplitAfterN("a,b,c,d", ",", 3), strings.SplitN("a,b,c", ",", 0), len(strings.Split("", ",")))
	fmt.Printf("%q %q %q\n", strings.TrimFunc("¡¡Hola!!", unicode.IsPunct), strings.TrimLeft("xxhixx", "x"), strings.TrimSuffix("file.tar.gz", ".gz"))
	var parts []string
	for p := range strings.SplitSeq("x|y||z", "|") {
		parts = append(parts, p)
	}
	for f := range strings.FieldsSeq(" one  two three ") {
		parts = append(parts, strings.ToUpper(f))
	}
	for l := range strings.Lines("l1\nl2\r\nl3") {
		parts = append(parts, fmt.Sprintf("%q", l))
	}
	fmt.Println(parts)
	fmt.Println(bytes.EqualFold([]byte("Go"), []byte("GO")), bytes.Title([]byte("abc def")), string(bytes.Map(unicode.ToUpper, []byte("map"))), bytes.ContainsFunc([]byte("abc1"), unicode.IsDigit))
	var bb bytes.Buffer
	bb.WriteString("buffer ")
	bb.WriteRune('✓')
	fmt.Fprintf(&bb, " %v", Slug("X Y").Normalize())
	line, err := bb.ReadString(' ')
	fmt.Printf("%q %v %q %d\n", line, err, bb.String(), bb.Len())
	rd := strings.NewReader("reader")
	rd.Seek(2, 0)
	rr, _, _ := rd.ReadRune()
	fmt.Println(string(rr), rd.Len(), rd.Size())
	fmt.Println(strings.Compare("a", "b"), strings.ContainsRune("abc", 'c'), strings.Repeat("ab", 3), strings.Index("chicken", "ken"), strings.IndexFunc("Hello, 世界", func(r rune) bool { return r > 127 }))
}
