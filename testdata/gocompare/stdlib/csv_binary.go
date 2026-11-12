// encoding/csv, encoding/base64, encoding/hex and encoding/binary with
// program types: binary.Write and binary.Read of program structs, Size,
// Append and Put functions, varints, and the errors of each package.
package main

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type Header struct {
	Magic   [4]byte
	Version uint16
	Flags   int8
	_       byte
	Count   uint32
	Ratio   float32
	Off     int64
	Pairs   [2]Pair
}

type Pair struct {
	A int16
	B uint8
}

type Row struct {
	Name  string
	Price string
	Qty   string
}

func main() {
	h := Header{Magic: [4]byte{'G', 'V', 'M', 0}, Version: 0x0127, Flags: -2, Count: 1 << 20, Ratio: 0.75, Off: -1234567890123, Pairs: [2]Pair{{-1, 2}, {300, 255}}}
	fmt.Println(binary.Size(h), binary.Size(&h), binary.Size([]Pair{{}, {}}), binary.Size(struct{ S []int }{}))
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		var buf bytes.Buffer
		fmt.Println(order, binary.Write(&buf, order, h))
		fmt.Println(hex.EncodeToString(buf.Bytes()))
		var back Header
		fmt.Println(binary.Read(&buf, order, &back))
		fmt.Printf("%+v\n", back)
		b, err := binary.Append(nil, order, []Pair{{1, 2}, {3, 4}})
		fmt.Println(b, err)
		var ps [2]Pair
		n, err := binary.Decode(b, order, &ps)
		fmt.Println(n, err, ps)
		b = make([]byte, 3)
		n, err = binary.Encode(b, order, Pair{5, 6})
		fmt.Println(n, err, b)
	}
	var short Header
	fmt.Println(binary.Read(bytes.NewReader([]byte{1, 2, 3}), binary.LittleEndian, &short))
	fmt.Println(binary.Write(io.Discard, binary.LittleEndian, struct{ S string }{"x"}))
	fmt.Println(binary.Write(io.Discard, binary.LittleEndian, 5))
	b := binary.BigEndian.AppendUint32(nil, 0xdeadbeef)
	b = binary.LittleEndian.AppendUint16(b, 0x1234)
	b = binary.AppendUvarint(b, 300)
	b = binary.AppendVarint(b, -300)
	fmt.Println(b, binary.BigEndian.Uint32(b), binary.LittleEndian.Uint16(b[4:]))
	u, n := binary.Uvarint(b[6:])
	v, m := binary.Varint(b[6+n:])
	fmt.Println(u, n, v, m)
	vb := make([]byte, binary.MaxVarintLen64)
	for _, x := range []int64{0, 1, -1, 63, -64, 1 << 40, -1 << 62} {
		k := binary.PutVarint(vb, x)
		y, _ := binary.Varint(vb[:k])
		fmt.Print(k, ":", hex.EncodeToString(vb[:k]), ":", y, " ")
	}
	fmt.Println()
	ub := make([]byte, 8)
	binary.BigEndian.PutUint64(ub, 0x0102030405060708)
	fmt.Println(ub, binary.LittleEndian.Uint64(ub))

	// base64 / base32 / hex.
	for _, s := range []string{"", "f", "fo", "foo", "foob", "fooba", "foobar", "\xff\xfe\x00?>"} {
		fmt.Printf("%q %s %s %s %s %s %s\n", s, base64.StdEncoding.EncodeToString([]byte(s)), base64.URLEncoding.EncodeToString([]byte(s)),
			base64.RawStdEncoding.EncodeToString([]byte(s)), base32.StdEncoding.EncodeToString([]byte(s)), hex.EncodeToString([]byte(s)), base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(s)))
	}
	for _, s := range []string{"Zm9vYmFy", "Zm9vYmE=", "Zm9vYmE", "Zm9v!mFy", "===="} {
		d, err := base64.StdEncoding.DecodeString(s)
		fmt.Printf("%q %v\n", d, err)
	}
	for _, s := range []string{"48656c6c6f", "4865zz", "486"} {
		d, err := hex.DecodeString(s)
		fmt.Printf("%q %v\n", d, err)
	}
	fmt.Print(hex.Dump([]byte("dump of a program string, 0123456789")))
	var sb strings.Builder
	w := base64.NewEncoder(base64.StdEncoding, &sb)
	w.Write([]byte("streamed "))
	w.Write([]byte("bytes"))
	w.Close()
	fmt.Println(sb.String(), base64.StdEncoding.EncodedLen(10), base64.StdEncoding.DecodedLen(16), hex.EncodedLen(5))
	fmt.Println(base64.StdEncoding.AppendEncode([]byte("x:"), []byte("hi")))

	// CSV.
	in := "name,price,qty\n\"Widget, large\",1.50,3\n\"Say \"\"hi\"\"\",0,1\n# comment\nlast,\"multi\nline\",9\n"
	r := csv.NewReader(strings.NewReader(in))
	r.Comment = '#'
	var rows []Row
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("csv error:", err)
			break
		}
		line, col := r.FieldPos(1)
		fmt.Printf("%q at %d:%d offset=%d\n", rec, line, col, r.InputOffset())
		rows = append(rows, Row{rec[0], rec[1], rec[2]})
	}
	fmt.Printf("%+v\n", rows)
	var out strings.Builder
	cw := csv.NewWriter(&out)
	for _, rw := range rows {
		cw.Write([]string{rw.Name, rw.Price, rw.Qty, " lead"})
	}
	cw.Comma = ';'
	cw.UseCRLF = true
	cw.Write([]string{"a;b", "c", ""})
	cw.Flush()
	fmt.Printf("%q %v\n", out.String(), cw.Error())
	for _, bad := range []string{"a,\"b\nc", "a,b\nc\n", "a,b\"c\",d\n", "x,y\n1,2,3\n"} {
		r := csv.NewReader(strings.NewReader(bad))
		recs, err := r.ReadAll()
		fmt.Printf("%q => %q %v\n", bad, recs, err)
	}
	r = csv.NewReader(strings.NewReader("a\tb\n c\t d\n"))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	recs, err := r.ReadAll()
	fmt.Printf("%q %v\n", recs, err)
}
