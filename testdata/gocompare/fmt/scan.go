// fmt scanning from strings: Sscan, Sscanln, Sscanf with many verbs and
// widths, Fscan and Fscanln from readers, the counts and the errors on
// bad input, unexpected newlines and EOF, and a type with a Scan method.
package main

import (
	"fmt"
	"strings"
)

type Pair struct{ A, B string }

func (p *Pair) Scan(st fmt.ScanState, verb rune) error {
	st.SkipSpace()
	tok, err := st.Token(false, func(r rune) bool { return r != ':' && r != ' ' && r != '\n' })
	if err != nil {
		return err
	}
	p.A = string(tok)
	r, _, err := st.ReadRune()
	if err != nil {
		return err
	}
	if r != ':' {
		return fmt.Errorf("want ':' got %q", r)
	}
	tok, err = st.Token(false, func(r rune) bool { return r != ' ' && r != '\n' })
	p.B = string(tok)
	return err
}

func main() {
	var i int
	var s string
	var f float64
	var b bool
	var u uint8
	var r rune
	var i64 int64
	n, err := fmt.Sscan("42 hello 3.5 true 200", &i, &s, &f, &b, &u)
	fmt.Println(n, err, i, s, f, b, u)
	n, err = fmt.Sscan("1\n\n2", &i, &i64)
	fmt.Println(n, err, i, i64)
	n, err = fmt.Sscan("7 x", &i, &f)
	fmt.Println(n, err, i, f)
	n, err = fmt.Sscan("", &i)
	fmt.Println(n, err)
	n, err = fmt.Sscan("300", &u)
	fmt.Println(n, err, u)
	n, err = fmt.Sscan("0x1F 0b101 0o17 017 -0x10 1_000", &i, &i64, &u, &r, &f, &s)
	fmt.Println(n, err, i, i64, u, r, f, s)
	n, err = fmt.Sscanln("1 2\n3", &i, &i64)
	fmt.Println(n, err, i, i64)
	n, err = fmt.Sscanln("1\n2", &i, &i64)
	fmt.Println(n, err, i)
	n, err = fmt.Sscanln("1 2 3", &i, &i64)
	fmt.Println(n, err)
	var a, c string
	var x, y int
	n, err = fmt.Sscanf("key=val; 10,20", "%3s=%s %d,%d", &a, &c, &x, &y)
	fmt.Println(n, err, a, c, x, y)
	n, err = fmt.Sscanf("abc123", "%3s%d", &a, &x)
	fmt.Println(n, err, a, x)
	n, err = fmt.Sscanf("ff 0x10 777 1010", "%x %v %o %b", &x, &y, &i, &i64)
	fmt.Println(n, err, x, y, i, i64)
	n, err = fmt.Sscanf("é z", "%c %c", &r, &x)
	fmt.Println(n, err, r, x)
	n, err = fmt.Sscanf("3.25e2 -1.5", "%g %f", &f, &f)
	fmt.Println(n, err, f)
	n, err = fmt.Sscanf(`"quoted str" 'x'`, "%q %q", &a, &r)
	fmt.Println(n, err, a, r)
	n, err = fmt.Sscanf("12345", "%2d%3d", &x, &y)
	fmt.Println(n, err, x, y)
	n, err = fmt.Sscanf("10 abc", "%d %d", &x, &y)
	fmt.Println(n, err, x)
	n, err = fmt.Sscanf("10", "%d %d", &x, &y)
	fmt.Println(n, err)
	n, err = fmt.Sscanf("lit 5", "lat %d", &x)
	fmt.Println(n, err)
	n, err = fmt.Sscanf("5%", "%d%%", &x)
	fmt.Println(n, err, x)
	n, err = fmt.Sscanf("true", "%t", &b)
	fmt.Println(n, err, b)
	n, err = fmt.Sscanf("1\n2", "%d %d", &x, &y)
	fmt.Println(n, err)
	n, err = fmt.Sscanf("1\n2", "%d\n%d", &x, &y)
	fmt.Println(n, err, x, y)
	n, err = fmt.Sscanf("x", "%d", &s)
	fmt.Println(n, err)
	n, err = fmt.Sscanf("5", "%s", &x)
	fmt.Println(n, err)
	var bs []byte
	n, err = fmt.Sscanf("6869 rest", "%x %s", &bs, &a)
	fmt.Println(n, err, string(bs), a)
	n, err = fmt.Sscan("1", 5)
	fmt.Println(n, err)
	var p Pair
	n, err = fmt.Sscan("k:v other", &p, &a)
	fmt.Println(n, err, p, a)
	n, err = fmt.Sscan("novalue", &p)
	fmt.Println(n, err, p)
	rd := strings.NewReader("1 2 3\n4 5\nsix\n")
	for {
		var v int
		n, err := fmt.Fscan(rd, &v)
		fmt.Print(n, " ", err, " ", v, "; ")
		if err != nil {
			break
		}
	}
	fmt.Println()
	rd = strings.NewReader("a b\nc\n\nd e f\n")
	for k := 0; k < 5; k++ {
		var p, q string
		n, err := fmt.Fscanln(rd, &p, &q)
		fmt.Printf("%d %v %q %q; ", n, err, p, q)
	}
	fmt.Println()
	rd = strings.NewReader("x=1 y=2")
	var k1, k2 string
	var v1, v2 int
	n, err = fmt.Fscanf(rd, "%1s=%d %1s=%d", &k1, &v1, &k2, &v2)
	fmt.Println(n, err, k1, v1, k2, v2)
	var c128 complex128
	n, err = fmt.Sscan("(1+2i)", &c128)
	fmt.Println(n, err, c128)
}
