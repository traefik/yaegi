// Strings, runes and bytes: conversions between string, []byte, []rune,
// rune and integers, invalid UTF-8, ranges over strings, indexing and
// slicing, string concatenation in loops, comparisons, and the strings,
// bytes, unicode/utf8 and strings.Builder APIs.
package main

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Name string
type Bytes []byte
type Runes []rune

func main() {
	s := "héllo, 世界!"
	fmt.Println(len(s), utf8.RuneCountInString(s), s[1], s[1:3], s[len(s)-1:])
	for i, r := range s {
		fmt.Print(i, ":", string(r), " ")
	}
	fmt.Println()
	for i := 0; i < len(s); i++ {
		fmt.Printf("%02x", s[i])
	}
	fmt.Println()

	// Invalid UTF-8.
	bad := "a\xffb\xc3"
	for i, r := range bad {
		fmt.Print(i, ":", r, " ")
	}
	fmt.Println(utf8.ValidString(bad), []rune(bad), string([]rune(bad)))
	fmt.Printf("%q %+q %x\n", bad, "é", bad)

	// Conversions.
	b := []byte(s)
	r := []rune(s)
	b[0] = 'H'
	r[1] = 'E'
	fmt.Println(string(b), string(r), s)
	fmt.Println(string(rune(65)), string(rune(0x4e16)), string(rune(-1)), string(rune(0x10ffff+1)))
	fmt.Println(Name("x")+"y", string(Bytes("raw")), string(Runes{'o', 'k'}), Bytes(Name("n")))
	var nb []byte
	fmt.Println(string(nb) == "", len(string(nb)))

	// Strings are immutable: changing the byte slice does not change them.
	src := []byte("abc")
	str := string(src)
	src[0] = 'z'
	fmt.Println(str, string(src))

	// Comparisons.
	fmt.Println("a" < "b", "ab" < "b", "" < "a", "Z" < "a", "é" > "z", strings.Compare("x", "x"))

	// Concatenation in a loop and Builder.
	acc := ""
	for i := range 5 {
		acc += fmt.Sprint(i)
	}
	var sb strings.Builder
	for i := range 5 {
		fmt.Fprintf(&sb, "[%d]", i)
	}
	sb.WriteByte('!')
	sb.WriteRune('é')
	fmt.Println(acc, sb.String(), sb.Len())

	// strings functions.
	fmt.Println(strings.Split("a,b,,c", ","), len(strings.Split("", ",")), strings.SplitN("a=b=c", "=", 2))
	fmt.Println(strings.Fields("  a \t b\n c  "), strings.TrimFunc("123abc456", unicode.IsDigit))
	fmt.Println(strings.Map(func(r rune) rune {
		if r == 'l' {
			return -1
		}
		return unicode.ToUpper(r)
	}, s))
	fmt.Println(strings.EqualFold("Straße", "STRASSE"), strings.EqualFold("Go", "GO"), strings.Title("x"))
	fmt.Println(strings.NewReplacer("a", "1", "b", "2").Replace("abcab"), strings.Repeat("-", 3))
	before, after, found := strings.Cut("key=value=x", "=")
	fmt.Println(before, after, found)
	fmt.Println(strings.Index(s, "世"), strings.IndexRune(s, 'l'), strings.LastIndex(s, "l"), strings.IndexByte(s, ','))
	fmt.Println(strings.ToUpper(s), strings.ToTitle("ǆ"), strings.ContainsAny(s, "xyz界"))
	for part := range strings.SplitSeq("x|y|z", "|") {
		fmt.Print(part, ";")
	}
	fmt.Println()
	for line := range strings.Lines("l1\nl2\n") {
		fmt.Printf("%q ", line)
	}
	fmt.Println()

	// bytes functions and Buffer.
	var buf bytes.Buffer
	buf.WriteString("hello ")
	buf.Write([]byte("world"))
	fmt.Println(buf.String(), buf.Len(), bytes.Contains(buf.Bytes(), []byte("lo w")))
	p := make([]byte, 5)
	n, _ := buf.Read(p)
	fmt.Println(n, string(p), buf.String())
	fmt.Println(bytes.Equal(nil, []byte{}), bytes.Compare([]byte("a"), []byte("b")), string(bytes.ToUpper([]byte("up"))))
	fmt.Println(bytes.Fields([]byte(" a b ")), string(bytes.Join([][]byte{[]byte("x"), []byte("y")}, []byte("-"))))
	fields := bytes.Split([]byte("1,2,3"), []byte(","))
	fields[0][0] = '9' // shares the array of the input
	fmt.Println(string(fields[0]), len(fields))

	// utf8.
	enc := utf8.AppendRune(nil, '€')
	dr, size := utf8.DecodeRune(enc)
	fmt.Println(enc, string(dr), size, utf8.RuneLen('€'), utf8.RuneLen(-1))
	lr, lsize := utf8.DecodeLastRuneInString("añ")
	fmt.Println(string(lr), lsize)

	// unicode.
	fmt.Println(unicode.IsLetter('é'), unicode.IsSpace(' '), unicode.Is(unicode.Han, '世'), unicode.SimpleFold('k'))

	// Sorting strings and runes.
	words := strings.Fields("pear Apple banana apple Cherry")
	slices.Sort(words)
	fmt.Println(words)
	rs := []rune("hello")
	slices.Sort(rs)
	fmt.Println(string(rs))

	// string from int types (needs rune()).
	codes := []int32{72, 105}
	out := ""
	for _, c := range codes {
		out += string(c)
	}
	fmt.Println(out)

	// Byte arithmetic.
	c := byte('a')
	c += 25
	c++
	fmt.Println(c, string(c), 'a'+1, string('a'+1))
}
