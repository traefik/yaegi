// bytes.Buffer: Write*, Read*, ReadFrom, WriteTo, Next, Truncate, Grow,
// UnreadByte/UnreadRune rules, ReadBytes/ReadString delimiters, Available
// and AvailableBuffer, and the errors at EOF.
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

func main() {
	var b bytes.Buffer
	fmt.Println(b.Len(), b.String() == "", b.Bytes() == nil)
	b.WriteString("héllo")
	b.WriteByte(' ')
	b.WriteRune('世')
	b.WriteRune(0x110000)
	b.Write([]byte("\nline2\nend"))
	fmt.Printf("%q %d\n", b.String(), b.Len())
	c, err := b.ReadByte()
	fmt.Println(c, err, b.UnreadByte(), b.UnreadByte())
	r, size, err := b.ReadRune()
	fmt.Printf("%q %d %v\n", r, size, err)
	r, size, err = b.ReadRune()
	fmt.Printf("%q %d %v\n", r, size, err)
	fmt.Println(b.UnreadRune(), b.UnreadRune())
	line, err := b.ReadString('\n')
	fmt.Printf("%q %v\n", line, err)
	lb, err := b.ReadBytes('\n')
	fmt.Printf("%q %v\n", lb, err)
	fmt.Println(b.UnreadByte())
	rest, err := b.ReadString('\n')
	fmt.Printf("%q %v\n", rest, err)
	rest, err = b.ReadString('\n')
	fmt.Printf("%q %v\n", rest, err)
	_, err = b.ReadByte()
	fmt.Println(err)
	_, _, err = b.ReadRune()
	fmt.Println(err)
	n, err := b.Read(make([]byte, 3))
	fmt.Println(n, err)
	n, err = b.Read(nil)
	fmt.Println(n, err)

	b2 := bytes.NewBufferString("0123456789")
	fmt.Printf("%q %q %d\n", b2.Next(3), b2.Next(0), b2.Len())
	fmt.Printf("%q\n", b2.Next(100))
	b2 = bytes.NewBuffer([]byte("abcdefgh"))
	b2.Truncate(5)
	fmt.Println(b2.String())
	b2.Next(2)
	b2.Truncate(1)
	fmt.Println(b2.String(), b2.Len())
	func() {
		defer func() { fmt.Println("truncate:", recover()) }()
		b2.Truncate(10)
	}()
	func() {
		defer func() { fmt.Println("grow:", recover()) }()
		b2.Grow(-1)
	}()
	b2.Reset()
	b2.Grow(64)
	fmt.Println(b2.Len(), b2.Cap() >= 64, b2.Available() >= 64)
	ab := b2.AvailableBuffer()
	ab = append(ab, "appended"...)
	b2.Write(ab)
	fmt.Println(b2.String())
	rn, err := b2.ReadFrom(strings.NewReader(strings.Repeat("z", 1000)))
	fmt.Println(rn, err, b2.Len())
	var sb strings.Builder
	wn, err := b2.WriteTo(&sb)
	fmt.Println(wn, err, sb.Len(), b2.Len())
	fmt.Fprintf(b2, "%05d|%-4s|", 42, "ab")
	fmt.Println(b2.String())
	data, err := io.ReadAll(b2)
	fmt.Printf("%q %v\n", data, err)
	var pb *bytes.Buffer
	fmt.Println(pb.String())
	b3 := bytes.NewBufferString("a,b,,c")
	for {
		tok, err := b3.ReadBytes(',')
		fmt.Printf("%q %v; ", tok, err)
		if err != nil {
			break
		}
	}
	fmt.Println()
	b4 := bytes.NewBufferString("x")
	b4.ReadByte()
	b4.WriteString("y")
	fmt.Println(b4.UnreadByte(), b4.String())
}
