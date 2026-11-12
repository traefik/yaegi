package main

// Native code decodes into a variable that holds a native slice view
// (vm/natslice.go) with opaque handles (-opaque): the pointer to the
// variable is a native pointer to a shadow of the slice header, not an
// opaque handle that the decoder refuses.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"runtime/metrics"
)

func main() {
	h := http.Header{"A": {"b", "a"}}
	vs := h["A"]
	err := json.Unmarshal([]byte(`["x"]`), &vs)
	fmt.Println(vs, len(vs), cap(vs), h["A"], err)
	vs = h["A"]
	err = json.Unmarshal([]byte(`["p","q","r"]`), &vs)
	vs[0] = "new array"
	fmt.Println(vs, h["A"], err)

	loc := regexp.MustCompile(`b+`).FindStringIndex("aabbbcc")
	is := loc
	err = json.Unmarshal([]byte(`[7]`), &is)
	fmt.Println(is, loc, err)

	hist := &metrics.Float64Histogram{Buckets: []float64{0.5, 1.5}}
	fl := hist.Buckets
	err = json.Unmarshal([]byte(`[3.5,4.5,5.5]`), &fl)
	fmt.Println(fl, hist.Buckets, err)
}
