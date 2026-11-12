// regexp Find functions: Find, FindAll, FindIndex, FindSubmatch,
// FindAllSubmatchIndex, named groups, SubexpNames, SubexpIndex,
// NumSubexp, Longest, MatchString, LiteralPrefix, Split and compile errors.
package main

import (
	"fmt"
	"regexp"
)

func main() {
	re := regexp.MustCompile(`(?P<key>\w+)=(?P<val>[^;]*);?`)
	s := "a=1;bb=two;c=;é=x;d"
	fmt.Println(re.String(), re.NumSubexp(), re.SubexpNames(), re.SubexpIndex("val"), re.SubexpIndex("none"))
	fmt.Printf("%q\n", re.FindString(s))
	fmt.Println(re.FindStringIndex(s))
	fmt.Printf("%q\n", re.FindStringSubmatch(s))
	fmt.Println(re.FindStringSubmatchIndex(s))
	fmt.Printf("%q\n", re.FindAllString(s, -1))
	fmt.Printf("%q\n", re.FindAllString(s, 2))
	fmt.Println(re.FindAllStringIndex(s, -1))
	fmt.Printf("%q\n", re.FindAllStringSubmatch(s, -1))
	fmt.Println(re.FindAllStringSubmatchIndex(s, -1))
	fmt.Printf("%q %q\n", re.Find([]byte(s)), re.FindSubmatch([]byte(s)))
	fmt.Printf("%q\n", re.FindAll([]byte(s), -1))
	fmt.Println(re.FindIndex([]byte("none")), re.FindStringSubmatch("none") == nil, re.FindAllString("", -1) == nil)
	opt := regexp.MustCompile(`(a)|(b)`)
	fmt.Printf("%q %v\n", opt.FindStringSubmatch("b"), opt.FindStringSubmatchIndex("xb"))
	empty := regexp.MustCompile(`x*`)
	fmt.Printf("%q %v\n", empty.FindAllString("axxbx", -1), empty.FindAllStringIndex("axxbx", -1))
	fmt.Printf("%q\n", regexp.MustCompile(``).FindAllString("héllo", -1))
	alt := regexp.MustCompile(`a|ab|abc`)
	fmt.Printf("%q", alt.FindString("abcd"))
	alt.Longest()
	fmt.Printf(" %q\n", alt.FindString("abcd"))
	lazy := regexp.MustCompile(`a+?`)
	fmt.Printf("%q", lazy.FindString("aaa"))
	lazy = regexp.MustCompilePOSIX(`a+`)
	fmt.Printf(" %q\n", lazy.FindString("aaa"))
	for _, p := range []string{`a(b`, `a**`, `[z-a]`, `\8`, `(?P<n>a)(?P<n>b)`, `x{1001}`, `\pX`, `(?<name>a)`} {
		_, err := regexp.Compile(p)
		fmt.Println(p, "->", err)
	}
	func() {
		defer func() { fmt.Println("must:", recover()) }()
		regexp.MustCompile(`(`)
	}()
	for _, c := range [][2]string{{`^\d+$`, "12345"}, {`^\d+$`, "12a"}, {`(?i)hello`, "HeLLo"}, {`(?s)a.b`, "a\nb"}, {`a.b`, "a\nb"}, {`(?m)^b$`, "a\nb\nc"}, {`\bfoo\b`, "a foo b"}, {`\p{Greek}+`, "abc αβγ"}, {`[[:upper:]]`, "abC"}, {`é`, "caf\xc3\xa9"}, {`.`, "\xff"}} {
		ok, err := regexp.MatchString(c[0], c[1])
		fmt.Printf("%s %q %v %v\n", c[0], c[1], ok, err)
	}
	for _, p := range []string{`abc`, `abc+`, `(abc)`, `^abc`, `a|b`, ``} {
		pre, complete := regexp.MustCompile(p).LiteralPrefix()
		fmt.Printf("%q %q %v\n", p, pre, complete)
	}
	sp := regexp.MustCompile(`\s*,\s*`)
	for _, n := range []int{-1, 0, 1, 2, 10} {
		fmt.Printf("%d %q\n", n, sp.Split("a , b,c ,, d", n))
	}
	fmt.Printf("%q %q\n", regexp.MustCompile(`x*`).Split("axbxxc", -1), regexp.MustCompile(``).Split("abc", -1))
	fmt.Println(regexp.QuoteMeta(`a.b*c+?(d)[e]{f}|^$\`), re.MatchString("k=v"), re.Match([]byte("nope")))
	fmt.Println(regexp.MustCompile(`(?U)a+`).FindString("aaa"), regexp.MustCompile(`a{2,3}`).FindAllString("aaaaaaa", -1))
}
