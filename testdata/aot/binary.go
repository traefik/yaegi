// AOT test program: encoding/binary on interpreted types.
//
//aot:lib encoding/binary
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Flags uint16

type Header struct {
	Magic   [4]byte
	Version uint8
	Flags   Flags
	Length  int32
	Ratio   float32
	_       [2]byte
	Items   [3]uint16
}

func main() {
	if benchMain(benchOp) {
		return
	}
	h := Header{Magic: [4]byte{'G', 'O', 'V', 'M'}, Version: 2, Flags: 0x8001, Length: -5, Ratio: 1.5, Items: [3]uint16{1, 2, 3}}
	fmt.Println(binary.Size(h), binary.Size(&h), binary.Size([]Flags{1, 2}))
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, h)
	fmt.Println(err, buf.Len(), fmt.Sprintf("% x", buf.Bytes()))
	var g Header
	err = binary.Read(bytes.NewReader(buf.Bytes()), binary.BigEndian, &g)
	fmt.Println(err, string(g.Magic[:]), g.Version, g.Flags, g.Length, g.Ratio, g.Items)
	fs := []Flags{1, 2, 0xffff}
	buf.Reset()
	binary.Write(&buf, binary.LittleEndian, fs)
	out := make([]Flags, 3)
	binary.Read(&buf, binary.LittleEndian, out)
	fmt.Println(out)
	b, err := binary.Append(nil, binary.LittleEndian, struct{ A, B int16 }{1, -1})
	fmt.Println(b, err)
	var x struct{ A, B int16 }
	n, err := binary.Decode(b, binary.LittleEndian, &x)
	fmt.Println(n, err, x)
	fmt.Println(binary.Write(&buf, binary.BigEndian, struct{ S string }{"no"}))
	var p *Header
	fmt.Println(binary.Size(p))
	fmt.Println(binary.BigEndian.Uint32([]byte{0, 0, 1, 0}), binary.PutUvarint(make([]byte, 10), 300))
}

// benchOp: write and read a header.
func benchOp() {
	var buf bytes.Buffer
	h := Header{Version: 2, Flags: 1, Length: 7, Items: [3]uint16{1, 2, 3}}
	if err := binary.Write(&buf, binary.BigEndian, h); err != nil {
		panic(err)
	}
	var g Header
	if err := binary.Read(&buf, binary.BigEndian, &g); err != nil {
		panic(err)
	}
}

func benchMain(op func()) bool {
	n, _ := strconv.Atoi(os.Getenv("AOT_BENCH"))
	if n <= 0 {
		return false
	}
	op()
	t0 := time.Now()
	for i := 0; i < n; i++ {
		op()
	}
	fmt.Printf("BENCH %d\n", time.Since(t0).Nanoseconds()/int64(n))
	return true
}
