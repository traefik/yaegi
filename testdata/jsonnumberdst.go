package main

// A native decoder fills a pointer to a native named type of basic kind
// (json.Number, time.Duration) given as the destination itself. In default
// mode the pointer goes to native code as a shadow of the interpreted cell.
// The write-back took a pointer to a native named type as a native value
// and did not write the shadow into the cell: the value stayed empty
// (golang-jwt's NumericDate.UnmarshalJSON failed to parse every token).

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

func main() {
	var number json.Number
	err := json.Unmarshal([]byte(`42`), &number)
	fmt.Printf("%q %v\n", number, err)
	f, err := number.Float64()
	fmt.Println(f, err)

	var d time.Duration
	err = json.Unmarshal([]byte(`42`), &d)
	fmt.Println(d, err)

	var n2 json.Number
	err = json.NewDecoder(strings.NewReader(`43`)).Decode(&n2)
	fmt.Printf("%q %v\n", n2, err)

	p := &n2
	err = json.Unmarshal([]byte(`44`), p)
	fmt.Printf("%q %q %v\n", n2, *p, err)

	var s struct{ N json.Number }
	err = json.Unmarshal([]byte(`45`), &s.N)
	fmt.Printf("%q %v\n", s.N, err)

	var pn *json.Number
	err = json.Unmarshal([]byte(`46`), &pn)
	fmt.Printf("%q %v\n", *pn, err)

	// a read only call keeps the value
	b, err := json.Marshal(&number)
	fmt.Println(string(b), err, number)

	// The same write-back runs for the shadow of a cell reached through
	// another value: the pointer keeps its identity (the cell, not a native
	// pointer to the shadow).
	var n3 json.Number
	s3 := struct {
		A int
		P *json.Number
	}{P: &n3}
	err = json.Unmarshal([]byte(`{"A":1,"P":47}`), &s3)
	fmt.Println(s3.A, *s3.P, n3, s3.P == &n3, err)

	var n4 json.Number
	p4 := &n4
	err = json.Unmarshal([]byte(`48`), &p4)
	fmt.Println(n4, *p4, p4 == &n4, err)

	var n5 json.Number
	m5 := map[string]*json.Number{"x": &n5}
	err = json.Unmarshal([]byte(`{"x":49}`), &m5)
	fmt.Println(n5, *m5["x"], err)

	// the same cell twice in one call, and other native named kinds
	var d2 time.Duration
	k, err := fmt.Sscan("1 2", &d2, &d2)
	fmt.Println(k, d2, err)
	var mo time.Month
	k, err = fmt.Sscan("7", &mo)
	fmt.Println(k, mo, err)
	var v url.Values
	err = json.Unmarshal([]byte(`{"a":["b"]}`), &v)
	fmt.Println(v, v.Get("a"), err)
	var ip net.IP
	err = json.Unmarshal([]byte(`"1.2.3.4"`), &ip)
	fmt.Println(ip, err)
}
