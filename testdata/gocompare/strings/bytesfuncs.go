// The bytes package functions that mirror strings (Split, Fields, Index*,
// Trim*, Replace, Map, case, EqualFold, Cut, Compare, Equal, Contains*,
// Repeat, Join, Runes, Title-less) with nil and empty slices and invalid
// UTF-8, plus the aliasing of returned slices.
package main

import (
	"bytes"
	"fmt"
	"unicode"
)

func main() {
	var nilb []byte
	empty := []byte{}
	s := []byte("  Héllo, Wörld \xff ")
	fmt.Println(bytes.Equal(nilb, empty), bytes.Compare(nilb, empty), bytes.Compare([]byte("a"), []byte("b")), bytes.Compare([]byte("b"), []byte("a")))
	fmt.Printf("%q %q %q\n", bytes.Split(s, []byte(",")), bytes.Fields(s), bytes.SplitN([]byte("a,b,c"), []byte(","), 2))
	fmt.Printf("%q %q\n", bytes.Split([]byte("héllo"), nil), bytes.SplitAfter([]byte("a,b"), []byte(",")))
	fmt.Printf("%q %q %q\n", bytes.TrimSpace(s), bytes.Trim(s, " \xff"), bytes.TrimFunc(s, unicode.IsSpace))
	fmt.Printf("%q %q %q %q\n", bytes.TrimLeft(s, " H"), bytes.TrimRight(s, " \xff"), bytes.TrimPrefix(s, []byte("  H")), bytes.TrimSuffix(s, []byte(" ")))
	fmt.Printf("%q %q %q %q\n", bytes.ToUpper(s), bytes.ToLower(s), bytes.ToTitle([]byte("ǆ")), bytes.ToValidUTF8(s, []byte("?")))
	fmt.Println(bytes.Index(s, []byte("llo")), bytes.LastIndex(s, []byte("l")), bytes.IndexByte(s, 'W'), bytes.IndexRune(s, 'ö'), bytes.IndexRune(s, unicode.ReplacementChar), bytes.IndexAny(s, "öx"), bytes.LastIndexAny(s, "é"), bytes.IndexFunc(s, unicode.IsUpper), bytes.LastIndexFunc(s, unicode.IsLetter), bytes.LastIndexByte(s, ' '))
	fmt.Println(bytes.Contains(s, []byte("Wö")), bytes.ContainsAny(s, "z"), bytes.ContainsRune(s, 'é'), bytes.ContainsFunc(s, unicode.IsDigit), bytes.Count(s, []byte("l")), bytes.Count(s, nil), bytes.HasPrefix(s, []byte("  ")), bytes.HasSuffix(s, nil))
	fmt.Printf("%q %q %q\n", bytes.Replace(s, []byte("l"), []byte("L"), 2), bytes.ReplaceAll([]byte("aaa"), []byte(""), []byte("-")), bytes.Map(func(r rune) rune {
		if r == ' ' {
			return -1
		}
		return unicode.ToUpper(r)
	}, s))
	fmt.Println(bytes.EqualFold([]byte("Straße"), []byte("STRASSE")), bytes.EqualFold([]byte("Gö"), []byte("GÖ")))
	before, after, ok := bytes.Cut([]byte("k=v=w"), []byte("="))
	fmt.Printf("%q %q %v\n", before, after, ok)
	p, okp := bytes.CutPrefix([]byte("prefix-rest"), []byte("prefix-"))
	q, oks := bytes.CutSuffix([]byte("rest.go"), []byte(".txt"))
	fmt.Printf("%q %v %q %v\n", p, okp, q, oks)
	fmt.Printf("%q %q %q\n", bytes.Repeat([]byte("ab"), 3), bytes.Join([][]byte{[]byte("a"), nil, []byte("c")}, []byte(", ")), bytes.Runes([]byte("a\xffé")))
	fmt.Println(bytes.Join(nil, []byte(",")) == nil, len(bytes.Join([][]byte{}, nil)))
	fmt.Println(bytes.Clone(nilb) == nil, bytes.Clone(empty) == nil, len(bytes.Clone(empty)))
	orig := []byte("a,b,c")
	parts := bytes.Split(orig, []byte(","))
	parts[0][0] = 'X'
	fmt.Printf("%q %q\n", orig, parts)
	parts[0] = append(parts[0], 'Y')
	fmt.Printf("%q %q\n", orig, parts)
	tr := bytes.TrimSpace([]byte("  x  "))
	fmt.Println(len(tr), cap(tr) >= 1)
	fmt.Println(bytes.TrimSpace([]byte("   ")) == nil, bytes.Trim(nil, "x") == nil, bytes.Fields([]byte("  ")) == nil)
	fmt.Println(bytes.MinRead)
	var out [][]byte
	for f := range bytes.FieldsFuncSeq([]byte("a1b22c"), unicode.IsDigit) {
		out = append(out, f)
	}
	fmt.Printf("%q\n", out)
}
