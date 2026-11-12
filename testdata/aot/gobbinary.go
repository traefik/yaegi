// AOT test program: encoding/gob with plugin types that implement
// encoding.BinaryMarshaler and encoding.TextMarshaler (and their
// Unmarshaler). The copy of gob asserts them with reflect.TypeAssert[I],
// for which cmd/aotgen writes no wrapper: the call stopped with "no AOT
// wrapper for interface encoding.BinaryMarshaler". The VM registers the
// wrappers of the interfaces of package encoding itself (aotwrapstd.go).
// (From the reflect battery of the modes challenge, case gob in AOT mode.)
//
//aot:lib encoding/gob
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"strconv"
)

type Ver struct{ Major, Minor int }

func (v Ver) MarshalBinary() ([]byte, error) { return []byte{byte(v.Major), byte(v.Minor)}, nil }

func (v *Ver) UnmarshalBinary(b []byte) error {
	if len(b) != 2 {
		return fmt.Errorf("bad version %q", b)
	}
	v.Major, v.Minor = int(b[0]), int(b[1])
	return nil
}

type Temp struct{ C float64 }

func (t Temp) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatFloat(t.C, 'f', 1, 64) + "C"), nil
}

func (t *Temp) UnmarshalText(b []byte) error {
	c, err := strconv.ParseFloat(string(b[:len(b)-1]), 64)
	t.C = c * 10
	return err
}

type Box struct {
	V    Ver
	T    Temp
	P    *Ver
	List []Ver
	ByT  map[string]Temp
}

func main() {
	var buf bytes.Buffer
	in := Box{V: Ver{1, 2}, T: Temp{2.5}, P: &Ver{3, 4}, List: []Ver{{5, 6}}, ByT: map[string]Temp{"k": {1}}}
	err := gob.NewEncoder(&buf).Encode(in)
	fmt.Printf("%q %v\n", buf.Bytes(), err)
	var out Box
	err = gob.NewDecoder(&buf).Decode(&out)
	fmt.Println(err, out.V, out.T, out.P != nil && *out.P == Ver{3, 4}, out.List, out.ByT["k"])

	// an error of UnmarshalBinary comes back
	buf.Reset()
	_ = gob.NewEncoder(&buf).Encode(struct{ V []byte }{[]byte{1}})
	var bad struct{ V Ver }
	fmt.Println(gob.NewDecoder(&buf).Decode(&bad))
}
