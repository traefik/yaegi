// A native value changed to another native named type with the same
// underlying type (propagation.HeaderCarrier(req.Header) in OpenTelemetry)
// has the new type: its methods, its dynamic type in an interface and
// fmt's view of it are those of the new type.
package main

import (
	"fmt"
	"net/http"
	"net/url"
)

type getter interface{ Get(string) string }

func main() {
	h := http.Header{"A": {"1"}}
	v := url.Values(h)
	fmt.Printf("%T %s\n", v, v.Encode())
	var g getter = v
	_, isValues := g.(url.Values)
	_, isHeader := g.(http.Header)
	fmt.Println(isValues, isHeader, g.Get("A"))
	back := http.Header(v)
	fmt.Printf("%T %s\n", back, back.Get("A"))
}
