package main

import (
	"fmt"
	"net/http"
	"net/url"
)

// Maps updated by native code.

func main() {
	v := url.Values{}
	v.Add("a", "1")
	v.Add("a", "2")
	v.Set("b", "3")
	fmt.Println(v.Get("a"), v["a"], v.Get("b"), v.Encode())
	v.Del("b")
	fmt.Println(len(v), v.Has("b"))

	h := http.Header{}
	h.Set("content-type", "text/plain")
	h.Add("X-A", "x")
	fmt.Println(h.Get("Content-Type"), h, len(h))

	// A plain map converted to a native named map type: native updates are
	// copied back, values native code did not change keep their identity.
	m := map[string][]string{"a": {"x"}}
	xa := m["a"]
	url.Values(m).Set("b", "y")
	xa[0] = "changed"
	fmt.Println(m["a"], m["b"], len(m))
}
