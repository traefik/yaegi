// strings.Builder (Write*, Grow, Len, Reset, and the panic on a copied
// Builder) and strings.Reader (Read, ReadAt, ReadByte, UnreadByte,
// ReadRune, UnreadRune, Seek from each origin, WriteTo, Len, Size, Reset).
package main

import (
	"fmt"
	"io"
	"strings"
)

func main() {
	var b strings.Builder
	fmt.Println(b.Len(), b.Cap() >= 0, b.String() == "")
	b.Grow(10)
	fmt.Println(b.Cap() >= 10)
	n, err := b.WriteString("héllo")
	fmt.Println(n, err)
	fmt.Println(b.WriteByte(' '))
	n, err = b.WriteRune('世')
	fmt.Println(n, err)
	n, err = b.WriteRune(0xd800)
	fmt.Println(n, err)
	n, err = b.WriteRune(-5)
	fmt.Println(n, err)
	n, err = b.Write([]byte{'!', 0xff})
	fmt.Println(n, err)
	fmt.Fprintf(&b, "[%d|%s]", 42, "x")
	fmt.Printf("%q %d\n", b.String(), b.Len())
	b.Reset()
	fmt.Printf("%q %d\n", b.String(), b.Len())
	for i := 0; i < 100; i++ {
		b.WriteString("ab")
	}
	fmt.Println(b.Len(), b.String()[:6])
	func() {
		defer func() { fmt.Println("grow:", recover()) }()
		var g strings.Builder
		g.Grow(-1)
	}()
	func() {
		defer func() { fmt.Println("copy:", recover()) }()
		var a strings.Builder
		a.WriteString("x")
		c := a
		c.WriteString("y")
	}()

	r := strings.NewReader("héllo, 世界!")
	fmt.Println(r.Len(), r.Size())
	buf := make([]byte, 4)
	n, err = r.Read(buf)
	fmt.Printf("%d %v %q %d\n", n, err, buf[:n], r.Len())
	ch, size, err := r.ReadRune()
	fmt.Printf("%q %d %v\n", ch, size, err)
	fmt.Println(r.UnreadRune())
	fmt.Println(r.UnreadRune())
	c, err := r.ReadByte()
	fmt.Println(c, err)
	fmt.Println(r.UnreadByte(), r.Len())
	fmt.Println(r.UnreadRune())
	for _, sk := range []struct {
		off    int64
		whence int
	}{{0, io.SeekStart}, {3, io.SeekStart}, {2, io.SeekCurrent}, {-1, io.SeekEnd}, {-100, io.SeekCurrent}, {100, io.SeekStart}, {0, 7}} {
		pos, err := r.Seek(sk.off, sk.whence)
		ch, size, rerr := r.ReadRune()
		fmt.Printf("seek %d,%d -> %d %v | %q %d %v\n", sk.off, sk.whence, pos, err, ch, size, rerr)
	}
	at := make([]byte, 5)
	for _, off := range []int64{0, 7, 13, 14, 20, -1} {
		n, err := r.ReadAt(at, off)
		fmt.Printf("at %d: %d %v %q\n", off, n, err, at[:n])
	}
	r.Seek(1, io.SeekStart)
	ch, size, err = r.ReadRune()
	fmt.Printf("mid: %q %d %v\n", ch, size, err)
	r.Reset("a\xffb")
	for {
		ch, size, err := r.ReadRune()
		if err != nil {
			fmt.Println(err, err == io.EOF)
			break
		}
		fmt.Printf("%q %d; ", ch, size)
	}
	r.Reset("write to me")
	r.ReadByte()
	var sb strings.Builder
	wn, err := r.WriteTo(&sb)
	fmt.Println(wn, err, sb.String(), r.Len())
	r.Reset("abc")
	all, err := io.ReadAll(r)
	fmt.Println(string(all), err)
	n, err = r.Read(buf)
	fmt.Println(n, err)
	fmt.Println(r.UnreadByte(), r.Len())
	var zr strings.Reader
	fmt.Println(zr.Len(), zr.Size())
	_, err = zr.ReadByte()
	fmt.Println(err, zr.UnreadByte())
}
