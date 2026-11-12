package main

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unsafe"
)

// The paths that allocate less: a field read through a native pointer
// (req.Header) without a boxed field address, a string map key not boxed
// again, and the iterators of the range loops over native maps taken from a
// pool with their slots. The values must stay the ones of Go.

type named string

func main() {
	fields()
	stringKeys()
	ranges()
}

func fields() {
	req, _ := http.NewRequest("GET", "http://example.com/a/b?x=1", nil)
	req.Header.Set("X-A", "1")
	h := req.Header // the map itself: later writes through req are seen
	req.Header.Set("X-B", "2")
	fmt.Println(len(h), h.Get("X-B"), req.Header.Get("X-A"))
	p := req.URL.Path // a string: a copy
	req.URL.Path = "/c"
	fmt.Println(p, req.URL.Path, req.URL.RawQuery, req.Method, req.ProtoMajor)

	// A struct field read through a native pointer is a copy.
	c, err := http.ParseSetCookie("a=b; Expires=Wed, 21 Oct 2015 07:28:00 GMT; Path=/p")
	if err != nil {
		panic(err)
	}
	e := c.Expires
	c.Expires = c.Expires.AddDate(1, 0, 0)
	fmt.Println(e.Year(), c.Expires.Year(), c.Path, c.Name, c.Value)

	// A nil native pointer still panics as in Go.
	var u *url.URL
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(u.Path)
	}()
}

func stringKeys() {
	m := map[string]int{}
	var empty string
	m[empty]++
	m[""]++
	m["a"+strings.Repeat("b", 2)] = 5
	m["abb"]++
	fmt.Println(len(m), m[""], m["abb"])
	n := map[named]string{"x": "1"}
	n[named("x")] += "2"
	n[""] = "e"
	k := named(strings.ToUpper("y"))
	n[k] = "3"
	fmt.Println(len(n), n["x"], n["Y"], n[""])
	a := map[any]int{"s": 1, named("s"): 2}
	a["s"]++
	fmt.Println(a["s"], a[named("s")], len(a))
	delete(m, "")
	_, ok := m[""]
	fmt.Println(ok, len(m))
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println(keys)

	// The keys that a range gives back, for maps that keep no second table
	// of keys (strings, integers, bools) and for the others.
	i8 := map[int8]bool{-1: true, 5: false, -128: true}
	u64 := map[uint64]int{1 << 63: 1, 7: 2}
	bs := map[bool]int{true: 1, false: 2}
	var out []string
	for k, v := range i8 {
		out = append(out, fmt.Sprint("i8 ", k, v))
	}
	for k, v := range u64 {
		out = append(out, fmt.Sprint("u64 ", k, v))
	}
	for k, v := range bs {
		out = append(out, fmt.Sprint("b ", k, v))
	}
	type pt struct{ x, y int }
	mixed := map[any]int{"s": 1, 2: 2, pt{1, 2}: 3, named("n"): 4, int8(2): 5}
	for k, v := range mixed {
		out = append(out, fmt.Sprintf("any %T %v %d", k, k, v))
	}
	sm := map[string]int{"a": 1}
	sm[strings.ToLower("B")] = 2
	ps := map[pt]string{{1, 2}: "a"}
	ps[pt{3, 4}] = "b"
	for k, v := range sm {
		out = append(out, fmt.Sprint("sm ", k, v))
	}
	for k, v := range ps {
		out = append(out, fmt.Sprint("ps ", k, v))
	}
	sort.Strings(out)
	for _, s := range out {
		fmt.Println(s)
	}

	// An empty substring keeps its data pointer, also as a map key.
	src := "abcdef"
	e := src[2:2]
	em := map[string]int{e: 1}
	for k := range em {
		fmt.Println(len(k), unsafe.StringData(k) == unsafe.StringData(e))
	}
}

// walk ranges over a native map recursively: each level has its own
// iterator while the others are still running.
func walk(h http.Header, depth int) int {
	if depth == 0 {
		return 0
	}
	n := 0
	for k, v := range h {
		n += len(k) + len(v) + walk(h, depth-1)
	}
	return n
}

func ranges() {
	h := http.Header{}
	h.Set("X-One", "1")
	h.Set("X-Two", "2")
	h.Add("X-Two", "22")
	_, params, err := mime.ParseMediaType("text/plain; charset=utf-8; a=b")
	if err != nil {
		panic(err)
	}
	q, _ := url.ParseQuery("k=1&k=2&z=")

	var kept []string
	var keptV [][]string
	brk := 0
	for i := 0; i < 3; i++ {
		// A loop left early leaves its iterator: the next ones must not
		// see its state.
		for k := range h {
			brk += len(k)
			break
		}
		// The slots change type between loops: string, then []string.
		for k, v := range params {
			kept = append(kept, k+"="+v)
		}
		for k, v := range q {
			keptV = append(keptV, v)
			kept = append(kept, k)
		}
		for _, v := range h {
			keptV = append(keptV, v)
		}
	}
	sort.Strings(kept)
	fmt.Println(kept, brk)
	var flat []string
	for _, v := range keptV {
		flat = append(flat, strings.Join(v, "+"))
	}
	sort.Strings(flat)
	fmt.Println(flat)
	fmt.Println(walk(h, 3))

	// The value of a native map shares its array with the map, as in Go,
	// also after the iterator went back to its pool.
	var saved []string
	for k, v := range h {
		if k == "X-Two" {
			saved = v
		}
	}
	for range h {
	}
	saved[0] = "changed"
	fmt.Println(h.Values("X-Two"), saved)
}
