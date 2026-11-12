// regexp with program types and funcs: ReplaceAllStringFunc and
// ReplaceAllFunc with program methods as callbacks, Expand with named
// groups into program structs, FindAllStringSubmatchIndex, Split, Longest,
// MatchReader over a program io.RuneReader, and compile errors.
package main

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

var semver = regexp.MustCompile(`v?(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(?:-(?P<pre>[0-9A-Za-z.]+))?`)

func parse(s string) (Version, bool) {
	m := semver.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[semver.SubexpIndex("major")])
	v.Minor, _ = strconv.Atoi(m[semver.SubexpIndex("minor")])
	v.Patch, _ = strconv.Atoi(m[semver.SubexpIndex("patch")])
	v.Pre = m[semver.SubexpIndex("pre")]
	return v, true
}

type bumper struct{ count int }

func (b *bumper) bump(s string) string {
	b.count++
	v, _ := parse(s)
	v.Minor++
	v.Patch = 0
	return v.String()
}

// runeSrc is a program io.RuneReader.
type runeSrc struct {
	rs []rune
	i  int
}

func (r *runeSrc) ReadRune() (rune, int, error) {
	if r.i >= len(r.rs) {
		return 0, 0, io.EOF
	}
	c := r.rs[r.i]
	r.i++
	return c, len(string(c)), nil
}

func main() {
	text := "go 1.26.3 then v1.27.0-rc.1, then 2.0.0 and x1.2 plus 10.20.30-beta"
	for _, m := range semver.FindAllString(text, -1) {
		v, ok := parse(m)
		fmt.Println(m, v, ok)
	}
	b := &bumper{}
	fmt.Println(semver.ReplaceAllStringFunc(text, b.bump), b.count)
	fmt.Println(string(semver.ReplaceAllFunc([]byte(text), func(m []byte) []byte { return []byte(strings.ToUpper(string(m))) })))
	fmt.Println(semver.ReplaceAllString(text, "<${major}|$minor|${pre}>"))
	fmt.Println(semver.ReplaceAllLiteralString(text, "$1"))
	fmt.Println(semver.SubexpNames(), semver.NumSubexp(), semver.String())
	var dst []byte
	for _, sm := range semver.FindAllStringSubmatchIndex(text, 2) {
		fmt.Println(sm)
		dst = semver.ExpandString(dst, "[$major.$minor] ", text, sm)
	}
	fmt.Println(string(dst))
	fmt.Println(semver.FindStringIndex("nothing"), semver.FindAllString("", -1) == nil, semver.MatchString("1.2.3"))
	sep := regexp.MustCompile(`\s*[,;]\s*`)
	fmt.Printf("%q %q %q\n", sep.Split("a , b;c ;; d", -1), sep.Split("a,b,c", 2), sep.Split("", -1))
	re := regexp.MustCompile(`a+?|a+b`)
	fmt.Println(re.FindString("aaab"))
	re.Longest()
	fmt.Println(re.FindString("aaab"))
	fmt.Println(semver.MatchReader(&runeSrc{rs: []rune("ver ünï 3.4.5")}), semver.FindReaderIndex(&runeSrc{rs: []rune("ü1.2.3")}))
	for _, bad := range []string{`(`, `a**`, `[z-a]`, `\8`, `(?P<n>a)(?P<n>b)`, `x{1001}`} {
		_, err := regexp.Compile(bad)
		fmt.Println(err)
	}
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		regexp.MustCompile(`(?<`)
	}()
	fmt.Println(regexp.QuoteMeta("1.2.3+[x]"), regexp.MustCompile(`(?i)GO`).FindAllStringIndex("go Go gO", -1))
	posix, err := regexp.CompilePOSIX(`a+|a+b`)
	fmt.Println(posix.FindString("aaab"), err)
	lit, complete := regexp.MustCompile(`hello.*`).LiteralPrefix()
	fmt.Println(lit, complete)
	multi := regexp.MustCompile(`(?m)^(\w+):\s*(.*)$`)
	for _, m := range multi.FindAllStringSubmatch("key: value\nother:  x y\nbad line\nz:", -1) {
		fmt.Printf("%q\n", m)
	}
}
