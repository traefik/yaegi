// AOT test program: encoding/asn1 on interpreted types.
//
//aot:lib encoding/asn1
package main

import (
	"encoding/asn1"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Version int

type Ext struct {
	ID       asn1.ObjectIdentifier
	Critical bool `asn1:"optional"`
	Value    []byte
}

type Cert struct {
	Version Version `asn1:"explicit,tag:0,default:0"`
	Serial  int64
	Subject string `asn1:"utf8"`
	Email   string `asn1:"ia5,optional"`
	Exts    []Ext  `asn1:"optional"`
	Flags   asn1.BitString
	Raw     asn1.RawContent
}

type Simple struct {
	A int
	B string `asn1:"printable"`
	C []int  `asn1:"set"`
}

func main() {
	if benchMain(benchOp) {
		return
	}
	c := Cert{Version: 2, Serial: 1234567, Subject: "José", Email: "a@b.c",
		Exts:  []Ext{{asn1.ObjectIdentifier{2, 5, 29, 15}, true, []byte{3, 2, 1}}},
		Flags: asn1.BitString{Bytes: []byte{0xa0}, BitLength: 3}}
	b, err := asn1.Marshal(c)
	fmt.Printf("%v %x\n", err, b)
	var d Cert
	rest, err := asn1.Unmarshal(b, &d)
	fmt.Println(err, len(rest), d.Version, d.Serial, d.Subject, d.Email, d.Exts[0].ID, d.Exts[0].Critical, d.Exts[0].Value, d.Flags.At(0), len(d.Raw) > 0)
	s := Simple{A: -3, B: "hi", C: []int{3, 1}}
	b2, err := asn1.Marshal(s)
	fmt.Printf("%v %x\n", err, b2)
	var t Simple
	_, err = asn1.Unmarshal(b2, &t)
	fmt.Println(err, t)
	var v Version
	_, err = asn1.Unmarshal([]byte{2, 1, 9}, &v)
	fmt.Println(err, v)
	_, err = asn1.Unmarshal(b2, &v)
	fmt.Println(err)
	_, err = asn1.Marshal(struct{ F float64 }{1})
	fmt.Println(err)
}

// benchOp: marshal and unmarshal a certificate-like structure.
func benchOp() {
	c := Cert{Version: 2, Serial: 99, Subject: "x", Exts: []Ext{{asn1.ObjectIdentifier{1, 2}, false, []byte{1}}}}
	b, err := asn1.Marshal(c)
	if err != nil {
		panic(err)
	}
	var d Cert
	if _, err := asn1.Unmarshal(b, &d); err != nil {
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
